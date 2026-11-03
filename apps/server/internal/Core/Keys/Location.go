package keys

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	backfill "github.com/Khaymah/dhow/apps/server/internal/Core/Backfill"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const BackfillPrefix = "keys.envelope."

var ErrDuplicateLocation = errors.New("two value locations name the same column")

type Value struct {
	Key     string
	Owner   Owner
	Context []byte
	Sealed  []byte
}

type Location interface {
	Spec() Spec
	Scan(ctx context.Context, db sqlc.DBTX, after string, limit int32, legacyOnly bool) ([]Value, error)
	Replace(ctx context.Context, db sqlc.DBTX, v Value, sealed []byte) (bool, error)
	CountLegacy(ctx context.Context, db sqlc.DBTX) (int64, error)
	CountUnder(ctx context.Context, db sqlc.DBTX, dataKeys [][]byte) (int64, error)
}

type Registry struct {
	locations []Location
}

func NewRegistry(locations ...Location) (*Registry, error) {
	seen := map[string]bool{}
	for _, l := range locations {
		spec := l.Spec()
		if err := spec.Valid(); err != nil {
			return nil, err
		}
		if seen[spec.Name()] {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateLocation, spec.Name())
		}
		seen[spec.Name()] = true
	}
	return &Registry{locations: append([]Location(nil), locations...)}, nil
}

func (r *Registry) Locations() []Location { return append([]Location(nil), r.locations...) }

func (r *Registry) BackfillJobs(values *Values, log *slog.Logger) ([]backfill.Job, error) {
	jobs := make([]backfill.Job, 0, len(r.locations))
	for _, l := range r.locations {
		col, err := values.Column(l.Spec())
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, &envelopeBackfill{location: l, column: col, log: log})
	}
	return jobs, nil
}

func BackfillName(spec Spec) string { return BackfillPrefix + spec.Name() }

type envelopeBackfill struct {
	location Location
	column   *Column
	log      *slog.Logger
}

func (b *envelopeBackfill) Name() string { return BackfillName(b.location.Spec()) }

func (b *envelopeBackfill) Step(ctx context.Context, tx sqlc.DBTX, after string, limit int32) (backfill.Batch, error) {
	rows, err := b.location.Scan(ctx, tx, after, limit, true)
	if err != nil {
		return backfill.Batch{}, err
	}
	out := backfill.Batch{Rows: len(rows)}
	for _, v := range rows {
		out.Last = v.Key
		sealed, err := b.column.Reseal(ctx, v.Owner, v.Context, v.Sealed)
		if errors.Is(err, ErrSealed) {
			out.Failed++
			b.logger().WarnContext(ctx, "a stored value cannot be opened, so it keeps its old format",
				"location", b.location.Spec().Name(), "row", v.Key)
			continue
		}
		if err != nil {
			return backfill.Batch{}, err
		}
		ok, err := b.location.Replace(ctx, tx, v, sealed)
		if err != nil {
			return backfill.Batch{}, err
		}
		if ok {
			out.Changed++
		}
	}
	return out, nil
}

func (b *envelopeBackfill) Remaining(ctx context.Context, tx sqlc.DBTX) (int64, error) {
	return b.location.CountLegacy(ctx, tx)
}

func (b *envelopeBackfill) logger() *slog.Logger {
	if b.log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return b.log
}
