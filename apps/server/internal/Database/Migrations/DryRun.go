package migrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	PreflightLockTimeout      = "3s"
	PreflightStatementTimeout = "5min"

	CodeLockTimeout      = "PREFLIGHT_LOCK_TIMEOUT"
	CodeStatementTimeout = "PREFLIGHT_STATEMENT_TIMEOUT"
	CodeMigrationFailed  = "PREFLIGHT_MIGRATION_FAILED"

	sqlStateLockNotAvailable = "55P03"
	sqlStateQueryCanceled    = "57014"
)

type DryRunOutcome string

const (
	DryRunApplied    DryRunOutcome = "applied"
	DryRunNotTried   DryRunOutcome = "not_tried"
	DryRunFailed     DryRunOutcome = "failed"
	DryRunNotReached DryRunOutcome = "not_reached"
)

type DryRunStep struct {
	Version  int64
	Name     string
	Phase    string
	Outcome  DryRunOutcome
	Duration time.Duration
}

type DryRunResult struct {
	Steps []DryRunStep
}

type PreflightError struct {
	Code      string
	Name      string
	Statement int
	Err       error
}

func (e *PreflightError) Error() string {
	return fmt.Sprintf("%s: %s, statement %d: %v", e.Code, e.Name, e.Statement, e.Err)
}

func (e *PreflightError) Unwrap() error { return e.Err }

func preflightCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateLockNotAvailable:
			return CodeLockTimeout
		case sqlStateQueryCanceled:
			return CodeStatementTimeout
		}
	}
	return CodeMigrationFailed
}

func (m *Migrator) DryRun(ctx context.Context) (res DryRunResult, retErr error) {
	entries, err := m.Plan(ctx)
	if err != nil {
		return res, err
	}
	pending := Pending(entries)
	if len(pending) == 0 {
		return res, nil
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("dry run: begin: %w", err)
	}
	defer func() {
		if err := tx.Rollback(context.WithoutCancel(ctx)); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("dry run: roll back: %w", err))
		}
	}()
	err = sqlc.New(tx).SetPreflightTimeouts(ctx, sqlc.SetPreflightTimeoutsParams{
		LockTimeout: PreflightLockTimeout, StatementTimeout: PreflightStatementTimeout,
	})
	if err != nil {
		return res, fmt.Errorf("dry run: set the timeouts: %w", err)
	}
	for i, e := range pending {
		step := DryRunStep{Version: e.Version, Name: e.Name, Phase: e.Phase, Outcome: DryRunNotTried}
		if !e.UseTx {
			res.Steps = append(res.Steps, step)
			continue
		}
		started := time.Now()
		failed, err := applyStatements(ctx, tx, e.Up)
		step.Duration = time.Since(started)
		if err != nil {
			step.Outcome = DryRunFailed
			res.Steps = append(res.Steps, step)
			for _, rest := range pending[i+1:] {
				res.Steps = append(res.Steps, DryRunStep{Version: rest.Version, Name: rest.Name, Phase: rest.Phase, Outcome: DryRunNotReached})
			}
			return res, &PreflightError{Code: preflightCode(err), Name: e.Name, Statement: failed + 1, Err: err}
		}
		step.Outcome = DryRunApplied
		res.Steps = append(res.Steps, step)
	}
	return res, nil
}

func applyStatements(ctx context.Context, tx pgx.Tx, stmts []string) (int, error) {
	for k, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return k, err
		}
	}
	return 0, nil
}
