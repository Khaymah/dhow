package keys

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sync"
)

const (
	checkLabel   = "dhow/kek-check/v1"
	CheckBytes   = sha256.Size
	gcmNonce     = 12
	gcmTag       = 16
	WrappedBytes = gcmNonce + KeyBytes + gcmTag
)

type KeyProvider interface {
	Name() string
	Versions() []int32
	CheckValue(ctx context.Context, version int32) ([]byte, error)
	Wrap(ctx context.Context, version int32, dek, aad []byte) ([]byte, error)
	Unwrap(ctx context.Context, version int32, wrapped, aad []byte) ([]byte, error)
}

type FileProvider struct {
	ring  *Keyring
	name  string
	aeads map[int32]cipher.AEAD
}

func NewFileProvider(ring *Keyring) (*FileProvider, error) {
	return newProvider(ring, FileProviderName)
}

func newProvider(ring *Keyring, name string) (*FileProvider, error) {
	p := &FileProvider{ring: ring, name: name, aeads: make(map[int32]cipher.AEAD, len(ring.versions))}
	for _, v := range ring.versions {
		key, _ := ring.key(v)
		aead, err := newAEAD(key)
		if err != nil {
			return nil, err
		}
		p.aeads[v] = aead
	}
	return p, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return aead, nil
}

func (p *FileProvider) Name() string { return p.name }

func (p *FileProvider) Versions() []int32 { return p.ring.Versions() }

func (p *FileProvider) Keyring() *Keyring { return p.ring }

func (p *FileProvider) CheckValue(_ context.Context, version int32) ([]byte, error) {
	key, ok := p.ring.key(version)
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKek, version)
	}
	return CheckValueOf(key), nil
}

func CheckValueOf(key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(checkLabel))
	return mac.Sum(nil)
}

func (p *FileProvider) Wrap(_ context.Context, version int32, dek, aad []byte) ([]byte, error) {
	aead, ok := p.aeads[version]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKek, version)
	}
	if len(dek) != KeyBytes {
		return nil, fmt.Errorf("a data key must be %d bytes", KeyBytes)
	}
	out := make([]byte, gcmNonce, WrappedBytes)
	if _, err := rand.Read(out); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	return aead.Seal(out, out[:gcmNonce], dek, aad), nil
}

func (p *FileProvider) Unwrap(_ context.Context, version int32, wrapped, aad []byte) ([]byte, error) {
	aead, ok := p.aeads[version]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKek, version)
	}
	if len(wrapped) != WrappedBytes {
		return nil, ErrWrappedSize
	}
	dek, err := aead.Open(nil, wrapped[:gcmNonce], wrapped[gcmNonce:], aad)
	if err != nil {
		return nil, ErrSealed
	}
	return dek, nil
}

type Fake struct {
	*FileProvider

	mu        sync.Mutex
	onWrap    func() error
	onUnwrap  func() error
	wrapCalls int
}

func NewFake(versions ...int32) (*Fake, error) {
	ring, err := GenerateKeyring(versions...)
	if err != nil {
		return nil, err
	}
	return NewFakeFrom(ring)
}

func NewFakeFrom(ring *Keyring) (*Fake, error) {
	p, err := newProvider(ring, "fake")
	if err != nil {
		return nil, err
	}
	return &Fake{FileProvider: p}, nil
}

func (f *Fake) BeforeWrap(fn func() error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onWrap = fn
}

func (f *Fake) BeforeUnwrap(fn func() error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onUnwrap = fn
}

func (f *Fake) WrapCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.wrapCalls
}

func (f *Fake) Wrap(ctx context.Context, version int32, dek, aad []byte) ([]byte, error) {
	f.mu.Lock()
	f.wrapCalls++
	hook := f.onWrap
	f.mu.Unlock()
	if hook != nil {
		if err := hook(); err != nil {
			return nil, err
		}
	}
	return f.FileProvider.Wrap(ctx, version, dek, aad)
}

func (f *Fake) Unwrap(ctx context.Context, version int32, wrapped, aad []byte) ([]byte, error) {
	f.mu.Lock()
	hook := f.onUnwrap
	f.mu.Unlock()
	if hook != nil {
		if err := hook(); err != nil {
			return nil, err
		}
	}
	return f.FileProvider.Unwrap(ctx, version, wrapped, aad)
}
