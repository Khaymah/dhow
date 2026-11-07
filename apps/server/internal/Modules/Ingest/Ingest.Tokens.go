package ingest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
	"github.com/Khaymah/dhow/internal/pathglob"
)

const (
	messageInvalid   = "errors.validationFailed"
	projectActive    = "active"
	day              = 24 * time.Hour
	uniqueViolation  = "23505"
	auditValueMax    = 256
	fieldRefPatterns = "RefPatterns"
	fieldExpires     = "ExpiresInDays"
	fieldAcknowledge = "AcknowledgeProtectedReach"
)

func invalid(field string) apperrors.IErrorDetail {
	return apperrors.IErrorDetail{Field: field, Message: messageInvalid}
}

func reachDetails(protected []string) []apperrors.IErrorDetail {
	details := []apperrors.IErrorDetail{invalid(fieldAcknowledge)}
	for _, name := range protected {
		details = append(details, apperrors.IErrorDetail{Field: apperrors.DetailProtectedReach, Message: name})
	}
	return details
}

func newTokenValue() (value string, hash []byte, prefix string, err error) {
	raw := make([]byte, config.IngestTokenRandomBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, "", fmt.Errorf("read random bytes for an ingest token: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	value = config.IngestTokenPrefix + body
	sum := sha256.Sum256([]byte(value))
	return value, sum[:], config.IngestTokenPrefix + body[:config.IngestTokenDisplayChars], nil
}

func hashOf(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func validPatterns(in []models.RefPattern) ([]models.RefPattern, bool) {
	if len(in) == 0 || len(in) > config.IngestMaxPatterns {
		return nil, false
	}
	out := make([]models.RefPattern, 0, len(in))
	seen := map[models.RefPattern]bool{}
	for _, p := range in {
		if (p.Kind != models.RefKindBranch && p.Kind != models.RefKindTag) || len(p.Pattern) > config.IngestPatternMax ||
			!pathglob.ValidPattern(p.Pattern) {
			return nil, false
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, true
}

func validateToken(in models.CreateTokenInput) ([]models.RefPattern, int, *apperrors.AppError) {
	var details []apperrors.IErrorDetail
	if !auth.ValidDisplayName(in.Name, config.IngestTokenNameMax) {
		details = append(details, invalid(fieldName))
	}
	patterns, ok := validPatterns(in.RefPatterns)
	if !ok {
		details = append(details, invalid(fieldRefPatterns))
	}
	days := in.ExpiresInDays
	if days == 0 {
		days = config.IngestTokenDefaultDays
	}
	if days < 1 || days > config.IngestTokenMaxDays {
		details = append(details, invalid(fieldExpires))
	}
	if len(details) > 0 {
		return nil, 0, apperrors.ValidationFailed(details...)
	}
	return patterns, days, nil
}

func statusOf(row sqlc.IngestToken, now time.Time) string {
	switch {
	case row.RevokedAt != nil:
		return models.StatusRevoked
	case !now.Before(row.ExpiresAt):
		return models.StatusExpired
	default:
		return models.StatusActive
	}
}

func patternsOf(raw []byte) ([]models.RefPattern, error) {
	var out []models.RefPattern
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode the ref patterns of an ingest token: %w", err)
	}
	return out, nil
}

func (s *Service) tokenOf(row sqlc.IngestToken) (models.Token, error) {
	patterns, err := patternsOf(row.RefPatterns)
	if err != nil {
		return models.Token{}, err
	}
	return models.Token{
		ID: row.Id, ProjectID: row.ProjectId, Name: row.Name, Prefix: row.Prefix, RefPatterns: patterns,
		Status: statusOf(row, s.now()), CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt,
		RevokedAt: row.RevokedAt, CreatedByUserID: row.CreatedByUserId,
	}, nil
}

func cut(v string) string {
	if len(v) <= auditValueMax {
		return v
	}
	return strings.ToValidUTF8(v[:auditValueMax], "")
}

func describe(patterns []models.RefPattern) string {
	parts := make([]string, 0, len(patterns))
	for _, p := range patterns {
		parts = append(parts, p.Kind+":"+p.Pattern)
	}
	return cut(strings.Join(parts, " "))
}

func (s *Service) requireActiveProject(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID) error {
	row, err := q.IngestTargetProject(ctx, sqlc.IngestTargetProjectParams{ProjectID: project, OrganizationID: org})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.NotFound()
	case err != nil:
		return err
	case row.State != projectActive:
		return apperrors.ProjectArchived()
	}
	return nil
}

func (s *Service) CreateToken(
	ctx context.Context, org, project uuid.UUID, in models.CreateTokenInput, factor auth.Factor,
) (models.CreatedToken, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.CreatedToken{}, err
	}
	if p.ViaToken() {
		return models.CreatedToken{}, apperrors.SessionRequired()
	}
	patterns, days, ae := validateToken(in)
	if ae != nil {
		return models.CreatedToken{}, ae
	}
	if err := s.factors.ConfirmFactor(ctx, p, factor); err != nil {
		return models.CreatedToken{}, err
	}
	value, hash, prefix, err := newTokenValue()
	if err != nil {
		return models.CreatedToken{}, err
	}
	raw, err := json.Marshal(patterns)
	if err != nil {
		return models.CreatedToken{}, fmt.Errorf("encode the ref patterns of an ingest token: %w", err)
	}
	now := s.now()
	var row sqlc.IngestToken
	var reaches []string
	var protected []reachEnv
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := q.LockIngestTokens(ctx, project.String()); err != nil {
			return err
		}
		if err := s.requireActiveProject(ctx, q, org, project); err != nil {
			return err
		}
		if err := s.checkRoom(ctx, q, org, project, in.Name, now); err != nil {
			return err
		}
		envs, err := s.environmentsOf(ctx, q, org, project)
		if err != nil {
			return err
		}
		reaches, protected = reachOf(patterns, envs)
		if len(protected) > 0 && !in.AcknowledgeProtectedReach {
			return apperrors.IngestReachUnconfirmed(reachDetails(namesOf(protected))...)
		}
		if len(protected) > 0 && !canAcknowledge(p) {
			return apperrors.IngestReachNeedsAdmin()
		}
		row, err = q.InsertIngestToken(ctx, sqlc.InsertIngestTokenParams{
			OrganizationID: org, ProjectID: project, Name: in.Name, TokenHash: hash, Prefix: prefix, RefPatterns: raw,
			ExpiresAt: now.Add(time.Duration(days) * day), CreatedByUserID: p.UserID, AcknowledgedEnvironmentIds: idsOf(protected),
		})
		if isUnique(err) {
			return apperrors.IngestTokenNameTaken()
		}
		if err != nil {
			return err
		}
		actor := p.UserID
		return s.audit(ctx, auditEntry{
			name: eventbus.IngestTokenCreated, org: org, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(), target: targetIngestToken,
			targetID: row.Id, tx: q, data: map[string]string{
				"Name": row.Name, "ProjectId": project.String(), "Patterns": describe(patterns), "ExpiresInDays": strconv.Itoa(days),
			},
		})
	})
	if err != nil {
		return models.CreatedToken{}, err
	}
	token, err := s.tokenOf(row)
	token.Reaches = reaches
	token.CreatorAllowed = true
	token.UnacknowledgedReach = []string{}
	return models.CreatedToken{Token: token, Value: value}, err
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == uniqueViolation
}

