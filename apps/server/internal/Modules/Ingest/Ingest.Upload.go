package ingest

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
	"github.com/Khaymah/dhow/internal/pathglob"
)

const (
	digestParam         = "sha256"
	fieldDigest         = "Sha256"
	bearerScheme        = "bearer"
	messageStored       = "ingest.artifact.stored"
	messageDuplicate    = "ingest.artifact.duplicate"
	messageStatus       = "ingest.artifact.status"
	limitStatusPerToken = "ingest-status"
	statusPerTokenMax   = 600
	statusLimitRows     = 20
	tokenBodyLength     = 43
)

type tokenAuth struct {
	row      sqlc.FindIngestTokenByHashRow
	patterns []models.RefPattern
}

func bearerOf(header string) (string, bool) {
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, bearerScheme) {
		return "", false
	}
	valid := strings.HasPrefix(value, config.IngestTokenPrefix) && len(value) == len(config.IngestTokenPrefix)+tokenBodyLength
	return value, valid
}

func (s *Service) authenticate(ctx context.Context, header string) (tokenAuth, error) {
	value, wellFormed := bearerOf(header)
	hash := hashOf(value)
	if !wellFormed {
		return tokenAuth{}, apperrors.IngestTokenInvalid()
	}
	row, err := s.queries().FindIngestTokenByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && subtle.ConstantTimeCompare(row.TokenHash, hash) != 1) {
		return tokenAuth{}, apperrors.IngestTokenInvalid()
	}
	if err != nil {
		return tokenAuth{}, err
	}
	now := s.now()
	switch {
	case row.RevokedAt != nil:
		return tokenAuth{}, apperrors.IngestTokenRevoked()
	case !now.Before(row.ExpiresAt):
		return tokenAuth{}, apperrors.IngestTokenExpired()
	case row.ProjectState != projectActive:
		return tokenAuth{}, apperrors.ProjectArchived()
	}
	patterns, err := patternsOf(row.RefPatterns)
	if err != nil {
		return tokenAuth{}, err
	}
	if err := s.queries().TouchIngestToken(ctx, sqlc.TouchIngestTokenParams{
		ID: row.Id, Now: &now, StaleBefore: ptrOf(now.Add(-config.IngestLastUsedEvery)),
	}); err != nil {
		s.log.WarnContext(ctx, "the last use of an ingest token was not recorded", "token_id", row.Id.String(), "error", err)
	}
	return tokenAuth{row: row, patterns: patterns}, nil
}

func allows(patterns []models.RefPattern, m Meta) bool {
	for _, p := range patterns {
		if p.Kind == m.RefKind && pathglob.MatchPattern(p.Pattern, m.RefName) {
			return true
		}
	}
	return false
}

func headerFailure(err error) error {
	var he *HeaderError
	if errors.As(err, &he) {
		return apperrors.ValidationFailed(invalid(he.Field))
	}
	return err
}

func (s *Service) Put(w http.ResponseWriter, r *http.Request) {
	var tok *tokenAuth
	res, err := s.put(w, r, &tok)
	if err != nil {
		s.fail(w, r, tok, err)
		return
	}
	status, message := http.StatusCreated, messageStored
	if !res.Created {
		status, message = http.StatusOK, messageDuplicate
	}
	response.Success(w, status, receivedView(res), message, nil)
}

func (s *Service) put(w http.ResponseWriter, r *http.Request, got **tokenAuth) (models.Received, error) {
	ctx := r.Context()
	network := ratelimit.Network(middleware.ClientIPFrom(ctx))
	if err := s.take(ctx, ratelimit.Limit{Name: limitPerIP, Max: config.IngestPerIPMax, Window: config.IngestPerIPWindow}, network); err != nil {
		return models.Received{}, err
	}
	tok, err := s.authenticate(ctx, r.Header.Get("Authorization"))
	if err != nil {
		return models.Received{}, err
	}
	*got = &tok
	limit := ratelimit.Limit{Name: limitPerToken, Max: config.IngestPerTokenMax, Window: config.IngestPerTokenWindow}
	if err := s.take(ctx, limit, tok.row.Id.String()); err != nil {
		return models.Received{}, err
	}
	d, err := blob.ParseDigest(chi.URLParam(r, digestParam))
	if err != nil {
		return models.Received{}, apperrors.ValidationFailed(invalid(fieldDigest))
	}
	meta, err := ParseHeaders(r.Header)
	if err != nil {
		return models.Received{}, headerFailure(err)
	}
	if !allows(tok.patterns, meta) {
		return models.Received{}, apperrors.RefNotAllowedForToken()
	}
	switch {
	case r.ContentLength < 0:
		return models.Received{}, apperrors.LengthRequired()
	case r.ContentLength > s.cfg.IngestMaxBytes:
		return models.Received{}, apperrors.ArtifactTooLarge()
	}
	if known, found, err := s.known(ctx, s.queries(), tok, meta, d); err != nil || found {
		return known, err
	}
	if err := s.checkQuota(ctx, s.queries(), tok.row.OrganizationId, r.ContentLength); err != nil {
		return models.Received{}, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return models.Received{}, apperrors.IngestBusy(config.IngestBusyRetry)
	}
	return s.receive(ctx, w, r, tok, meta, d)
}

