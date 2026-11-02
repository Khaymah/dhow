package runners

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners/Models"
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
	Keys      *runnerauth.Keys
	Bus       Publisher
	Origin    string
	PublicURL string
	Dev       bool
	Log       *slog.Logger
	Now       func() time.Time
}

type Service struct {
	db        DB
	keys      *runnerauth.Keys
	bus       Publisher
	origin    string
	publicURL string
	dev       bool
	log       *slog.Logger
	now       func() time.Time
}

var _ runnerauth.Verifier = (*Service)(nil)

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: o.DB, keys: o.Keys, bus: o.Bus, origin: o.Origin, publicURL: o.PublicURL, dev: o.Dev, log: o.Log, now: now}
}

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func (s *Service) Resolve(ctx context.Context, tokenHash []byte, now time.Time) (runnerauth.Principal, error) {
	q := s.queries()
	row, err := q.ResolveRunnerSession(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return runnerauth.Principal{}, runnerauth.ErrNoSession
	}
	if err != nil {
		return runnerauth.Principal{}, err
	}
	if row.State == models.StateRevoked {
		return runnerauth.Principal{}, runnerauth.ErrRevoked
	}
	if row.SessionRevokedAt != nil || !row.SessionExpiresAt.After(now) || row.SessionKeyVersion != row.KeyVersion {
		return runnerauth.Principal{}, runnerauth.ErrNoSession
	}
	if row.LastSeenAt == nil || now.Sub(*row.LastSeenAt) >= config.RunnerTouchInterval {
		touchErr := q.TouchRunner(ctx, sqlc.TouchRunnerParams{
			ID: row.Id, Now: now, IpAddress: runnerauth.RequestMetaFrom(ctx).IPString(), TouchBefore: now.Add(-config.RunnerTouchInterval),
		})
		if touchErr != nil {
			s.log.WarnContext(ctx, "touch runner failed", "error", touchErr)
		}
	}
	labels := append(append([]string{}, row.Labels...), row.CapabilityLabels...)
	return runnerauth.Principal{
		RunnerID: row.Id, OrganizationID: row.OrganizationId, SessionID: row.SessionId, Name: row.Name,
		Labels: labels, Capacity: int(row.Capacity), IsolationLevels: row.IsolationLevels,
		SigningPublicKey: row.SigningPublicKey, EncryptionPublicKey: row.EncryptionPublicKey, KeyVersion: row.KeyVersion,
		Disabled: row.State == models.StateDisabled, Features: row.Features,
	}, nil
}

func runnerOf(r sqlc.Runner, now time.Time) models.Runner {
	return models.Runner{
		ID: r.Id, OrganizationID: r.OrganizationId, Name: r.Name, Labels: r.Labels, CapabilityLabels: r.CapabilityLabels,
		Os: r.Os, Arch: r.Arch, Version: r.Version, IsolationLevels: r.IsolationLevels, Capacity: int(r.Capacity),
		State: r.State, Status: statusOf(r.State, r.LastSeenAt, now), DisabledReason: r.DisabledReason,
		KeyVersion: int(r.KeyVersion), KeysRotatedAt: r.KeysRotatedAt, LastSeenAt: r.LastSeenAt, CreatedAt: r.CreatedAt,
		RevokedAt: r.RevokedAt,
	}
}

func statusOf(state string, lastSeen *time.Time, now time.Time) string {
	switch {
	case state == models.StateRevoked:
		return models.StatusRevoked
	case state == models.StateDisabled:
		return models.StatusDisabled
	case lastSeen != nil && now.Sub(*lastSeen) < config.RunnerOnlineWindow:
		return models.StatusOnline
	default:
		return models.StatusOffline
	}
}

func tokenOf(t sqlc.RunnerRegistrationToken, now time.Time) models.RegistrationToken {
	status := models.TokenActive
	switch {
	case t.RevokedAt != nil:
		status = models.TokenRevoked
	case t.Uses >= t.MaxUses:
		status = models.TokenUsedUp
	case !t.ExpiresAt.After(now):
		status = models.TokenExpired
	}
	return models.RegistrationToken{
		ID: t.Id, Description: t.Description, Labels: t.Labels, MaxUses: int(t.MaxUses), Uses: int(t.Uses),
		ExpiresAt: t.ExpiresAt, Status: status, CreatedAt: t.CreatedAt, CreatedByUserID: t.CreatedByUserId,
		LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt,
	}
}
