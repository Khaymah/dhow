package apptest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var remoteCounter atomic.Int64

type TestRunner struct {
	T         testing.TB
	Env       *Env
	Identity  *RunnerIdentity
	RunnerID  string
	ServerKey ed25519.PublicKey
	Client    *RunnerClient
}

type Grant struct {
	Signed *runnerv1.SignedLeaseGrant
	Grant  *runnerv1.LeaseGrant
	Ref    *runnerv1.LeaseRef
}

func nextRemote() string {
	n := remoteCounter.Add(1)
	return fmt.Sprintf("198.18.%d.%d:7000", (n/250)%250, n%250+1)
}

func (e *Env) ConnectRunner(token string) *TestRunner {
	e.T.Helper()
	remote := nextRemote()
	id := NewRunnerIdentity(e.T)
	res, err := e.RunnerClient(RunnerClientOptions{Remote: remote}).Registration.Register(context.Background(), connect.NewRequest(id.RegisterRequest(token)))
	require.NoError(e.T, err)
	access, err := e.Authenticate(e.RunnerClient(RunnerClientOptions{Remote: remote}), id, res.Msg.GetRunnerId())
	require.NoError(e.T, err)
	return &TestRunner{
		T: e.T, Env: e, Identity: id, RunnerID: res.Msg.GetRunnerId(), ServerKey: res.Msg.GetServerSigningPublicKey(),
		Client: e.RunnerClient(RunnerClientOptions{Remote: remote, AccessToken: access}),
	}
}

func (e *Env) ConnectRunners(owner *Client, org uuid.UUID, labels []string, n int) []*TestRunner {
	e.T.Helper()
	out := make([]*TestRunner, 0, n)
	for len(out) < n {
		uses := min(n-len(out), 50)
		token, _ := e.CreateRegistrationToken(owner, org, RegistrationTokenSpec{Labels: labels, MaxUses: uses})
		for range uses {
			out = append(out, e.ConnectRunner(token))
		}
	}
	return out
}

func (r *TestRunner) Lease(ctx context.Context, freeSlots, wait int32) (*Grant, error) {
	res, err := r.Client.Lease.Lease(ctx, connect.NewRequest(&runnerv1.LeaseRequest{FreeSlots: freeSlots, WaitSeconds: wait}))
	if err != nil {
		return nil, err
	}
	signed := res.Msg.GetGrant()
	if signed == nil {
		return nil, nil
	}
	if !ed25519.Verify(r.ServerKey, runnersig.GrantMessage(signed.GetGrantBytes()), signed.GetSignature()) {
		return nil, fmt.Errorf("the lease grant is not signed by the server")
	}
	var g runnerv1.LeaseGrant
	if err := proto.Unmarshal(signed.GetGrantBytes(), &g); err != nil {
		return nil, err
	}
	if g.GetRunnerId() != r.RunnerID {
		return nil, fmt.Errorf("the lease grant names runner %s", g.GetRunnerId())
	}
	return &Grant{Signed: signed, Grant: &g, Ref: &runnerv1.LeaseRef{LeaseId: g.GetLeaseId(), FencingToken: g.GetFencingToken()}}, nil
}

func (r *TestRunner) MustLease(wait int32) *Grant {
	r.T.Helper()
	g, err := r.Lease(context.Background(), 1, wait)
	require.NoError(r.T, err)
	require.NotNil(r.T, g, "expected a lease")
	return g
}

func (r *TestRunner) Proof(g *Grant) []byte {
	digest := runnersig.GrantDigest(g.Signed.GetGrantBytes())
	return ed25519.Sign(r.Identity.Signing, runnersig.StartMessage(g.Ref.GetLeaseId(), g.Ref.GetFencingToken(), digest))
}

