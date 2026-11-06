package environments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	targetEnvironment = "environment"
	uniqueViolation   = "23505"
	constraintName    = "UX_Environments_Project_Name"
	projectArchived   = "archived"
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Options struct {
	DB      DB
	Factors auth.FactorConfirmer
	Bus     Publisher
	Log     *slog.Logger
	Now     func() time.Time
	Dialer  netguard.Dialer
	Limiter ratelimit.Limiter
}

type Service struct {
	db        DB
	factors   auth.FactorConfirmer
	bus       Publisher
	log       *slog.Logger
	now       func() time.Time
	reopen    func(ctx context.Context, run uuid.UUID) error
	dialer    netguard.Dialer
	limiter   ratelimit.Limiter
	secrets   secretstore.EnvironmentWriter
	synthetic deployments.Synthetic
}

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: o.DB, factors: o.Factors, bus: o.Bus, log: o.Log, now: now, dialer: o.Dialer, limiter: o.Limiter}
}

func (s *Service) UseSecretWriter(w secretstore.EnvironmentWriter) { s.secrets = w }

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func environmentOf(row sqlc.Environment) (models.Environment, error) {
	refs := []models.Ref{}
	if err := json.Unmarshal(row.AllowedRefs, &refs); err != nil {
		return models.Environment{}, fmt.Errorf("decode the allowed refs of environment %s: %w", row.Id, err)
	}
	e := models.Environment{
		ID: row.Id, OrganizationID: row.OrganizationId, ProjectID: row.ProjectId, Name: row.Name, Description: row.Description,
		Version: row.Version, RuleVersion: row.RuleVersion, PausedAt: row.PausedAt, PausedByUserID: row.PausedByUserId,
		PausedReason: row.PausedReason.String, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		Rules: models.Rules{
			AllowedRefs: refs, AllowPullRequests: row.AllowPullRequests, RequiredApprovals: int(row.RequiredApprovals),
			AllowSelfApproval: row.AllowSelfApproval, WaitMinutes: int(row.WaitMinutes), ApprovalTimeoutHours: int(row.ApprovalTimeoutHours),
			RequirePinnedImages: row.RequirePinnedImages, AllowedPlugins: row.AllowedPlugins, RequiredIsolation: row.RequiredIsolation,
			AllowAdminBypass: row.AllowAdminBypass, SupersedePending: row.SupersedePending,
			AllowExtraDeployEgress: row.AllowExtraDeployEgress,
		},
	}
	return e, nil
}

func (s *Service) withReviewers(ctx context.Context, q *sqlc.Queries, org uuid.UUID, rows ...sqlc.Environment) ([]models.Environment, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Id)
	}
	reviewers, err := q.ListEnvironmentReviewers(ctx, sqlc.ListEnvironmentReviewersParams{OrganizationID: org, EnvironmentIds: ids})
	if err != nil {
		return nil, err
	}
	byEnv := map[uuid.UUID]*models.Reviewers{}
	for _, id := range ids {
		byEnv[id] = &models.Reviewers{}
	}
	for _, r := range reviewers {
		into := byEnv[r.EnvironmentId]
		if r.UserId != nil {
			into.UserIDs = append(into.UserIDs, *r.UserId)
		}
		if r.TeamId != nil {
			into.TeamIDs = append(into.TeamIDs, *r.TeamId)
		}
	}
	out := make([]models.Environment, 0, len(rows))
	for _, r := range rows {
		e, err := environmentOf(r)
		if err != nil {
			return nil, err
		}
		e.Rules.Reviewers = *byEnv[r.Id]
		e.Rules = Normalize(e.Rules)
		out = append(out, e)
	}
	return out, nil
}

func (s *Service) one(ctx context.Context, q *sqlc.Queries, row sqlc.Environment) (models.Environment, error) {
	list, err := s.withReviewers(ctx, q, row.OrganizationId, row)
	if err != nil {
		return models.Environment{}, err
	}
	return list[0], nil
}

func nameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraintName
}

func notFoundOr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound()
	}
	return err
}

