package sources

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	targetProject  = "project"
	contextDeploy  = "deploy-key:"
	contextWebhook = "webhook-secret:"
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Options struct {
	DB        DB
	Bus       Publisher
	Log       *slog.Logger
	Now       func() time.Time
	Values    *keys.Values
	PublicURL string
	Triggers  triggers.Sink
	Dialer    netguard.Dialer
	Limiter   ratelimit.Limiter
	Providers map[string]Provider
}

type Service struct {
	db         DB
	bus        Publisher
	log        *slog.Logger
	now        func() time.Time
	publicURL  string
	triggers   triggers.Sink
	dialer     netguard.Dialer
	limiter    ratelimit.Limiter
	deployKeys *keys.Column
	webhooks   *keys.Column
	providers  map[string]Provider
}

func NewService(o Options) (*Service, error) {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	if o.Values == nil {
		return nil, errors.New("the sources service needs the keyring to seal deploy keys and webhook secrets")
	}
	deployKeys, err := o.Values.Column(DeployKeySpec())
	if err != nil {
		return nil, err
	}
	webhooks, err := o.Values.Column(WebhookSecretSpec())
	if err != nil {
		return nil, err
	}
	s := &Service{
		db: o.DB, bus: o.Bus, log: o.Log, now: now, publicURL: strings.TrimSuffix(o.PublicURL, "/"), triggers: o.Triggers,
		dialer: o.Dialer, limiter: o.Limiter, deployKeys: deployKeys, webhooks: webhooks, providers: map[string]Provider{},
	}
	s.providers[models.ProviderGeneric] = genericProvider{keys: s}
	for name, p := range o.Providers {
		s.providers[name] = p
	}
	return s, nil
}

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func text(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func sealContext(kind string, repo uuid.UUID) []byte { return []byte(kind + repo.String()) }

func (s *Service) WebhookURL(hookID string) string {
	return s.publicURL + config.WebhookPrefix + "/git/" + hookID
}

func (s *Service) repositoryOf(row sqlc.Repository) models.Repository {
	r := models.Repository{
		ID: row.Id, OrganizationID: row.OrganizationId, ProjectID: row.ProjectId, Provider: row.Provider, CloneURL: row.CloneUrl,
		Transport: row.Transport, FullName: row.FullName, DefaultBranch: row.DefaultBranch, HookID: row.HookId.String,
		DeployKeyPublic: row.DeployKeyPublic.String, PreviousDeployKeyPublic: row.PreviousDeployKeyPublic.String,
		PreviousDeployKeyExpiresAt: row.PreviousDeployKeyExpiresAt, HostKey: row.HostKey.String, LastDeliveryAt: row.LastDeliveryAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, State: models.StateReady,
	}
	r.HostKeyFingerprint = fingerprintOf(row.HostKey.String)
	r.PendingHostKeyFingerprint = fingerprintOf(row.PendingHostKey.String)
	if r.Transport == models.TransportSSH && r.HostKey == "" {
		r.State = models.StateHostKeyUnconfirmed
	}
	return r
}

func (s *Service) audit(ctx context.Context, p auth.Principal, name string, repo models.Repository, data map[string]string) error {
	actor := p.UserID
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{repo.OrganizationID}, ActorUserID: &actor, TargetType: targetProject,
		TargetID: repo.ProjectID.String(), IP: auth.RequestMetaFrom(ctx).IPString(), Data: data,
	}})
}

func (s *Service) openDeployKey(ctx context.Context, project, repo uuid.UUID, sealed []byte) ([]byte, error) {
	if len(sealed) == 0 {
		return nil, errors.New("the repository has no deploy key")
	}
	return s.deployKeys.Open(ctx, keys.ProjectOwner(project), sealContext(contextDeploy, repo), sealed)
}

func (s *Service) openWebhookSecret(ctx context.Context, project, repo uuid.UUID, sealed []byte) ([]byte, error) {
	return s.webhooks.Open(ctx, keys.ProjectOwner(project), sealContext(contextWebhook, repo), sealed)
}