func (r *TestRunner) Start(g *Grant) (*runnerv1.StartResponse, error) {
	res, err := r.Client.Lease.Start(context.Background(), connect.NewRequest(&runnerv1.StartRequest{Lease: g.Ref, PopSignature: r.Proof(g)}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (r *TestRunner) Heartbeat(ref *runnerv1.LeaseRef) (*runnerv1.LeaseControl, error) {
	res, err := r.Client.Lease.Heartbeat(context.Background(), connect.NewRequest(&runnerv1.HeartbeatRequest{Lease: ref}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetControl(), nil
}

func (r *TestRunner) BeginStep(ref *runnerv1.LeaseRef, index int32, mutating bool) error {
	_, err := r.Client.Lease.BeginStep(context.Background(), connect.NewRequest(&runnerv1.BeginStepRequest{Lease: ref, Index: index, Mutating: mutating}))
	return err
}

func (r *TestRunner) EndStep(ref *runnerv1.LeaseRef, index, exitCode int32) error {
	_, err := r.Client.Lease.EndStep(context.Background(), connect.NewRequest(&runnerv1.EndStepRequest{Lease: ref, Index: index, ExitCode: exitCode}))
	return err
}

func (r *TestRunner) EndStepWith(ref *runnerv1.LeaseRef, index, exitCode int32, reason runnerv1.FailureReason) error {
	_, err := r.Client.Lease.EndStep(context.Background(), connect.NewRequest(&runnerv1.EndStepRequest{Lease: ref, Index: index, ExitCode: exitCode, FailureReason: reason}))
	return err
}

func Digest(g *Grant, result runnerv1.AttemptResult, exitCode int32, reason runnerv1.FailureReason) []byte {
	return runnersig.CompletionDigest(g.Grant.GetAttemptId(), g.Ref.GetFencingToken(), int32(result), exitCode, int32(reason))
}

func (r *TestRunner) Complete(g *Grant, result runnerv1.AttemptResult, exitCode int32, reason runnerv1.FailureReason) error {
	return r.CompleteWith(g, result, exitCode, reason, Digest(g, result, exitCode, reason))
}

func (r *TestRunner) CompleteWith(g *Grant, result runnerv1.AttemptResult, exitCode int32, reason runnerv1.FailureReason, digest []byte) error {
	_, err := r.Client.Lease.Complete(context.Background(), connect.NewRequest(&runnerv1.CompleteRequest{
		Lease: g.Ref, Result: result, ExitCode: exitCode, FailureReason: reason, Digest: digest,
	}))
	return err
}

type SeedRunnerSpec struct {
	Labels    []string
	Isolation []string
	Capacity  int
	Features  []string
}

func (e *Env) SeedConnectedRunner(org, creator uuid.UUID, spec SeedRunnerSpec) *TestRunner {
	e.T.Helper()
	if len(spec.Isolation) == 0 {
		spec.Isolation = []string{"container"}
	}
	if spec.Capacity == 0 {
		spec.Capacity = 2
	}
	if spec.Labels == nil {
		spec.Labels = []string{}
	}
	if spec.Features == nil {
		spec.Features = []string{}
	}
	capabilities := []string{"dhow.os=linux", "dhow.arch=amd64"}
	for _, l := range spec.Isolation {
		capabilities = append(capabilities, "dhow.isolation="+l)
	}
	ctx := context.Background()
	token := e.SeedRegistrationToken(org, creator)
	id := NewRunnerIdentity(e.T)
	var runnerID uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "Runners" ("OrganizationId", "RegistrationTokenId", "Name", "Labels", "CapabilityLabels", "Os", "Arch", "Version",
		 "IsolationLevels", "Capacity", "SigningPublicKey", "EncryptionPublicKey", "Features") VALUES ($1, $2, 'seeded', $3, $4, 'linux', 'amd64', '0.1.0',
		 $5, $6, $7, $8, $9) RETURNING "Id"`, org, token, spec.Labels, capabilities, spec.Isolation, spec.Capacity,
		id.SigningPublicKey(), id.EncryptionPublicKey(), spec.Features).Scan(&runnerID))
	access, hash, err := runnerauth.NewAccessToken()
	require.NoError(e.T, err)
	nonce := make([]byte, runnersig.NonceSize)
	_, err = rand.Read(nonce)
	require.NoError(e.T, err)
	now := e.Clock.Now()
	_, err = e.Pool.Exec(ctx,
		`INSERT INTO "RunnerSessions" ("RunnerId", "TokenHash", "ChallengeNonce", "KeyVersion", "ExpiresAt", "CreatedAt", "UpdatedAt")
		 VALUES ($1, $2, $3, 1, $4, $5, $5)`, runnerID, hash, nonce, now.Add(config.RunnerAccessTokenTTL), now)
	require.NoError(e.T, err)
	return &TestRunner{
		T: e.T, Env: e, Identity: id, RunnerID: runnerID.String(), ServerKey: e.ServerSigningKey(),
		Client: e.RunnerClient(RunnerClientOptions{Remote: nextRemote(), AccessToken: access}),
	}
}

func (e *Env) ServerSigningKey() ed25519.PublicKey {
	e.T.Helper()
	keys, err := runnerauth.DeriveKeys(e.Config.SecretKey)
	require.NoError(e.T, err)
	return keys.SigningPublicKey()
}