func (s *Service) checkQuota(ctx context.Context, q *sqlc.Queries, org uuid.UUID, extra int64) error {
	used, err := q.IngestOrganizationBytes(ctx, org)
	if err != nil {
		return err
	}
	if used+extra > s.cfg.OrgMaxBytes {
		return apperrors.ArtifactQuotaExceeded()
	}
	return nil
}

func (s *Service) known(ctx context.Context, q *sqlc.Queries, tok tokenAuth, m Meta, d blob.Digest) (models.Received, bool, error) {
	row, err := q.FindIngestedArtifact(ctx, sqlc.FindIngestedArtifactParams{
		ProjectID: tok.row.ProjectId, OrganizationID: tok.row.OrganizationId, Digest: d[:], Name: m.Name, Ref: m.Ref, Sha: m.Sha,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Received{}, false, nil
	}
	if err != nil {
		return models.Received{}, false, err
	}
	out, err := s.resultOf(ctx, q, row, false)
	return out, true, err
}

func (s *Service) resultOf(ctx context.Context, q *sqlc.Queries, row sqlc.IngestedArtifact, created bool) (models.Received, error) {
	out := models.Received{
		ArtifactID: row.Id, Name: row.Name, Digest: hexOf(row.Digest), SizeBytes: row.SizeBytes, Created: created,
	}
	run, err := q.IngestedArtifactRun(ctx, sqlc.IngestedArtifactRunParams{IngestedArtifactID: row.Id, OrganizationID: row.OrganizationId})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Triggered = true
	out.Run = &models.Run{ID: run.RunId, Number: run.Number, State: run.State}
	out.RunURL = s.runURL(row.ProjectId, run.Number)
	return out, nil
}

func (s *Service) fail(w http.ResponseWriter, r *http.Request, tok *tokenAuth, err error) {
	ae := apperrors.From(err)
	ctx := r.Context()
	if ae.Status >= http.StatusInternalServerError && ae.Status != http.StatusServiceUnavailable {
		s.log.ErrorContext(ctx, "an ingest request failed", "request_id", middleware.RequestIDFrom(ctx), "error", ae)
	}
	if tok != nil && ae.Status < http.StatusInternalServerError && ae.Status != http.StatusTooManyRequests {
		s.auditRejected(ctx, *tok, ae.Code)
	}
	response.Error(w, r, ae)
}

func (s *Service) auditRejected(ctx context.Context, tok tokenAuth, code string) {
	limit := ratelimit.Limit{Name: limitRejectAudit, Max: 1, Window: config.IngestRejectAuditEvery}
	if err := s.take(context.WithoutCancel(ctx), limit, tok.row.Id.String()); err != nil {
		return
	}
	err := s.audit(context.WithoutCancel(ctx), auditEntry{
		name: eventbus.IngestRejected, org: tok.row.OrganizationId, target: targetIngestToken, targetID: tok.row.Id,
		data: map[string]string{"RejectCode": code, "Name": tok.row.Name, "ProjectId": tok.row.ProjectId.String()},
	})
	if err != nil {
		s.log.ErrorContext(ctx, "an ingest rejection was not recorded", "token_id", tok.row.Id.String(), "error", err)
	}
}

func (s *Service) Status(w http.ResponseWriter, r *http.Request) {
	var tok *tokenAuth
	list, err := s.status(r, &tok)
	if err != nil {
		s.fail(w, r, tok, err)
		return
	}
	response.Success(w, http.StatusOK, list, messageStatus, nil)
}

func (s *Service) status(r *http.Request, got **tokenAuth) ([]receivedJSON, error) {
	ctx := r.Context()
	network := ratelimit.Network(middleware.ClientIPFrom(ctx))
	if err := s.take(ctx, ratelimit.Limit{Name: limitPerIP, Max: config.IngestPerIPMax, Window: config.IngestPerIPWindow}, network); err != nil {
		return nil, err
	}
	tok, err := s.authenticate(ctx, r.Header.Get("Authorization"))
	if err != nil {
		return nil, err
	}
	*got = &tok
	if err := s.take(ctx, ratelimit.Limit{Name: limitStatusPerToken, Max: statusPerTokenMax, Window: config.IngestPerTokenWindow}, tok.row.Id.String()); err != nil {
		return nil, err
	}
	d, err := blob.ParseDigest(chi.URLParam(r, digestParam))
	if err != nil {
		return nil, apperrors.ValidationFailed(invalid(fieldDigest))
	}
	q := s.queries()
	rows, err := q.ListIngestedByDigest(ctx, sqlc.ListIngestedByDigestParams{
		ProjectID: tok.row.ProjectId, OrganizationID: tok.row.OrganizationId, Digest: d[:], MaxRows: statusLimitRows,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, apperrors.NotFound()
	}
	out := make([]receivedJSON, 0, len(rows))
	for _, row := range rows {
		res, err := s.resultOf(ctx, q, row, false)
		if err != nil {
			return nil, err
		}
		view := receivedView(res)
		view.Ref, view.Sha = row.Ref, row.Sha
		out = append(out, view)
	}
	return out, nil
}
