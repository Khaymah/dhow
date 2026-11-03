package keys

import (
	"context"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const (
	FormatVersion  = byte(3)
	idBytes        = 16
	saltBytes      = 32
	saltOffset     = 1 + idBytes
	nonceOffset    = saltOffset + saltBytes
	headerBytes    = nonceOffset + gcmNonce
	Overhead       = headerBytes + gcmTag
	MaxPlaintext   = 1 << 20
	maxLabelBytes  = 255
	dekAADLabel    = "dhow/dek/v1"
	subkeyInfo     = "dhow/value/v2"
	insertAttempts = 3
)

type Envelope struct {
	store    Store
	provider KeyProvider
}

func NewEnvelope(store Store, provider KeyProvider) *Envelope {
	return &Envelope{store: store, provider: provider}
}

func (e *Envelope) Seal(ctx context.Context, owner Owner, label string, rowContext, plaintext []byte) ([]byte, error) {
	if err := checkPurpose(owner, label, rowContext); err != nil {
		return nil, err
	}
	if len(plaintext) > MaxPlaintext {
		return nil, ErrTooLarge
	}
	dk, err := e.dataKeyFor(ctx, owner)
	if err != nil {
		return nil, err
	}
	out := make([]byte, headerBytes, headerBytes+len(plaintext)+gcmTag)
	out[0] = FormatVersion
	copy(out[1:saltOffset], dk.ID[:])
	if _, err := rand.Read(out[saltOffset:headerBytes]); err != nil {
		return nil, fmt.Errorf("read salt and nonce: %w", err)
	}
	aead, err := e.valueAEAD(ctx, dk, out[saltOffset:nonceOffset], label)
	if err != nil {
		return nil, err
	}
	return aead.Seal(out, out[nonceOffset:headerBytes], plaintext, valueAAD(dk.ID, label, rowContext)), nil
}

func (e *Envelope) Open(ctx context.Context, owner Owner, label string, rowContext, sealed []byte) ([]byte, error) {
	if err := checkPurpose(owner, label, rowContext); err != nil {
		return nil, err
	}
	if len(sealed) < Overhead || sealed[0] != FormatVersion {
		return nil, ErrSealed
	}
	id, err := uuid.FromBytes(sealed[1:saltOffset])
	if err != nil {
		return nil, ErrSealed
	}
	dk, err := e.store.DataKey(ctx, id)
	if errors.Is(err, ErrNoDataKey) {
		return nil, ErrSealed
	}
	if err != nil {
		return nil, err
	}
	if dk.Owner != owner {
		return nil, ErrSealed
	}
	if dk.Destroyed {
		return nil, ErrShredded
	}
	aead, err := e.valueAEAD(ctx, dk, sealed[saltOffset:nonceOffset], label)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, sealed[nonceOffset:headerBytes], sealed[headerBytes:], valueAAD(id, label, rowContext))
	if err != nil {
		return nil, ErrSealed
	}
	return plain, nil
}

func (e *Envelope) valueAEAD(ctx context.Context, dk DataKey, salt []byte, label string) (cipher.AEAD, error) {
	dek, err := e.unwrap(ctx, dk)
	if err != nil {
		return nil, err
	}
	defer clear(dek)
	subkey, err := hkdf.Key(sha256.New, dek, salt, subkeyInfo+label, KeyBytes)
	if err != nil {
		return nil, fmt.Errorf("derive the value key: %w", err)
	}
	defer clear(subkey)
	return newAEAD(subkey)
}

func (e *Envelope) Destroy(ctx context.Context, owner Owner) (int64, error) {
	if !owner.Valid() {
		return 0, ErrBadOwner
	}
	return e.store.DestroyOwner(ctx, owner)
}

func checkPurpose(owner Owner, label string, rowContext []byte) error {
	if !owner.Valid() {
		return ErrBadOwner
	}
	if label == "" || len(label) > maxLabelBytes {
		return ErrBadLabel
	}
	if len(rowContext) == 0 {
		return ErrBadContext
	}
	return nil
}

func (e *Envelope) dataKeyFor(ctx context.Context, owner Owner) (DataKey, error) {
	for range insertAttempts {
		dk, err := e.store.ActiveDataKey(ctx, owner)
		if !errors.Is(err, ErrNoDataKey) {
			return dk, err
		}
		if err := e.newDataKey(ctx, owner); err != nil {
			return DataKey{}, err
		}
	}
	return e.store.ActiveDataKey(ctx, owner)
}

func (e *Envelope) newDataKey(ctx context.Context, owner Owner) error {
	version, err := e.store.ActiveKekVersion(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("make a data key id: %w", err)
	}
	dek, err := randomKey()
	if err != nil {
		return err
	}
	defer clear(dek)
	wrapped, err := e.provider.Wrap(ctx, version, dek, dekAAD(id, owner, version))
	if err != nil {
		return fmt.Errorf("wrap a new data key with KEK version %d: %w", version, err)
	}
	return e.store.InsertDataKey(ctx, DataKey{ID: id, Owner: owner, KekVersion: version, Wrapped: wrapped})
}

func (e *Envelope) unwrap(ctx context.Context, dk DataKey) ([]byte, error) {
	if dk.Destroyed || dk.Wrapped == nil {
		return nil, ErrShredded
	}
	dek, err := e.provider.Unwrap(ctx, dk.KekVersion, dk.Wrapped, dekAAD(dk.ID, dk.Owner, dk.KekVersion))
	if err != nil {
		return nil, fmt.Errorf("unwrap data key %s with KEK version %d: %w", dk.ID, dk.KekVersion, err)
	}
	return dek, nil
}

func valueAAD(id uuid.UUID, label string, rowContext []byte) []byte {
	out := make([]byte, 0, 1+idBytes+2+len(label)+len(rowContext))
	out = append(out, FormatVersion)
	out = append(out, id[:]...)
	out = binary.BigEndian.AppendUint16(out, uint16(len(label))) //nolint:gosec // The label is at most 255 bytes, checked by checkPurpose.
	out = append(out, label...)
	return append(out, rowContext...)
}

func dekAAD(id uuid.UUID, owner Owner, version int32) []byte {
	out := make([]byte, 0, len(dekAADLabel)+1+idBytes+1+len(owner.Kind)+idBytes+4)
	out = append(out, dekAADLabel...)
	out = append(out, 0)
	out = append(out, id[:]...)
	out = append(out, byte(len(owner.Kind))) //nolint:gosec // Owner kinds are short constants, checked by Owner.Valid.
	out = append(out, owner.Kind...)
	out = append(out, owner.ID[:]...)
	return binary.BigEndian.AppendUint32(out, uint32(version)) //nolint:gosec // KEK versions are positive.
}

func IsSealedV2(value []byte) bool { return len(value) >= Overhead && value[0] == FormatVersion }
