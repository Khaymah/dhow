package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type purge struct {
	table string
	after time.Duration
	run   func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error)
}

var purges = []purge{
	{"LoginThrottles", config.RetentionThrottleAfter, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeLoginThrottles(ctx, sqlc.PurgeLoginThrottlesParams{Before: before, Batch: batch})
	}},
	{"RunnerThrottles", config.RetentionThrottleAfter, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeRunnerThrottles(ctx, sqlc.PurgeRunnerThrottlesParams{Before: before, Batch: batch})
	}},
	{"AuthChallenges", config.RetentionChallengeAfter, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeAuthChallenges(ctx, sqlc.PurgeAuthChallengesParams{Before: before, Batch: batch})
	}},
	{"OidcFlows", config.RetentionOidcFlowAfter, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeOidcFlows(ctx, sqlc.PurgeOidcFlowsParams{Before: before, Batch: batch})
	}},
	{"Invitations", config.RetentionInviteAfter, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeInvitations(ctx, sqlc.PurgeInvitationsParams{Before: before, Batch: batch})
	}},
	{"SetupTokens", config.RetentionSetupAfter, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeSetupTokens(ctx, sqlc.PurgeSetupTokensParams{Before: before, Batch: batch})
	}},
	{"RateLimits", config.RetentionRateLimitAfter, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeRateLimits(ctx, sqlc.PurgeRateLimitsParams{Before: before, Batch: batch})
	}},
	{"NotificationOutbox", time.Duration(config.NotifyRetentionDefault) * 24 * time.Hour, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.PurgeNotificationOutbox(ctx, sqlc.PurgeNotificationOutboxParams{Before: before, Batch: batch})
	}},
	{"SecretVersions", config.SecretVersionRetention, func(ctx context.Context, q *sqlc.Queries, before time.Time, batch int32) (int64, error) {
		return q.EraseExpiredSecretVersions(ctx, sqlc.EraseExpiredSecretVersionsParams{Before: before, Batch: batch})
	}},
}

func Tables() []string {
	out := make([]string, 0, len(purges))
	for _, p := range purges {
		out = append(out, p.table)
	}
	return out
}

type Options struct {
	Pool       *pgxpool.Pool
	NotifyDays int
	Log        *slog.Logger
	Now        func() time.Time
	Batch      int
	MaxBatches int
}

type Runner struct {
	pool       *pgxpool.Pool
	notifyDays int
	log        *slog.Logger
	now        func() time.Time
	batch      int32
	maxBatches int
}

type Report struct {
	Ran     bool
	Now     time.Time
	Deleted map[string]int64
	Batches map[string]int
}

func New(o Options) *Runner {
	batch, maxBatches := o.Batch, o.MaxBatches
	if batch <= 0 || batch > config.RetentionBatch {
		batch = config.RetentionBatch
	}
	if maxBatches <= 0 {
		maxBatches = config.RetentionMaxBatches
	}
	return &Runner{
		pool: o.Pool, log: o.Log, now: o.Now, maxBatches: maxBatches, notifyDays: o.NotifyDays,
		batch: int32(batch),
	}
}

func (r *Runner) Run(ctx context.Context) (report Report, err error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("retention: acquire a connection: %w", err)
	}
	defer conn.Release()
	q := sqlc.New(conn)
	locked, err := q.TryRetentionLock(ctx, config.RetentionLockKey)
	if err != nil || !locked {
		return Report{}, err
	}
	defer func() {
		released, unlockErr := q.ReleaseRetentionLock(context.WithoutCancel(ctx), config.RetentionLockKey)
		if unlockErr != nil || !released {
			closeErr := conn.Hijack().Close(context.WithoutCancel(ctx))
			err = errors.Join(err, fmt.Errorf("retention: release the lock (released=%t): %w", released, errors.Join(unlockErr, closeErr)))
		}
	}()
	now, err := r.clock(ctx, q)
	if err != nil {
		return Report{}, err
	}
	report = Report{Ran: true, Now: now, Deleted: map[string]int64{}, Batches: map[string]int{}}
	for _, p := range purges {
		if err := r.purgeTable(ctx, q, p, now.Add(-r.periodOf(p)), &report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (r *Runner) clock(ctx context.Context, q *sqlc.Queries) (time.Time, error) {
	if r.now != nil {
		return r.now().UTC(), nil
	}
	now, err := q.RetentionClock(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("retention: read the database clock: %w", err)
	}
	return now.UTC(), nil
}

func (r *Runner) purgeTable(ctx context.Context, q *sqlc.Queries, p purge, before time.Time, report *Report) error {
	for range r.maxBatches {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := p.run(ctx, q, before, r.batch)
		if err != nil {
			return fmt.Errorf("retention: purge %s: %w", p.table, err)
		}
		report.Batches[p.table]++
		report.Deleted[p.table] += n
		if n < int64(r.batch) {
			return nil
		}
	}
	return nil
}

func (r *Runner) Loop(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = config.RetentionEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		r.runOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) runOnce(ctx context.Context) {
	report, err := r.Run(ctx)
	if err != nil {
		if ctx.Err() == nil {
			r.log.ErrorContext(ctx, "retention run failed", "error", err)
		}
		return
	}
	if !report.Ran {
		r.log.DebugContext(ctx, "retention skipped: another instance holds the lock")
		return
	}
	for _, table := range Tables() {
		if n := report.Deleted[table]; n > 0 {
			r.log.InfoContext(ctx, "expired rows deleted", "table", table, "count", n, "batches", report.Batches[table])
		}
	}
}

func (r *Runner) periodOf(p purge) time.Duration {
	if p.table == "NotificationOutbox" && r.notifyDays > 0 {
		return time.Duration(r.notifyDays) * 24 * time.Hour
	}
	return p.after
}
