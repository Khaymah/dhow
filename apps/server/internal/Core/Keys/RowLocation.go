package keys

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type RowLocation[R any] struct {
	Of          Spec
	ScanRows    func(ctx context.Context, q *sqlc.Queries, after uuid.UUID, limit int32, legacyOnly bool) ([]R, error)
	Read        func(row R) (id uuid.UUID, owner Owner, rowContext, sealed []byte)
	ReplaceRow  func(ctx context.Context, q *sqlc.Queries, id uuid.UUID, old, sealed []byte) (int64, error)
	CountOld    func(ctx context.Context, q *sqlc.Queries) (int64, error)
	CountByKeys func(ctx context.Context, q *sqlc.Queries, dataKeys [][]byte) (int64, error)
}

func (l RowLocation[R]) Spec() Spec { return l.Of }

func (l RowLocation[R]) Scan(ctx context.Context, db sqlc.DBTX, after string, limit int32, legacyOnly bool) ([]Value, error) {
	start := uuid.Nil
	if after != "" {
		id, err := uuid.Parse(after)
		if err != nil {
			return nil, fmt.Errorf("%s: the cursor %q is not a row id: %w", l.Of.Name(), after, err)
		}
		start = id
	}
	rows, err := l.ScanRows(ctx, sqlc.New(db), start, limit, legacyOnly)
	if err != nil {
		return nil, fmt.Errorf("read the values of %s: %w", l.Of.Name(), err)
	}
	out := make([]Value, 0, len(rows))
	for _, r := range rows {
		id, owner, rowContext, sealed := l.Read(r)
		out = append(out, Value{Key: id.String(), Owner: owner, Context: rowContext, Sealed: sealed})
	}
	return out, nil
}

func (l RowLocation[R]) Replace(ctx context.Context, db sqlc.DBTX, v Value, sealed []byte) (bool, error) {
	id, err := uuid.Parse(v.Key)
	if err != nil {
		return false, fmt.Errorf("%s: the row %q is not a row id: %w", l.Of.Name(), v.Key, err)
	}
	n, err := l.ReplaceRow(ctx, sqlc.New(db), id, v.Sealed, sealed)
	if err != nil {
		return false, fmt.Errorf("store a value of %s: %w", l.Of.Name(), err)
	}
	return n == 1, nil
}

func (l RowLocation[R]) CountLegacy(ctx context.Context, db sqlc.DBTX) (int64, error) {
	return l.CountOld(ctx, sqlc.New(db))
}

func (l RowLocation[R]) CountUnder(ctx context.Context, db sqlc.DBTX, dataKeys [][]byte) (int64, error) {
	return l.CountByKeys(ctx, sqlc.New(db), dataKeys)
}
