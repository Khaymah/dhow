package keys

import (
	"context"
	"errors"
	"fmt"

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
	Scan(ctx context.Context, db sqlc.DBTX, after string, limit int32) ([]Value, error)
	Replace(ctx context.Context, db sqlc.DBTX, v Value, sealed []byte) (bool, error)
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

func BackfillName(spec Spec) string { return BackfillPrefix + spec.Name() }