func (s *Service) List(ctx context.Context, org, project uuid.UUID, page, limit int) (models.Page, error) {
	q := s.queries()
	total, err := q.CountEnvironments(ctx, sqlc.CountEnvironmentsParams{ProjectID: project, OrganizationID: org})
	if err != nil {
		return models.Page{}, err
	}
	rows, err := q.ListEnvironments(ctx, sqlc.ListEnvironmentsParams{
		ProjectID: project, OrganizationID: org, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return models.Page{}, err
	}
	items, err := s.withReviewers(ctx, q, org, rows...)
	if err != nil {
		return models.Page{}, err
	}
	return models.Page{Items: items, Total: total}, nil
}

func (s *Service) Get(ctx context.Context, org, project uuid.UUID, name string) (models.Environment, error) {
	if !ValidName(name) {
		return models.Environment{}, apperrors.NotFound()
	}
	q := s.queries()
	row, err := q.GetEnvironment(ctx, sqlc.GetEnvironmentParams{ProjectID: project, OrganizationID: org, Name: name})
	if err != nil {
		return models.Environment{}, notFoundOr(err)
	}
	return s.one(ctx, q, row)
}

func (s *Service) Create(ctx context.Context, org, project uuid.UUID, in models.CreateInput, factor auth.Factor) (models.Environment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Environment{}, err
	}
	if ae := validateInput(&in.Name, in.Description, in.Rules); ae != nil {
		return models.Environment{}, ae
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Environment{}, err
	}
	var out models.Environment
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		proj, err := q.GetEnvironmentProject(ctx, sqlc.GetEnvironmentProjectParams{ProjectID: project, OrganizationID: org})
		if err != nil {
			return notFoundOr(err)
		}
		if proj.State == projectArchived {
			return apperrors.ProjectArchived()
		}
		rules := Apply(DefaultRules(proj.DefaultBranch), in.Rules)
		if details := ValidateRules(rules); len(details) > 0 {
			return apperrors.ValidationFailed(details...)
		}
		if err := checkReviewers(ctx, q, org, project, rules.Reviewers); err != nil {
			return err
		}
		refs, err := json.Marshal(rules.AllowedRefs)
		if err != nil {
			return err
		}
		description := ""
		if in.Description != nil {
			description = *in.Description
		}
		now := s.now()
		row, err := q.InsertEnvironment(ctx, sqlc.InsertEnvironmentParams{
			OrganizationID: org, ProjectID: project, Name: in.Name, Description: description, AllowedRefs: refs,
			AllowPullRequests: rules.AllowPullRequests, RequiredApprovals: int32(rules.RequiredApprovals), //nolint:gosec // RequiredApprovals is validated to be 1.
			AllowSelfApproval: rules.AllowSelfApproval, WaitMinutes: int32(rules.WaitMinutes), //nolint:gosec // WaitMinutes is validated to fit.
			ApprovalTimeoutHours: int32(rules.ApprovalTimeoutHours), //nolint:gosec // ApprovalTimeoutHours is validated to fit.
			RequirePinnedImages:  rules.RequirePinnedImages, AllowedPlugins: rules.AllowedPlugins, RequiredIsolation: rules.RequiredIsolation,
			AllowAdminBypass: rules.AllowAdminBypass, SupersedePending: rules.SupersedePending,
			AllowExtraDeployEgress: rules.AllowExtraDeployEgress, CreatedByUserID: p.UserID, Now: now,
		})
		if nameTaken(err) {
			return apperrors.EnvironmentExists()
		}
		if err != nil {
			return err
		}
		if err := insertReviewers(ctx, q, org, row.Id, rules.Reviewers, now); err != nil {
			return err
		}
		if out, err = s.one(ctx, q, row); err != nil {
			return err
		}
		data := auditData{"Version": versionOf(out)}
		data.rules(models.Rules{}, out.Rules, true)
		return s.audit(ctx, q, p, eventbus.EnvironmentCreated, out, data)
	})
	if err != nil {
		return models.Environment{}, err
	}
	return out, nil
}

func insertReviewers(ctx context.Context, q *sqlc.Queries, org, env uuid.UUID, r models.Reviewers, now time.Time) error {
	for _, id := range r.UserIDs {
		if err := q.InsertEnvironmentReviewer(ctx, sqlc.InsertEnvironmentReviewerParams{
			OrganizationID: org, EnvironmentID: env, UserID: &id, Now: now,
		}); err != nil {
			return err
		}
	}
	for _, id := range r.TeamIDs {
		if err := q.InsertEnvironmentReviewer(ctx, sqlc.InsertEnvironmentReviewerParams{
			OrganizationID: org, EnvironmentID: env, TeamID: &id, Now: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func checkReviewers(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, r models.Reviewers) error {
	var details []apperrors.IErrorDetail
	if len(r.UserIDs) > 0 {
		rows, err := q.ReviewerUserRoles(ctx, sqlc.ReviewerUserRolesParams{ProjectID: &project, OrganizationID: org, UserIds: r.UserIDs})
		if err != nil {
			return err
		}
		eligible := map[uuid.UUID]bool{}
		for _, row := range rows {
			role, ok := policy.ParseRole(row.Role)
			if !ok {
				continue
			}
			bindings := make([]policy.Role, 0, len(row.Bindings))
			for _, b := range row.Bindings {
				if br, ok := policy.ParseRole(b); ok {
					bindings = append(bindings, br)
				}
			}
			eligible[row.UserId] = policy.AtLeast(policy.ProjectRole(role, bindings), policy.RoleDeveloper)
		}
		for i, id := range r.UserIDs {
			if !eligible[id] {
				details = append(details, apperrors.IErrorDetail{Field: reviewerField("UserIds", i), Message: messageReviewerUser})
			}
		}
	}
	return teamDetails(ctx, q, org, r.TeamIDs, details)
}

func reviewerField(list string, i int) string {
	return fieldRules + ruleReviewers + "." + list + "[" + strconv.Itoa(i) + "]"
}

func teamDetails(ctx context.Context, q *sqlc.Queries, org uuid.UUID, teams []uuid.UUID, details []apperrors.IErrorDetail) error {
	if len(teams) > 0 {
		found, err := q.ReviewerTeams(ctx, sqlc.ReviewerTeamsParams{OrganizationID: org, TeamIds: teams})
		if err != nil {
			return err
		}
		for i, id := range teams {
			if !slices.Contains(found, id) {
				details = append(details, apperrors.IErrorDetail{Field: reviewerField("TeamIds", i), Message: messageReviewerTeam})
			}
		}
	}
	if len(details) > 0 {
		return apperrors.ReviewerNotEligible(details...)
	}
	return nil
}

func versionOf(e models.Environment) string { return strconv.FormatInt(e.RuleVersion, 10) }
