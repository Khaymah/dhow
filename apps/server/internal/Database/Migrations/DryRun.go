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
	DefaultDryRunTimeout      = time.Minute

	CodeLockTimeout        = "PREFLIGHT_LOCK_TIMEOUT"
	CodeStatementTimeout   = "PREFLIGHT_STATEMENT_TIMEOUT"
	CodeMigrationFailed    = "PREFLIGHT_MIGRATION_FAILED"
	CodeTransactionControl = "PREFLIGHT_TRANSACTION_CONTROL"
	CodeTransactionEnded   = "PREFLIGHT_TRANSACTION_ENDED"
	CodeDeadline           = "PREFLIGHT_DEADLINE"

	sqlStateLockNotAvailable = "55P03"
	sqlStateQueryCanceled    = "57014"
	txStatusInTransaction    = 'T'
)

var (
	ErrTransactionEnded   = errors.New("the statement ended the dry run's transaction")
	ErrTransactionControl = errors.New("the statement controls the transaction; migrations never run BEGIN, COMMIT, END, ROLLBACK, SAVEPOINT, RELEASE or PREPARE TRANSACTION")
)

type DryRunOptions struct {
	Timeout time.Duration
}

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

func preflightCode(ctx context.Context, err error) string {
	if errors.Is(err, ErrTransactionEnded) {
		return CodeTransactionEnded
	}
	if ctx.Err() != nil {
		return CodeDeadline
	}
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

func refuseTransactionControl(pending []PlanEntry) error {
	for _, e := range pending {
		if !e.UseTx {
			continue
		}
		for k, stmt := range e.Up {
			if TransactionControl(stmt) {
				return &PreflightError{Code: CodeTransactionControl, Name: e.Name, Statement: k + 1, Err: ErrTransactionControl}
			}
		}
	}
	return nil
}

func (m *Migrator) DryRun(ctx context.Context, opts DryRunOptions) (DryRunResult, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultDryRunTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	entries, err := m.Plan(ctx)
	if err != nil {
		return DryRunResult{}, err
	}
	pending := Pending(entries)
	if len(pending) == 0 {
		return DryRunResult{}, nil
	}
	if err := refuseTransactionControl(pending); err != nil {
		return DryRunResult{}, err
	}
	return m.tryInTransaction(ctx, pending)
}

func (m *Migrator) tryInTransaction(ctx context.Context, pending []PlanEntry) (res DryRunResult, retErr error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("dry run: begin: %w", err)
	}
	defer func() {
		err := tx.Rollback(context.WithoutCancel(ctx))
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) && ctx.Err() == nil {
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
			return res, &PreflightError{Code: preflightCode(ctx, err), Name: e.Name, Statement: failed + 1, Err: err}
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
		if status := tx.Conn().PgConn().TxStatus(); status != txStatusInTransaction {
			return k, fmt.Errorf("%w (%s, transaction status %q)", ErrTransactionEnded, CodeTransactionEnded, status)
		}
	}
	return 0, nil
}
