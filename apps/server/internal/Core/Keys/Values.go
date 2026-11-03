package keys

import (
	"context"
	"errors"
	"fmt"

	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
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
}

func NewValues(envelope *Envelope, legacySecret []byte) *Values {
	return &Values{envelope: envelope, legacy: append([]byte(nil), legacySecret...)}
}

func (v *Values) Envelope() *Envelope { return v.envelope }

func (v *Values) Column(spec Spec) (*Column, error) {
	if err := spec.Valid(); err != nil {
		return nil, err
	}
	c := &Column{spec: spec, envelope: v.envelope}
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
	legacy   *appcrypto.Sealer
}

func (c *Column) Spec() Spec { return c.spec }

func (c *Column) Seal(ctx context.Context, owner Owner, rowContext, plaintext []byte) ([]byte, error) {
	if owner.Kind != c.spec.Owner {
		return nil, ErrWrongOwnerKind
	}
	if len(plaintext) > c.spec.MaxPlaintext {
		return nil, fmt.Errorf("%s: %w", c.spec.Name(), ErrTooLarge)
	}
	return c.envelope.Seal(ctx, owner, c.spec.Label, rowContext, plaintext)
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
		if c.legacy == nil || len(rowContext) == 0 {
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

func (c *Column) Reseal(ctx context.Context, owner Owner, rowContext, sealed []byte) ([]byte, error) {
	plain, err := c.Open(ctx, owner, rowContext, sealed)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	return c.envelope.Seal(ctx, owner, c.spec.Label, rowContext, plain)
}

func IsLegacy(sealed []byte) bool { return len(sealed) > 0 && sealed[0] == LegacyFormat }
