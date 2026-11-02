package runnerauth

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/netip"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNoSession = errors.New("no active runner session")
	ErrRevoked   = errors.New("runner is revoked")
)

type Principal struct {
	RunnerID            uuid.UUID
	OrganizationID      uuid.UUID
	SessionID           uuid.UUID
	Name                string
	Labels              []string
	Capacity            int
	IsolationLevels     []string
	SigningPublicKey    ed25519.PublicKey
	EncryptionPublicKey []byte
	KeyVersion          int32
	Disabled            bool
	Features            []string
}

type Verifier interface {
	Resolve(ctx context.Context, tokenHash []byte, now time.Time) (Principal, error)
}

type RequestMeta struct {
	IP            netip.Addr
	RunnerVersion string
}

func (m RequestMeta) IPString() string {
	if !m.IP.IsValid() {
		return ""
	}
	return m.IP.String()
}

type principalKey struct{}

type metaKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func WithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

func RequestMetaFrom(ctx context.Context) RequestMeta {
	m, _ := ctx.Value(metaKey{}).(RequestMeta)
	return m
}
