package apptest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"golang.org/x/crypto/ssh"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type FakeDefinitions struct {
	mu      sync.Mutex
	files   map[string][]byte
	blocked map[string]bool
	calls   int
	err     error
}

func (f *FakeDefinitions) Block(sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked == nil {
		f.blocked = map[string]bool{}
	}
	f.blocked[sha] = true
}

func NewFakeDefinitions() *FakeDefinitions { return &FakeDefinitions{files: map[string][]byte{}} }

func (f *FakeDefinitions) Put(sha string, source string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[sha] = []byte(source)
}

func (f *FakeDefinitions) FailWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *FakeDefinitions) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *FakeDefinitions) Fetch(ctx context.Context, r pipelines.DefinitionRequest) (pipelines.Fetched, error) {
	f.mu.Lock()
	f.calls++
	if f.blocked[r.Sha] {
		f.mu.Unlock()
		<-ctx.Done()
		return pipelines.Fetched{}, ctx.Err()
	}
	defer f.mu.Unlock()
	if f.err != nil {
		return pipelines.Fetched{}, f.err
	}
	src, ok := f.files[r.Sha]
	if !ok {
		return pipelines.Fetched{}, nil
	}
	return pipelines.Fetched{Source: append([]byte(nil), src...), Sha: r.Sha, Found: true}, nil
}

func (e *Env) SeedRepository(org, project, creator uuid.UUID) uuid.UUID {
	e.T.Helper()
	var id uuid.UUID
	hook := uuid.New()
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "Repositories" ("OrganizationId", "ProjectId", "Provider", "FullName", "CloneUrl", "Transport", "HookId",
		 "WebhookSecretSealed", "CreatedByUserId")
		 VALUES ($1, $2, 'generic', 'team/app', 'https://git.example.com/team/app.git', 'https', $3, '\x01', $4) RETURNING "Id"`,
		org, project, hex32(hook), creator).Scan(&id))
	return id
}

func hex32(id uuid.UUID) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 32)
	for _, b := range id {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}

func (e *Env) SeedRun(project uuid.UUID, state string) uuid.UUID {
	e.T.Helper()
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "Number", "Trigger", "Ref", "DefinitionPath", "State")
		 SELECT p."OrganizationId", p."Id", (SELECT COALESCE(max("Number"), 0) + 1 FROM "PipelineRuns" WHERE "ProjectId" = p."Id"),
		        'manual', 'refs/heads/main', '.dhow.yml', $2
		 FROM "Projects" p WHERE p."Id" = $1 RETURNING "Id"`,
		project, state).Scan(&id))
	return id
}

