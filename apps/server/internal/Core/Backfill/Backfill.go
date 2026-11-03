package backfill

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	DefaultBatchSize = 1000
	DefaultPause     = 100 * time.Millisecond
	DefaultEvery     = 30 * time.Second
	CodeNotDone      = "BACKFILL_NOT_DONE"
	maxKeyBytes      = 512
)

var (
	validName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
	ErrBadName     = errors.New("a backfill name is 1 to 200 letters, digits, dots, dashes or underscores")
	ErrDuplicate   = errors.New("two backfills have the same name")
	ErrKeyTooLong  = errors.New("a backfill cursor key is longer than 512 bytes")
	ErrKeyNotAhead = errors.New("a backfill batch did not move its cursor forward")
)

type Batch struct {
	Last    string
	Rows    int
	Changed int64
	Failed  int64
}

type Job interface {
	Name() string
	Step(ctx context.Context, tx sqlc.DBTX, after string, limit int32) (Batch, error)
	Remaining(ctx context.Context, tx sqlc.DBTX) (int64, error)
}

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Options struct {
	DB        DB
	Jobs      []Job
	BatchSize int32
	Pause     time.Duration
	Every     time.Duration
	Log       *slog.Logger
	Wait      func(ctx context.Context, d time.Duration) bool
}

type Runner struct {
	db    DB
	jobs  []Job
	batch int32
	pause time.Duration
	every time.Duration
	log   *slog.Logger
	wait  func(ctx context.Context, d time.Duration) bool
}

func New(o Options) (*Runner, error) {
	seen := map[string]bool{}
	for _, j := range o.Jobs {
		if !validName.MatchString(j.Name()) {
			return nil, fmt.Errorf("%w: %q", ErrBadName, j.Name())
		}
		if seen[j.Name()] {
			return nil, fmt.Errorf("%w: %s", ErrDuplicate, j.Name())
		}
		seen[j.Name()] = true
	}
	r := &Runner{db: o.DB, jobs: o.Jobs, batch: o.BatchSize, pause: o.Pause, every: o.Every, log: o.Log, wait: o.Wait}
	if r.batch <= 0 {
		r.batch = DefaultBatchSize
	}
	if r.pause < 0 {
		r.pause = 0
	}
	if r.every <= 0 {
		r.every = DefaultEvery
	}
	if r.log == nil {
		r.log = slog.New(slog.DiscardHandler)
	}
	if r.wait == nil {
		r.wait = sleep
	}
	return r, nil
}

func (r *Runner) Names() []string {
	out := make([]string, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, j.Name())
	}
	return out
}

type Result struct {
	Batch
	Locked    bool
	Done      bool
	PassEnded bool
	Remaining int64
}

func (r *Runner) Step(ctx context.Context, job Job) (Result, error) {
	var res Result
	err := dbtx.RunTx(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		locked, err := q.TryLockBackfill(ctx, job.Name())
		if err != nil {
			return fmt.Errorf("lock backfill %s: %w", job.Name(), err)
		}
		if !locked {
			res.Locked = true
			return nil
		}
		if err := q.EnsureBackfill(ctx, job.Name()); err != nil {
			return fmt.Errorf("record backfill %s: %w", job.Name(), err)
		}
		row, err := q.LockBackfillRow(ctx, job.Name())
		if err != nil {
			return fmt.Errorf("read backfill %s: %w", job.Name(), err)
		}
		if row.DoneAt != nil {
			res.Done = true
			return nil
		}
		res, err = r.advance(ctx, tx, job, row.LastKey)
		return err
	})
	return res, err
}

func (r *Runner) advance(ctx context.Context, tx pgx.Tx, job Job, after string) (Result, error) {
	q := sqlc.New(tx)
	b, err := job.Step(ctx, tx, after, r.batch)
	if err != nil {
		return Result{}, fmt.Errorf("backfill %s after %q: %w", job.Name(), after, err)
	}
	res := Result{Batch: b}
	if b.Rows > 0 {
		if len(b.Last) > maxKeyBytes {
			return Result{}, fmt.Errorf("backfill %s: %w", job.Name(), ErrKeyTooLong)
		}
		if b.Last <= after {
			return Result{}, fmt.Errorf("backfill %s: %w", job.Name(), ErrKeyNotAhead)
		}
		err := q.AdvanceBackfill(ctx, sqlc.AdvanceBackfillParams{LastKey: b.Last, Processed: b.Changed, Failed: b.Failed, Name: job.Name()})
		if err != nil {
			return Result{}, fmt.Errorf("record the progress of backfill %s: %w", job.Name(), err)
		}
		return res, nil
	}
	remaining, err := job.Remaining(ctx, tx)
	if err != nil {
		return Result{}, fmt.Errorf("count what backfill %s has left: %w", job.Name(), err)
	}
	if err := q.EndBackfillPass(ctx, sqlc.EndBackfillPassParams{Remaining: remaining, Name: job.Name()}); err != nil {
		return Result{}, fmt.Errorf("end a pass of backfill %s: %w", job.Name(), err)
	}
	res.PassEnded, res.Remaining, res.Done = true, remaining, remaining == 0
	return res, nil
}

func (r *Runner) Drain(ctx context.Context) error {
	var errs []error
	for _, job := range r.jobs {
		if err := r.drainJob(ctx, job); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) drainJob(ctx context.Context, job Job) error {
	for {
		res, err := r.Step(ctx, job)
		if err != nil {
			return err
		}
		if res.Failed > 0 {
			r.log.WarnContext(ctx, "a backfill skipped values it cannot convert; they stay as they are", "backfill", job.Name(), "failed", res.Failed)
		}
		switch {
		case res.Locked:
			return nil
		case res.Done && res.PassEnded:
			r.log.InfoContext(ctx, "backfill done", "backfill", job.Name())
			return nil
		case res.Done:
			return nil
		case res.PassEnded:
			r.log.WarnContext(ctx, "a backfill pass ended with rows left; it tries again later", "backfill", job.Name(), "remaining", res.Remaining)
			return nil
		}
		if !r.wait(ctx, r.pause) {
			return ctx.Err()
		}
	}
}

func (r *Runner) Loop(ctx context.Context) {
	for {
		if err := r.Drain(ctx); err != nil && ctx.Err() == nil {
			r.log.ErrorContext(ctx, "a backfill failed; it resumes from its last batch later", "error", err)
		}
		if !r.wait(ctx, r.every) {
			return
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
