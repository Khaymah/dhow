package projects

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Projects/Models"
)

const (
	targetProject   = "project"
	uniqueViolation = "23505"
	constraintSlug  = "UX_Projects_Organization_Slug"
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Options struct {
	DB  DB
	Bus Publisher
	Log *slog.Logger
	Now func() time.Time
}

type Service struct {
	db  DB
	bus Publisher
	log *slog.Logger
	now func() time.Time
}

var _ auth.ProjectResolver = (*Service)(nil)

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: o.DB, bus: o.Bus, log: o.Log, now: now}
}

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func projectOf(row sqlc.Project) models.Project {
	return models.Project{
		ID: row.Id, OrganizationID: row.OrganizationId, Slug: row.Slug, Name: row.Name, PipelinePath: row.PipelinePath,
		ForkPolicy: row.ForkPolicy, State: row.State, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ArchivedAt: row.ArchivedAt,
	}
}

func slugTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraintSlug
}

func conflictOnSlug() *apperrors.AppError {
	e := apperrors.Conflict()
	e.Details = []apperrors.IErrorDetail{{Field: fieldSlug, Message: messageSlugTaken}}
	return e
}

func (s *Service) ProjectBindings(ctx context.Context, org, project, user uuid.UUID) ([]policy.Role, error) {
	row, err := s.queries().GetProjectBindings(ctx, sqlc.GetProjectBindingsParams{ProjectID: project, OrganizationID: org, UserID: &user})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, auth.ErrNoProject
	}
	if err != nil {
		return nil, err
	}
	roles := make([]policy.Role, 0, len(row.Roles))
	for _, raw := range row.Roles {
		if r, ok := policy.ParseRole(raw); ok {
			roles = append(roles, r)
		}
	}
	return roles, nil
}

func (s *Service) List(ctx context.Context, org uuid.UUID, state string, page, limit int) (models.Page, error) {
	q := s.queries()
	total, err := q.CountProjects(ctx, sqlc.CountProjectsParams{OrganizationID: org, State: state})
	if err != nil {
		return models.Page{}, err
	}
	rows, err := q.ListProjects(ctx, sqlc.ListProjectsParams{
		OrganizationID: org, State: state, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return models.Page{}, err
	}
	out := models.Page{Items: make([]models.Project, 0, len(rows)), Total: total}
	for _, r := range rows {
		out.Items = append(out.Items, projectOf(r))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, org, id uuid.UUID) (models.Project, error) {
	row, err := s.queries().GetProject(ctx, sqlc.GetProjectParams{ID: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Project{}, apperrors.NotFound()
	}
	if err != nil {
		return models.Project{}, err
	}
	return projectOf(row), nil
}

func (s *Service) Create(ctx context.Context, org uuid.UUID, in models.CreateInput) (models.Project, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Project{}, err
	}
	in, ae := validateCreate(in)
	if ae != nil {
		return models.Project{}, ae
	}
	if *in.ForkPolicy == models.ForkPolicyAutomatic {
		if role, _ := p.RoleIn(org); !policy.AtLeast(role, policy.RoleAdmin) {
			return models.Project{}, apperrors.ForkPolicyNotAllowed()
		}
	}
	row, err := s.queries().InsertProject(ctx, sqlc.InsertProjectParams{
		OrganizationID: org, Slug: in.Slug, Name: in.Name, PipelinePath: *in.PipelinePath, ForkPolicy: *in.ForkPolicy,
		CreatedByUserID: p.UserID, Now: s.now(),
	})
	if slugTaken(err) {
		return models.Project{}, conflictOnSlug()
	}
	if err != nil {
		return models.Project{}, err
	}
	project := projectOf(row)
	return project, s.audit(ctx, p, eventbus.ProjectCreated, project, map[string]string{
		"Name": project.Name, "Slug": project.Slug, "PipelinePath": project.PipelinePath, "ForkPolicy": project.ForkPolicy,
	})
}

func (s *Service) audit(ctx context.Context, p auth.Principal, name string, project models.Project, data map[string]string) error {
	actor := p.UserID
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{project.OrganizationID}, ActorUserID: &actor, TargetType: targetProject,
		TargetID: project.ID.String(), IP: auth.RequestMetaFrom(ctx).IPString(), Data: data,
	}})
}

func (s *Service) Linked(ctx context.Context, org uuid.UUID, ids ...uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := s.queries().LinkedProjects(ctx, sqlc.LinkedProjectsParams{OrganizationID: org, ProjectIds: ids})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]bool, len(rows))
	for _, id := range rows {
		out[id] = true
	}
	return out, nil
}
