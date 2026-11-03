package keys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	backfill "github.com/Khaymah/dhow/apps/server/internal/Core/Backfill"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const LegacyFormat = byte(1)

var ErrWrongOwnerKind = errors.New("this column belongs to another kind of owner")

type Spec struct {
	Table        string
	Column       string
	Label        string
	Owner        OwnerKind
	MaxPlaintext int
	MaxSealed    int
}

func (s Spec) Name() string { return s.Table + "." + s.Column }

func (s Spec) Valid() error {
	switch {
	case s.Table == "" || s.Column == "":
		return errors.New("a value location needs its table and column")
	case s.Label == "" || len(s.Label) > maxLabelBytes:
		return fmt.Errorf("%s: %w", s.Name(), ErrBadLabel)
	case !knownKind(s.Owner):
		return fmt.Errorf("%s: %w", s.Name(), ErrBadOwner)
	case s.MaxPlaintext <= 0 || s.MaxPlaintext > MaxPlaintext:
		return fmt.Errorf("%s: the largest plaintext must be 1 byte to 1 MiB", s.Name())
	case s.MaxPlaintext+Overhead > s.MaxSealed:
		return fmt.Errorf("%s: the largest plaintext (%d bytes) does not fit the column (%d bytes) once sealed", s.Name(), s.MaxPlaintext, s.MaxSealed)
	}
	return nil
}

type Values struct {
	envelope *Envelope
	legacy   []byte
	mu       sync.RWMutex
	closed   map[string]bool
}

func NewValues(envelope *Envelope, legacySecret []byte) *Values {
	return &Values{envelope: envelope, legacy: append([]byte(nil), legacySecret...), closed: map[string]bool{}}
}

func (v *Values) CloseLegacy(column string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.closed[column] = true
}

func (v *Values) LegacyClosed(column string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.closed[column]
}

func (v *Values) BackfillDone(name string) {
	if column, ok := strings.CutPrefix(name, BackfillPrefix); ok {
		v.CloseLegacy(column)
	}
}

func (v *Values) LoadLegacyState(ctx context.Context, db sqlc.DBTX) error {
	list, err := backfill.List(ctx, db)
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.Done() {
			v.BackfillDone(p.Name)
		}
	}
	return nil
}

func (v *Values) Envelope() *Envelope { return v.envelope }

func (v *Values) Column(spec Spec) (*Column, error) {
	if err := spec.Valid(); err != nil {
		return nil, err
	}
	c := &Column{spec: spec, envelope: v.envelope, values: v}
	if len(v.legacy) == 0 {
		return c, nil
	}
	legacy, err := appcrypto.NewSealer(v.legacy, spec.Label)
	if err != nil {
		return nil, fmt.Errorf("%s: open the legacy format: %w", spec.Name(), err)
	}
	c.legacy = legacy
	return c, nil
}

type Column struct {
	spec     Spec
	envelope *Envelope
	values   *Values
	legacy   *appcrypto.Sealer
}

func (c *Column) Spec() Spec { return c.spec }

func (c *Column) SealIn(ctx context.Context, q *sqlc.Queries, owner Owner, rowContext, plaintext []byte) ([]byte, error) {
	if owner.Kind != c.spec.Owner {
		return nil, ErrWrongOwnerKind
	}
	if len(plaintext) > c.spec.MaxPlaintext {
		return nil, fmt.Errorf("%s: %w", c.spec.Name(), ErrTooLarge)
	}
	return c.envelope.SealIn(ctx, q, owner, c.spec.Label, rowContext, plaintext)
}

func (c *Column) Open(ctx context.Context, owner Owner, rowContext, sealed []byte) ([]byte, error) {
	if owner.Kind != c.spec.Owner {
		return nil, ErrWrongOwnerKind
	}
	if len(sealed) == 0 {
		return nil, ErrSealed
	}
	switch sealed[0] {
	case FormatVersion:
		return c.envelope.Open(ctx, owner, c.spec.Label, rowContext, sealed)
	case LegacyFormat:
		if c.legacy == nil || len(rowContext) == 0 || c.values.LegacyClosed(c.spec.Name()) {
			return nil, ErrSealed
		}
		plain, err := c.legacy.Open(sealed, rowContext)
		if err != nil {
			return nil, ErrSealed
		}
		return plain, nil
	}
	return nil, ErrSealed
}

func (c *Column) Reseal(ctx context.Context, q *sqlc.Queries, owner Owner, rowContext, sealed []byte) ([]byte, error) {
	plain, err := c.Open(ctx, owner, rowContext, sealed)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	return c.envelope.sealIn(ctx, q, owner, c.spec.Label, rowContext, plain, false)
}

func IsLegacy(sealed []byte) bool { return len(sealed) > 0 && sealed[0] == LegacyFormat }
