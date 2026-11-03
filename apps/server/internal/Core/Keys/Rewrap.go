package keys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const RewrapBatch = 500

type RewrapResult struct {
	Rewrapped int
	Retired   []int32
	Skipped   bool
}

func (m *Manager) Rewrap(ctx context.Context) (RewrapResult, error) {
	var total RewrapResult
	for {
		batch, err := m.rewrapBatch(ctx)
		if err != nil {
			return total, err
		}
		total.Rewrapped += batch.Rewrapped
		total.Retired = append(total.Retired, batch.Retired...)
		if batch.Skipped {
			total.Skipped = true
			return total, nil
		}
		if batch.Rewrapped == 0 {
			return total, nil
		}
	}
}

func (m *Manager) rewrapBatch(ctx context.Context) (RewrapResult, error) {
	var res RewrapResult
	err := dbtx.Run(ctx, m.db, func(q *sqlc.Queries) error {
		locked, err := q.TryLockKeyRewrap(ctx)
		if err != nil {
			return fmt.Errorf("lock the re-wrap: %w", err)
		}
		if !locked {
			res.Skipped = true
			return nil
		}
		active, err := q.GetActiveKeyEncryptionKey(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read the active KEK version: %w", err)
		}
		rows, err := q.ListDataKeysToRewrap(ctx, sqlc.ListDataKeysToRewrapParams{ActiveVersion: active, Batch: RewrapBatch})
		if err != nil {
			return fmt.Errorf("read the data keys to re-wrap: %w", err)
		}
		for _, r := range rows {
			if err := m.rewrapOne(ctx, q, r, active); err != nil {
				return err
			}
		}
		res.Rewrapped = len(rows)
		if len(rows) > 0 {
			return nil
		}
		res.Retired, err = q.RetireDrainedKeyEncryptionKeys(ctx)
		if err != nil {
			return fmt.Errorf("retire drained KEK versions: %w", err)
		}
		return nil
	})
	return res, err
}

func (m *Manager) rewrapOne(ctx context.Context, q *sqlc.Queries, r sqlc.ListDataKeysToRewrapRow, active int32) error {
	owner := ownerFrom(r.OwnerKind, r.OwnerId)
	dek, err := m.provider.Unwrap(ctx, r.KekVersion, r.Wrapped, dekAAD(r.Id, owner, r.KekVersion))
	if err != nil {
		return fmt.Errorf("unwrap data key %s with KEK version %d: %w", r.Id, r.KekVersion, err)
	}
	defer clear(dek)
	wrapped, err := m.provider.Wrap(ctx, active, dek, dekAAD(r.Id, owner, active))
	if err != nil {
		return fmt.Errorf("wrap data key %s with KEK version %d: %w", r.Id, active, err)
	}
	n, err := q.RewrapDataKey(ctx, sqlc.RewrapDataKeyParams{KekVersion: active, Wrapped: wrapped, ID: r.Id, FromVersion: r.KekVersion})
	if err != nil {
		return fmt.Errorf("store data key %s: %w", r.Id, err)
	}
	if n != 1 {
		return fmt.Errorf("data key %s changed while it was re-wrapped", r.Id)
	}
	return nil
}

func (m *Manager) RewrapLoop(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		m.rewrapAndLog(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) rewrapAndLog(ctx context.Context) {
	res, err := m.Rewrap(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.log.ErrorContext(ctx, "re-wrap of data keys failed; it will retry", "error", err, "rewrapped", res.Rewrapped)
		}
		return
	}
	if res.Rewrapped > 0 {
		m.log.InfoContext(ctx, "re-wrapped data keys with the active KEK version", "rewrapped", res.Rewrapped)
	}
	for _, v := range res.Retired {
		m.log.InfoContext(ctx, "KEK version retired: no data key uses it any more", "kek_version", v)
	}
}