func (s *Service) checkRoom(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, name string, now time.Time) error {
	taken, err := q.IngestTokenNameTaken(ctx, sqlc.IngestTokenNameTakenParams{ProjectID: project, OrganizationID: org, Name: name})
	if err != nil {
		return err
	}
	if taken {
		return apperrors.IngestTokenNameTaken()
	}
	active, err := q.CountActiveIngestTokens(ctx, sqlc.CountActiveIngestTokensParams{OrganizationID: org, ProjectID: project, Now: now})
	if err != nil {
		return err
	}
	if active >= config.IngestMaxTokensPerProject {
		return apperrors.IngestTokenLimit()
	}
	return nil
}

func (s *Service) ListTokens(ctx context.Context, org, project uuid.UUID, page, limit int) (models.TokenPage, error) {
	q := s.queries()
	total, err := q.CountIngestTokens(ctx, sqlc.CountIngestTokensParams{OrganizationID: org, ProjectID: project})
	if err != nil {
		return models.TokenPage{}, err
	}
	rows, err := q.ListIngestTokens(ctx, sqlc.ListIngestTokensParams{
		OrganizationID: org, ProjectID: project, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // Bounded by the page parameters.
	})
	if err != nil {
		return models.TokenPage{}, err
	}
	items, err := s.viewsOf(ctx, q, org, project, rows)
	if err != nil {
		return models.TokenPage{}, err
	}
	return models.TokenPage{Items: items, Total: total}, nil
}

func (s *Service) RevokeToken(ctx context.Context, org, project, id uuid.UUID, factor auth.Factor) (models.Token, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Token{}, err
	}
	if p.ViaToken() {
		return models.Token{}, apperrors.SessionRequired()
	}
	if err := s.factors.ConfirmFactor(ctx, p, factor); err != nil {
		return models.Token{}, err
	}
	var row sqlc.IngestToken
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if _, err := q.GetIngestToken(ctx, sqlc.GetIngestTokenParams{ID: id, OrganizationID: org, ProjectID: project}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.NotFound()
			}
			return err
		}
		actor := p.UserID
		now := s.now()
		var err error
		row, err = q.RevokeIngestToken(ctx, sqlc.RevokeIngestTokenParams{
			ID: id, OrganizationID: org, ProjectID: project, Now: &now, RevokedByUserID: &actor,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			row, err = q.GetIngestToken(ctx, sqlc.GetIngestTokenParams{ID: id, OrganizationID: org, ProjectID: project})
			return err
		}
		if err != nil {
			return err
		}
		return s.audit(ctx, auditEntry{
			name: eventbus.IngestTokenRevoked, org: org, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString(), target: targetIngestToken,
			targetID: row.Id, tx: q, data: map[string]string{"Name": row.Name, "ProjectId": project.String()},
		})
	})
	if err != nil {
		return models.Token{}, err
	}
	views, err := s.viewsOf(ctx, s.queries(), org, project, []sqlc.IngestToken{row})
	if err != nil {
		return models.Token{}, err
	}
	return views[0], nil
}
