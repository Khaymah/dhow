package sources

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
)

var hookIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type Incoming struct {
	Provider string
	HookID   string
	Header   http.Header
	Body     []byte
}

type Hook struct {
	provider string
	repo     sqlc.Repository
	found    bool
	secret   []byte
}

func (h Hook) Known() bool { return h.found }

func (h Hook) ID() string { return h.repo.HookId.String }

func (s *Service) OpenHook(ctx context.Context, provider, hookID string) (Hook, error) {
	if _, ok := s.providers[provider]; !ok {
		return Hook{}, apperrors.WebhookRejected()
	}
	dummy, err := randomBytes(config.WebhookSecretBytes)
	if err != nil {
		return Hook{}, err
	}
	out := Hook{provider: provider, secret: dummy}
	if !hookIDPattern.MatchString(hookID) {
		return out, nil
	}
	row, err := s.queries().GetRepositoryByHook(ctx, sqlc.GetRepositoryByHookParams{HookID: text(hookID), Provider: provider})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	secret, err := s.openWebhookSecret(ctx, row.Repository.ProjectId, row.Repository.Id, row.Repository.WebhookSecretSealed)
	if err != nil {
		s.log.ErrorContext(ctx, "a webhook secret cannot be opened; check the keyring, then rotate the webhook secret",
			"repository_id", row.Repository.Id)
		return out, nil
	}
	return Hook{provider: provider, repo: row.Repository, found: true, secret: secret}, nil
}

func (s *Service) Receive(ctx context.Context, in Incoming) (models.Received, error) {
	h, err := s.OpenHook(ctx, in.Provider, in.HookID)
	if err != nil {
		return models.Received{}, err
	}
	return s.ReceiveFor(ctx, h, in.Header, in.Body)
}

func (s *Service) ReceiveFor(ctx context.Context, h Hook, header http.Header, body []byte) (models.Received, error) {
	provider, ok := s.providers[h.provider]
	if !ok {
		return models.Received{}, apperrors.WebhookRejected()
	}
	in := Incoming{Provider: h.provider, HookID: h.ID(), Header: header, Body: body}
	req := WebhookRequest{Header: in.Header, Body: in.Body, Secret: h.secret, Now: s.now()}
	proof, err := provider.VerifyWebhook(req)
	switch {
	case h.found && errors.Is(err, ErrDeliveryID):
		return models.Received{}, apperrors.ValidationFailed(apperrors.IErrorDetail{Field: "DeliveryId", Message: apperrors.KeyWebhookDeliveryIDMissing})
	case err != nil || !h.found:
		return models.Received{}, apperrors.WebhookRejected()
	}
	ev, err := provider.ParseEvent(req, proof)
	if err != nil {
		return models.Received{}, apperrors.ValidationFailed(apperrors.IErrorDetail{Field: "Payload", Message: messageInvalidValue})
	}
	digest := sha256.Sum256(in.Body)
	var out models.Received
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		var err error
		out, err = s.record(ctx, q, h.repo, in, proof, ev, digest[:], req)
		return err
	})
	return out, err
}

func (s *Service) outcome(ctx context.Context, q *sqlc.Queries, repo sqlc.Repository, ev models.Event) (string, error) {
	if ev.Ignore != "" {
		return ev.Ignore, nil
	}
	state, err := q.GetProjectState(ctx, sqlc.GetProjectStateParams{ProjectID: repo.ProjectId, OrganizationID: repo.OrganizationId})
	if err != nil {
		return "", err
	}
	if state != projectStateActive {
		return models.ReasonProjectArchived, nil
	}
	if repo.Transport == models.TransportSSH && !repo.HostKey.Valid {
		return models.ReasonHostKeyPending, nil
	}
	return "", nil
}

func (s *Service) record(
	ctx context.Context, q *sqlc.Queries, repo sqlc.Repository, in Incoming, proof Proof, ev models.Event, digest []byte, req WebhookRequest,
) (models.Received, error) {
	hookID := repo.HookId.String
	if err := q.LockWebhookHook(ctx, sqlc.LockWebhookHookParams{Provider: in.Provider, HookID: hookID}); err != nil {
		return models.Received{}, err
	}
	seen, err := q.WebhookDeliverySeen(ctx, sqlc.WebhookDeliverySeenParams{
		Provider: in.Provider, HookID: hookID, DeliveryID: proof.DeliveryID, DeliverySince: req.Now.Add(-config.WebhookDedupeWindow),
		PayloadDigest: digest, DigestSince: req.Now.Add(-config.WebhookDigestWindow),
	})
	if err != nil || seen {
		return models.Received{Duplicate: seen}, err
	}
	locked, err := q.LockRepositoryByProject(ctx, sqlc.LockRepositoryByProjectParams{ProjectID: repo.ProjectId, OrganizationID: repo.OrganizationId})
	if err != nil {
		return models.Received{}, err
	}
	reason, err := s.outcome(ctx, q, locked, ev)
	if err != nil {
		return models.Received{}, err
	}
	out := models.Received{Outcome: models.OutcomeTriggered, Reason: reason}
	if reason != "" {
		out.Outcome = models.OutcomeIgnored
	}
	row, err := q.InsertWebhookDelivery(ctx, sqlc.InsertWebhookDeliveryParams{
		Provider: in.Provider, HookID: hookID, DeliveryID: proof.DeliveryID, PayloadDigest: digest, OrganizationID: &repo.OrganizationId,
		RepositoryID: &repo.Id, Event: ev.Name, Ref: text(ev.Ref), Sha: text(ev.Sha), Outcome: out.Outcome, Reason: text(reason),
		Now: req.Now,
	})
	if err != nil {
		return models.Received{}, err
	}
	out.DeliveryID = row.Id
	if err := q.TouchRepositoryDelivery(ctx, sqlc.TouchRepositoryDeliveryParams{ID: repo.Id, Now: req.Now}); err != nil {
		return models.Received{}, err
	}
	if out.Outcome != models.OutcomeTriggered || s.triggers == nil {
		return out, nil
	}
	_, err = s.triggers.Deliver(ctx, q, triggers.Trigger{
		OrganizationID: repo.OrganizationId, ProjectID: repo.ProjectId, RepositoryID: repo.Id, DeliveryID: row.Id,
		Provider: in.Provider, Event: ev.Name, Ref: ev.Ref, Sha: ev.Sha, Actor: ev.Actor,
	})
	return out, err
}
