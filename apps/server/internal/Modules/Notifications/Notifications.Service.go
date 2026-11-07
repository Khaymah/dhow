package notifications

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	targetChannel = "notification_channel"
	targetRule    = "notification_rules"
	targetTest    = "notification_test"
)

var (
	ErrNoValues  = errors.New("the notification service needs the sealed values")
	ErrNoFactors = errors.New("the notification service needs a factor confirmer")
	ErrNoPool    = errors.New("the notification service needs a database pool")
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Subscriber interface {
	Subscribe(name string, h eventbus.Handler)
}

type Options struct {
	DB        DB
	Pool      *pgxpool.Pool
	Values    *keys.Values
	Factors   auth.FactorConfirmer
	Bus       Publisher
	Mailer    mailer.Sender
	Config    config.Notify
	PublicURL string
	TLS       *tls.Config
	Lookup    netguard.Lookup
	Dial      netguard.DialFunc
	Log       *slog.Logger
	Now       func() time.Time
}

type Service struct {
	db        DB
	pool      *pgxpool.Pool
	column    *keys.Column
	factors   auth.FactorConfirmer
	bus       Publisher
	mailer    mailer.Sender
	cfg       config.Notify
	publicURL string
	tls       *tls.Config
	lookup    netguard.Lookup
	dial      netguard.DialFunc
	log       *slog.Logger
	now       func() time.Time
	worker    *outbox.Worker
}

func NewService(o Options) (*Service, error) {
	if o.DB == nil {
		return nil, ErrNoPool
	}
	if o.Values == nil {
		return nil, ErrNoValues
	}
	if o.Factors == nil {
		return nil, ErrNoFactors
	}
	column, err := o.Values.Column(ConfigSpec())
	if err != nil {
		return nil, err
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	cfg := o.Config
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = config.NotifyRetentionDefault
	}
	m := o.Mailer
	if m == nil {
		m = mailer.Disabled{}
	}
	s := &Service{
		db: o.DB, pool: o.Pool, column: column, factors: o.Factors, bus: o.Bus, mailer: m, cfg: cfg,
		publicURL: strings.TrimSuffix(o.PublicURL, "/"), tls: o.TLS, lookup: o.Lookup, dial: o.Dial, log: o.Log, now: now,
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
