package backfill

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const undefinedTable = "42P01"

type Progress struct {
	Name      string
	LastKey   string
	Processed int64
	Failed    int64
	Remaining *int64
	Passes    int32
	StartedAt time.Time
	UpdatedAt time.Time
	DoneAt    *time.Time
}

func (p Progress) Done() bool { return p.DoneAt != nil }

func List(ctx context.Context, db sqlc.DBTX) ([]Progress, error) {
	rows, err := sqlc.New(db).ListBackfills(ctx)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == undefinedTable {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the backfills: %w", err)
	}
	out := make([]Progress, 0, len(rows))
	for _, r := range rows {
		var remaining *int64
		if r.Remaining.Valid {
			n := r.Remaining.Int64
			remaining = &n
		}
		out = append(out, Progress{
			Name: r.Name, LastKey: r.LastKey, Processed: r.Processed, Failed: r.Failed, Remaining: remaining, Passes: r.Passes,
			StartedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DoneAt: r.DoneAt,
		})
	}
	return out, nil
}

type NotDoneError struct {
	Names []string
}

func (e *NotDoneError) Error() string {
	return CodeNotDone + ": these background backfills have not finished: " + strings.Join(e.Names, ", ") +
		"; run the release that starts them and wait until dhow-server migrate status shows them done"
}

func RequireDone(ctx context.Context, db sqlc.DBTX, names ...string) error {
	if len(names) == 0 {
		return nil
	}
	done, err := sqlc.New(db).ListDoneBackfills(ctx, names)
	if err != nil {
		return fmt.Errorf("read the backfills: %w", err)
	}
	var missing []string
	for _, n := range names {
		if !slices.Contains(done, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return &NotDoneError{Names: missing}
	}
	return nil
}
