package artifacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type SweepReport struct {
	Ran      bool
	Expired  int64
	Removed  int
	Orphans  int
	TempGone int
}

func (s *Service) RunRetention(ctx context.Context) {
	tick := time.NewTicker(config.BlobSweepEvery)
	defer tick.Stop()
	for {
		if _, err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			s.log.ErrorContext(ctx, "artifact retention failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Service) Sweep(ctx context.Context) (SweepReport, error) {
	var report SweepReport
	for {
		n, err := s.queries().ExpireArtifacts(ctx, config.BlobSweepBatch)
		if err != nil {
			return report, fmt.Errorf("expire artifacts: %w", err)
		}
		report.Expired += n
		if n < config.BlobSweepBatch {
			break
		}
	}
	removed, orphans, ran, err := s.sweepBlobs(ctx)
	report.Removed, report.Orphans, report.Ran = removed, orphans, ran
	if err != nil {
		return report, err
	}
	gone, err := s.store.SweepTemp(ctx, s.now().Add(-config.BlobTempGrace).UnixNano())
	report.TempGone = gone
	return report, err
}

func (s *Service) sweepBlobs(ctx context.Context) (removed, orphans int, ran bool, err error) {
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return 0, 0, false, fmt.Errorf("acquire a connection for the blob sweep: %w", err)
	}
	defer conn.Release()
	q := sqlc.New(conn)
	locked, err := q.TryBlobSweepLock(ctx, config.BlobSweepLockKey)
	if err != nil || !locked {
		return 0, 0, false, err
	}
	defer func() {
		released, unlockErr := q.ReleaseBlobSweepLock(context.WithoutCancel(ctx), config.BlobSweepLockKey)
		if unlockErr != nil || !released {
			closeErr := conn.Hijack().Close(context.WithoutCancel(ctx))
			err = errors.Join(err, fmt.Errorf("release the blob sweep lock (released=%t): %w", released, errors.Join(unlockErr, closeErr)))
		}
	}()
	before := s.now().Add(-config.BlobSweepGrace)
	if removed, err = s.markAndSweep(ctx, conn, before); err != nil {
		return removed, 0, true, err
	}
	orphans, err = s.sweepOrphans(ctx, q, before)
	return removed, orphans, true, err
}

func (s *Service) markAndSweep(ctx context.Context, conn *pgxpool.Conn, before time.Time) (int, error) {
	removed := 0
	for {
		var digests [][]byte
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			var err error
			txq := sqlc.New(tx)
			digests, err = txq.MarkUnusedBlobs(ctx, sqlc.MarkUnusedBlobsParams{Before: before, Batch: config.BlobSweepBatch})
			if err != nil || len(digests) == 0 {
				return err
			}
			return txq.RetireBlobOwners(ctx, digests)
		})
		if err != nil {
			return removed, fmt.Errorf("mark unused blobs: %w", err)
		}
		for _, raw := range digests {
			d, ok := blob.DigestOf(raw)
			if !ok {
				continue
			}
			if err := s.store.Remove(ctx, d); err != nil {
				return removed, err
			}
			removed++
		}
		if len(digests) < config.BlobSweepBatch {
			return removed, nil
		}
	}
}

func (s *Service) sweepOrphans(ctx context.Context, q *sqlc.Queries, before time.Time) (int, error) {
	stored, err := s.store.Stored(ctx, before.UnixNano())
	if err != nil {
		return 0, err
	}
	removed := 0
	for start := 0; start < len(stored); start += config.BlobSweepBatch {
		batch := stored[start:min(start+config.BlobSweepBatch, len(stored))]
		raw := make([][]byte, 0, len(batch))
		for i := range batch {
			raw = append(raw, batch[i][:])
		}
		known, err := q.KnownBlobs(ctx, raw)
		if err != nil {
			return removed, fmt.Errorf("look up stored blobs: %w", err)
		}
		keep := make(map[blob.Digest]bool, len(known))
		for _, k := range known {
			if d, ok := blob.DigestOf(k); ok {
				keep[d] = true
			}
		}
		for _, d := range batch {
			if keep[d] {
				continue
			}
			if err := s.store.Remove(ctx, d); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}
