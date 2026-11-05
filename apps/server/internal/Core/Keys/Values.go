package keys

import (
	"context"
	"errors"
	"fmt"
	"slices"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

var ErrWrongOwnerKind = errors.New("this column belongs to another kind of owner")

type Spec struct {
	Table        string
	Column       string
	Label        string
	Owner        OwnerKind
	AlsoOwners   []OwnerKind
	MaxPlaintext int
	MaxSealed    int
}

func (s Spec) Name() string { return s.Table + "." + s.Column }

func (s Spec) Allows(kind OwnerKind) bool {
	return kind == s.Owner || slices.Contains(s.AlsoOwners, kind)
}

func (s Spec) OwnerKinds() []OwnerKind { return append([]OwnerKind{s.Owner}, s.AlsoOwners...) }

func (s Spec) Valid() error {
	switch {
	case s.Table == "" || s.Column == "":
		return errors.New("a value location needs its table and column")
	case s.Label == "" || len(s.Label) > maxLabelBytes:
		return fmt.Errorf("%s: %w", s.Name(), ErrBadLabel)
	case !knownKind(s.Owner) || slices.ContainsFunc(s.AlsoOwners, func(k OwnerKind) bool { return !knownKind(k) || k == OwnerInstance }):
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
}

func NewValues(envelope *Envelope) *Values {
	return &Values{envelope: envelope}
}

func (v *Values) Envelope() *Envelope { return v.envelope }

func (v *Values) Column(spec Spec) (*Column, error) {
	if err := spec.Valid(); err != nil {
		return nil, err
	}
	return &Column{spec: spec, envelope: v.envelope}, nil
}

type Column struct {
	spec     Spec
	envelope *Envelope
}

func (c *Column) Spec() Spec { return c.spec }

func (c *Column) SealIn(ctx context.Context, q *sqlc.Queries, owner Owner, rowContext, plaintext []byte) ([]byte, error) {
	if !c.spec.Allows(owner.Kind) {
		return nil, ErrWrongOwnerKind
	}
	if len(plaintext) > c.spec.MaxPlaintext {
		return nil, fmt.Errorf("%s: %w", c.spec.Name(), ErrTooLarge)
	}
	return c.envelope.SealIn(ctx, q, owner, c.spec.Label, rowContext, plaintext)
}

func (c *Column) Open(ctx context.Context, owner Owner, rowContext, sealed []byte) ([]byte, error) {
	if !c.spec.Allows(owner.Kind) {
		return nil, ErrWrongOwnerKind
	}
	if len(sealed) == 0 || sealed[0] != FormatVersion {
		return nil, ErrSealed
	}
	return c.envelope.Open(ctx, owner, c.spec.Label, rowContext, sealed)
}

func (c *Column) Reseal(ctx context.Context, q *sqlc.Queries, owner Owner, rowContext, sealed []byte) ([]byte, error) {
	plain, err := c.Open(ctx, owner, rowContext, sealed)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	return c.envelope.sealIn(ctx, q, owner, c.spec.Label, rowContext, plain, false)
}