func (e *Env) SeedPipelineJob(org, project, creator uuid.UUID) uuid.UUID {
	e.T.Helper()
	run := e.SeedRun(project, "pending")
	ctx := context.Background()
	var runJob uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position") VALUES ($1, $2, 'build', 'build', 'build', 0)
		 RETURNING "Id"`, org, run).Scan(&runJob))
	job := e.SeedJob(org, creator)
	_, err := e.Pool.Exec(ctx, `UPDATE "Jobs" SET "OriginKind" = 'pipeline', "OriginId" = $2 WHERE "Id" = $1`, job, runJob)
	require.NoError(e.T, err)
	return job
}

type RunJobView struct {
	State  string
	Reason string
	Job    *uuid.UUID
}

func (e *Env) Deliver(t triggers.Trigger) triggers.Accepted {
	e.T.Helper()
	var out triggers.Accepted
	require.NoError(e.T, dbtx.Run(context.Background(), e.Pool, func(q *sqlc.Queries) error {
		var err error
		out, err = e.Wired.Pipelines.Deliver(context.Background(), q, t)
		return err
	}))
	return out
}

func (e *Env) StartRuns() {
	e.T.Helper()
	_, err := e.Wired.Pipelines.StartPending(context.Background())
	require.NoError(e.T, err)
	_, err = e.Wired.Pipelines.StartQueued(context.Background())
	require.NoError(e.T, err)
}

func (e *Env) RunState(id uuid.UUID) (string, string) {
	e.T.Helper()
	var state, reason string
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`SELECT "State", "FailureReason" FROM "PipelineRuns" WHERE "Id" = $1`, id).Scan(&state, &reason))
	return state, reason
}

func (e *Env) RunJobs(run uuid.UUID) map[string]RunJobView {
	e.T.Helper()
	rows, err := e.Pool.Query(context.Background(),
		`SELECT "Key", "State", "FailureReason", "CurrentJobId" FROM "RunJobs" WHERE "RunId" = $1`, run)
	require.NoError(e.T, err)
	defer rows.Close()
	out := map[string]RunJobView{}
	for rows.Next() {
		var key string
		var v RunJobView
		require.NoError(e.T, rows.Scan(&key, &v.State, &v.Reason, &v.Job))
		out[key] = v
	}
	require.NoError(e.T, rows.Err())
	return out
}

func (e *Env) RunJobKey(job uuid.UUID) string {
	e.T.Helper()
	var key string
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`SELECT rj."Key" FROM "Jobs" j JOIN "RunJobs" rj ON rj."Id" = j."OriginId" WHERE j."Id" = $1`, job).Scan(&key))
	return key
}

func (r *TestRunner) FinishNext(result runnerv1.AttemptResult) (string, bool) {
	r.T.Helper()
	g, err := r.Lease(context.Background(), 1, 0)
	require.NoError(r.T, err)
	if g == nil {
		return "", false
	}
	key := r.Env.RunJobKey(uuid.MustParse(g.Grant.GetJobId()))
	_, err = r.Start(g)
	require.NoError(r.T, err)
	reason := runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED
	exit := int32(0)
	if result == runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED {
		reason, exit = runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, 1
	}
	require.NoError(r.T, r.Complete(g, result, exit, reason))
	return key, true
}

type SSHRepository struct {
	ID         uuid.UUID
	PrivateKey []byte
	KnownHost  string
	CloneURL   string
}

func (e *Env) SeedSSHRepository(org, project, creator uuid.UUID) SSHRepository {
	e.T.Helper()
	_, deployPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(e.T, err)
	block, err := ssh.MarshalPrivateKey(deployPriv, config.DeployKeyComment)
	require.NoError(e.T, err)
	private := pem.EncodeToMemory(block)
	deployPub, err := ssh.NewPublicKey(deployPriv.Public())
	require.NoError(e.T, err)
	hostPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(e.T, err)
	hostKey, err := ssh.NewPublicKey(hostPub)
	require.NoError(e.T, err)
	hostLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostKey)))
	id := uuid.New()
	sealer, err := appcrypto.NewSealer(e.Config.SecretKey, config.DeployKeySealLabel)
	require.NoError(e.T, err)
	sealed, err := sealer.Seal(private, []byte("deploy-key:"+id.String()))
	require.NoError(e.T, err)
	out := SSHRepository{ID: id, PrivateKey: private, KnownHost: "git.example.com " + hostLine, CloneURL: "ssh://git@git.example.com/team/app.git"}
	_, err = e.Pool.Exec(context.Background(),
		`INSERT INTO "Repositories" ("Id", "OrganizationId", "ProjectId", "Provider", "FullName", "CloneUrl", "Transport", "HookId",
		 "WebhookSecretSealed", "HostKey", "HostKeyConfirmedAt", "HostKeyConfirmedByUserId", "DeployKeyPublic", "DeployKeySealed",
		 "CreatedByUserId")
		 VALUES ($1, $2, $3, 'generic', 'team/app', $4, 'ssh', $5, '\x01', $6, now(), $7, $8, $9, $7)`,
		id, org, project, out.CloneURL, hex32(uuid.New()), hostLine, creator,
		strings.TrimSpace(string(ssh.MarshalAuthorizedKey(deployPub))), sealed)
	require.NoError(e.T, err)
	return out
}

func DiscoveryLog(source string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(source))
	var b strings.Builder
	for len(encoded) > 0 {
		n := min(76, len(encoded))
		b.WriteString("dhow-definition:" + encoded[:n] + "\n")
		encoded = encoded[n:]
	}
	return b.String()
}

func DiscoveryOutputs(source string) map[string]string {
	sum := sha256.Sum256([]byte(source))
	return map[string]string{"dhow_found": "true", "dhow_sha256": hex.EncodeToString(sum[:])}
}

func (r *TestRunner) Discover(source string) (*Grant, bool) {
	r.T.Helper()
	g, err := r.Lease(context.Background(), 1, 0)
	require.NoError(r.T, err)
	if g == nil {
		return nil, false
	}
	_, err = r.Start(g)
	require.NoError(r.T, err)
	require.NoError(r.T, r.BeginStep(g.Ref, 0, false))
	step := int32(0)
	r.MustAppend(g.Ref, &runnerv1.LogChunk{Seq: 1, Stream: runnerv1.LogStream_LOG_STREAM_STDOUT, StepIndex: &step, Data: []byte(DiscoveryLog(source))})
	require.NoError(r.T, r.EndStep(g.Ref, 0, 0))
	require.NoError(r.T, r.CompleteOutputs(g, DiscoveryOutputs(source)))
	return g, true
}

func (r *TestRunner) CompleteOutputs(g *Grant, outputs map[string]string) error {
	result := runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED
	reason := runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED
	_, err := r.Client.Lease.Complete(context.Background(), connect.NewRequest(&runnerv1.CompleteRequest{
		Lease: g.Ref, Result: result, FailureReason: reason, Digest: Digest(g, result, 0, reason), Outputs: outputs,
	}))
	return err
}
