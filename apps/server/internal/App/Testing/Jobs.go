package apptest

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const TestDeploySSHImage = "ghcr.io/khaymah/dhow-deploy-ssh@sha256:03fddb3b486b758f338f8999afbf04ced985d74825963dd28b92e650645e6778"

const TestImage = "registry.example.com/dhow/test@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type FakeSecrets struct {
	mu       sync.Mutex
	values   map[string]string
	resolved int
}

func NewFakeSecrets(values map[string]string) *FakeSecrets {
	return &FakeSecrets{values: values}
}

func (f *FakeSecrets) Accepts(_ context.Context, _ jobs.Origin, names []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range names {
		if _, ok := f.values[n]; !ok {
			return jobs.ErrSecretsUnavailable
		}
	}
	return nil
}

func (f *FakeSecrets) Resolve(_ context.Context, req jobs.SecretRequest) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved++
	out := make(map[string][]byte, len(req.Names))
	for _, n := range req.Names {
		v, ok := f.values[n]
		if !ok {
			return nil, jobs.ErrSecretsUnavailable
		}
		out[n] = []byte(v)
	}
	return out, nil
}

func (f *FakeSecrets) Forget(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, name)
}

func (f *FakeSecrets) Resolved() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resolved
}

func startListener(t *testing.T, wired *app.Wired) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		wired.Jobs.RunListener(ctx)
	}()
	stopSettings := app.StartSettings(ctx, wired.Settings)
	stopEvents := app.StartEvents(ctx, wired.Events)
	t.Cleanup(func() {
		cancel()
		stopSettings()
		stopEvents()
		<-done
	})
}

type JobSpec map[string]any

func Steps(steps ...map[string]any) []map[string]any { return steps }

func SimpleSpec(steps ...map[string]any) JobSpec {
	if len(steps) == 0 {
		steps = Steps(map[string]any{"Name": "build", "Run": "make build"}, map[string]any{"Name": "test", "Run": "make test"})
	}
	return JobSpec{"Image": TestImage, "Steps": steps}
}

func (e *Env) CreateJob(c *Client, org uuid.UUID, name string, spec JobSpec, extra map[string]any) Response {
	e.T.Helper()
	body := map[string]any{"Name": name, "Spec": spec}
	for k, v := range extra {
		body[k] = v
	}
	return c.Do(Request{Method: http.MethodPost, Path: "/organizations/" + org.String() + "/jobs", Body: body})
}

func (e *Env) CreateJobOK(c *Client, org uuid.UUID, spec JobSpec, extra map[string]any) uuid.UUID {
	e.T.Helper()
	res := e.CreateJob(c, org, "job", spec, extra)
	require.Equal(e.T, http.StatusCreated, res.Status, string(res.Raw))
	var data struct{ Job struct{ Id string } }
	res.Decode(e.T, &data)
	return uuid.MustParse(data.Job.Id)
}

func (e *Env) SeedJob(org, creator uuid.UUID) uuid.UUID {
	e.T.Helper()
	spec, err := jobspec.Prepare(&runnerv1.JobSpec{Image: TestImage, Steps: []*runnerv1.Step{{Name: "seeded", Run: "true"}}})
	require.NoError(e.T, err)
	raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(spec)
	require.NoError(e.T, err)
	digest, err := jobspec.Digest(spec)
	require.NoError(e.T, err)
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "Jobs" ("OrganizationId", "CreatedByUserId", "Name", "Spec", "SpecDigest", "Image", "Isolation", "QueueDeadline")
		 VALUES ($1, $2, 'seeded', $3, $4, $5, 'container', now() + interval '1 hour') RETURNING "Id"`,
		org, creator, raw, digest, TestImage).Scan(&id))
	return id
}

func (e *Env) JobState(id uuid.UUID) (string, string) {
	e.T.Helper()
	var state, reason string
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`SELECT "State", "FailureReason" FROM "Jobs" WHERE "Id" = $1`, id).Scan(&state, &reason))
	return state, reason
}

func (e *Env) ExpireLease(leaseID string) {
	e.T.Helper()
	_, err := e.Pool.Exec(context.Background(),
		`UPDATE "Leases" SET "ExpiresAt" = now() - interval '1 minute' WHERE "Id" = $1`, leaseID)
	require.NoError(e.T, err)
}

func (e *Env) Reconcile() jobs.ReconcileResult {
	e.T.Helper()
	res, err := e.Wired.Jobs.Reconcile(context.Background())
	require.NoError(e.T, err)
	return res
}
