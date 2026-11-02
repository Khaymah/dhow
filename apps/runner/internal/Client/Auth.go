package client

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

const (
	maxTokenLength = 128
	expirySlack    = 30 * time.Second
)

type flight struct {
	done  chan struct{}
	token string
	err   error
}

type tokens struct {
	mu        sync.Mutex
	auth      runnerv1connect.AuthServiceClient
	id        *identity.Identity
	features  []runnerv1.RunnerFeature
	now       func() time.Time
	token     string
	renewAt   time.Time
	expiresAt time.Time
	inflight  *flight
}

func (t *tokens) get(ctx context.Context) (string, error) {
	t.mu.Lock()
	now := t.now()
	if t.token != "" && now.Before(t.renewAt) {
		token := t.token
		t.mu.Unlock()
		return token, nil
	}
	current := ""
	if t.token != "" && now.Before(t.expiresAt) {
		current = t.token
	}
	f := t.inflight
	if f == nil {
		f = &flight{done: make(chan struct{})}
		t.inflight = f
		go t.renew(context.WithoutCancel(ctx), f)
	}
	t.mu.Unlock()
	if current != "" {
		return current, nil
	}
	select {
	case <-f.done:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return f.token, f.err
}

func (t *tokens) renew(ctx context.Context, f *flight) {
	token, renewAt, expiresAt, err := t.authenticate(ctx)
	t.mu.Lock()
	if err == nil {
		t.token, t.renewAt, t.expiresAt = token, renewAt, expiresAt
	}
	t.inflight = nil
	t.mu.Unlock()
	f.token, f.err = token, err
	close(f.done)
}

func (t *tokens) invalidate(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == token {
		t.token = ""
	}
}

func (t *tokens) authenticate(ctx context.Context) (string, time.Time, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, config.RPCTimeout)
	defer cancel()
	started := t.now()
	ref := &runnerv1.RunnerRef{RunnerId: t.id.RunnerID}
	ch, err := t.auth.Challenge(ctx, connect.NewRequest(&runnerv1.ChallengeRequest{Runner: ref}))
	if err != nil {
		return "", time.Time{}, time.Time{}, mapAuthError(err)
	}
	issued := ch.Msg.GetChallenge()
	var parsed runnerv1.AuthChallenge
	if err := proto.Unmarshal(issued.GetChallengeBytes(), &parsed); err != nil || parsed.GetRunnerId() != t.id.RunnerID {
		return "", time.Time{}, time.Time{}, ErrBadChallenge
	}
	names, err := runnersig.FeatureNames(t.features)
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	message := runnersig.AuthMessage(t.id.ServerOrigin, t.id.ServerKeyVersion, issued.GetChallengeBytes())
	if len(names) > 0 {
		message = runnersig.AuthFeaturesMessage(t.id.ServerOrigin, t.id.ServerKeyVersion, issued.GetChallengeBytes(), names)
	}
	res, err := t.auth.Authenticate(ctx, connect.NewRequest(&runnerv1.AuthenticateRequest{
		Runner: ref, Challenge: issued, Signature: t.id.Sign(message), Features: t.features,
	}))
	if err != nil {
		err = mapAuthError(err)
		if Code(err) == connect.CodeUnauthenticated {
			err = errors.Join(ErrCredentials, err)
		}
		return "", time.Time{}, time.Time{}, err
	}
	token := res.Msg.GetAccessToken()
	lifetime := time.Duration(res.Msg.GetExpiresInSeconds()) * time.Second
	if !strings.HasPrefix(token, config.AccessTokenPrefix) || len(token) > maxTokenLength || lifetime <= 0 {
		return "", time.Time{}, time.Time{}, ErrBadToken
	}
	return token, started.Add(lifetime * config.TokenRenewNumerator / config.TokenRenewDivisor), started.Add(lifetime - min(expirySlack, lifetime/2)), nil
}
