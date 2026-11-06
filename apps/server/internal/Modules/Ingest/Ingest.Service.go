package ingest

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	limitPerIP        = "ingest-ip"
	limitPerToken     = "ingest-token"
	limitRejectAudit  = "ingest-reject"
	targetIngestToken = "ingest_token"
	targetArtifact    = "ingested_artifact"
)

var (
	ErrNoStore   = errors.New("the ingest service needs a blob store")
	ErrNoLimiter = errors.New("the ingest service needs a rate limiter")
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
	Store     blob.Store
	Triggers  triggers.Sink
	Bus       Publisher
	Factors   auth.FactorConfirmer
	Limiter   ratelimit.Limiter
	Config    config.Artifacts
	PublicURL string
	Log       *slog.Logger
	Now       func() time.Time
}

type Service struct {
	db        DB
	store     blob.Store
	triggers  triggers.Sink
	bus       Publisher
	factors   auth.FactorConfirmer
	limiter   ratelimit.Limiter
	cfg       config.Artifacts
	publicURL string
	log       *slog.Logger
	now       func() time.Time
	slots     chan struct{}
	inflight  *inflight
}

func NewService(o Options) (*Service, error) {
	if o.Store == nil {
		return nil, ErrNoStore
	}
	if o.Limiter == nil {
		return nil, ErrNoLimiter
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	cfg := o.Config
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = config.ArtifactMaxBytesDefault
	}
	if cfg.IngestMaxBytes <= 0 {
		cfg.IngestMaxBytes = cfg.MaxBytes
	}
	if cfg.OrgMaxBytes <= 0 {
		cfg.OrgMaxBytes = config.ArtifactOrgMaxBytesDefault
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = config.ArtifactRetentionDefault
	}
	return &Service{
		db: o.DB, store: o.Store, triggers: o.Triggers, bus: o.Bus, factors: o.Factors, limiter: o.Limiter, cfg: cfg,
		publicURL: strings.TrimSuffix(o.PublicURL, "/"), log: o.Log, now: now, slots: make(chan struct{}, config.IngestConcurrent), inflight: newInflight(),
	}, nil
}

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func (s *Service) take(ctx context.Context, limit ratelimit.Limit, subject string) error {
	d, err := s.limiter.Take(ctx, limit, subject)
	if err != nil {
		return err
	}
	return d.Err()
}

func (s *Service) runURL(project uuid.UUID, number int64) string {
	if s.publicURL == "" {
		return ""
	}
	return s.publicURL + "/projects/" + project.String() + "/runs/" + strconv.FormatInt(number, 10)
}

type auditEntry struct {
	name     string
	org      uuid.UUID
	actor    *uuid.UUID
	ip       string
	target   string
	targetID uuid.UUID
	data     map[string]string
	tx       *sqlc.Queries
}

func (s *Service) audit(ctx context.Context, e auditEntry) error {
	if s.bus == nil {
		return nil
	}
	return s.bus.Publish(ctx, eventbus.Event{Name: e.name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{e.org}, ActorUserID: e.actor, TargetType: e.target, TargetID: e.targetID.String(),
		IP: e.ip, Data: e.data, Tx: e.tx,
	}})
}
