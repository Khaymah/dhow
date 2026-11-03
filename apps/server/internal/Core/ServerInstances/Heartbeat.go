package serverinstances

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type Querier interface {
	RegisterServerInstance(ctx context.Context, arg sqlc.RegisterServerInstanceParams) (uuid.UUID, error)
	TouchServerInstance(ctx context.Context, id uuid.UUID) (int64, error)
	RetireServerInstance(ctx context.Context, id uuid.UUID) error
	RetireStaleServerInstances(ctx context.Context, staleAfterSeconds float64) (int64, error)
}

type Identity struct {
	Key           string
	Hostname      string
	Version       string
	Commit        string
	SchemaVersion int64
	KekVersions   []int32
}

type Heartbeat struct {
	q          Querier
	ident      Identity
	staleAfter time.Duration
	log        *slog.Logger

	mu sync.Mutex
	id uuid.UUID
}

func New(q Querier, ident Identity, staleAfter time.Duration, log *slog.Logger) *Heartbeat {
	return &Heartbeat{q: q, ident: ident, staleAfter: staleAfter, log: log}
}

func (h *Heartbeat) ID() uuid.UUID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.id
}

func (h *Heartbeat) Register(ctx context.Context) error {
	id, err := h.q.RegisterServerInstance(ctx, sqlc.RegisterServerInstanceParams{
		InstanceKey: h.ident.Key, Hostname: h.ident.Hostname, Version: h.ident.Version, Commit: h.ident.Commit, SchemaVersion: h.ident.SchemaVersion,
		KekVersions: kekVersions(h.ident.KekVersions),
	})
	if err != nil {
		return fmt.Errorf("register this server instance: %w", err)
	}
	h.mu.Lock()
	h.id = id
	h.mu.Unlock()
	return nil
}

func (h *Heartbeat) Beat(ctx context.Context) error {
	touched, err := h.q.TouchServerInstance(ctx, h.ID())
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	if touched == 0 {
		if err := h.Register(ctx); err != nil {
			return err
		}
	}
	if _, err := h.q.RetireStaleServerInstances(ctx, h.staleAfter.Seconds()); err != nil {
		return fmt.Errorf("retire stale server instances: %w", err)
	}
	return nil
}

func (h *Heartbeat) Retire(ctx context.Context) error {
	if err := h.q.RetireServerInstance(ctx, h.ID()); err != nil {
		return fmt.Errorf("retire this server instance: %w", err)
	}
	return nil
}

func (h *Heartbeat) Loop(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.Beat(ctx); err != nil && ctx.Err() == nil {
				h.log.WarnContext(ctx, "server heartbeat failed", "error", err)
			}
		}
	}
}

func kekVersions(v []int32) []int32 {
	if v == nil {
		return []int32{}
	}
	return v
}
