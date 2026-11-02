package e2e_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	shaA = "1111111111111111111111111111111111111111"
	shaB = "2222222222222222222222222222222222222222"
	shaC = "3333333333333333333333333333333333333333"

	runnerPool = 12

	succeeded = runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED
	failed    = runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED
)

type fakeImages struct {
	mu     sync.Mutex
	pinned map[string]string
	calls  int
}

func (f *fakeImages) Resolve(_ context.Context, image string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	v, ok := f.pinned[image]
	if !ok {
		return "", pipelines.ErrImageNotFound
	}
	return v, nil
}

func (f *fakeImages) move(image, pinned string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pinned[image] = pinned
}

type world struct {
	env     *apptest.Env
	owner   *apptest.Client
	ownerID uuid.UUID
	org     uuid.UUID
	project uuid.UUID
	repo    uuid.UUID
	defs    *apptest.FakeDefinitions
	images  *fakeImages
	runners []*apptest.TestRunner
	next    *atomic.Int64
}

func newWorld(t *testing.T) world {
	t.Helper()
	return newWorldWith(t, apptest.Options{})
}

func newWorldWith(t *testing.T, o apptest.Options) world {
	t.Helper()
	defs := apptest.NewFakeDefinitions()
	images := &fakeImages{pinned: map[string]string{}}
	o.Definitions = defs
	if o.Images == nil {
		o.Images = images
	}
	env := apptest.NewWithOptions(t, o)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	ownerID := uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	runners := make([]*apptest.TestRunner, 0, runnerPool)
	for range runnerPool {
		runners = append(runners, env.SeedConnectedRunner(org, ownerID, apptest.SeedRunnerSpec{Capacity: 8}))
	}
	return world{
		env: env, owner: owner, ownerID: ownerID, org: org, project: project, repo: env.SeedRepository(org, project, ownerID),
		defs: defs, images: images, runners: runners, next: &atomic.Int64{},
	}
}

func (w world) finish(result runnerv1.AttemptResult) (string, bool) {
	r := w.runners[int(w.next.Add(1))%len(w.runners)]
	return r.FinishNext(result)
}

func pipeline(body string) string {
	return strings.ReplaceAll(body, "IMAGE", apptest.TestImage)
}

func (w world) push(ref, sha string) uuid.UUID {
	w.env.T.Helper()
	got := w.env.Deliver(w.trigger(triggers.EventPush, ref, sha))
	require.True(w.env.T, got.Created)
	return got.RunID
}

func (w world) trigger(event, ref, sha string) triggers.Trigger {
	return triggers.Trigger{
		OrganizationID: w.org, ProjectID: w.project, RepositoryID: w.repo, DeliveryID: uuid.New(), Provider: "generic",
		Event: event, Ref: ref, Sha: sha, Actor: "sara",
	}
}

func (w world) runPath(run uuid.UUID, rest string) string {
	return apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/runs/"+run.String()+rest)
}

type runView struct {
	Run struct {
		Id, State, FailureReason, Trigger, Ref, DefinitionSha, DefinitionDigest string
		Sha                                                                     *string
		ConcurrencyGroup                                                        *string
	}
	Jobs []struct {
		Key, JobId, State string
		FailureReason     *string
		CurrentJobId      *string
		Attempts          int
		Matrix            map[string]string
	}
	Issues []struct {
		Code, Severity, Path string
		Line, Column         int
	}
	ImagePins []struct{ Image, Pinned string }
}

func (w world) view(t *testing.T, run uuid.UUID) runView {
	t.Helper()
	res := w.owner.Get(w.runPath(run, ""))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var v runView
	res.Decode(t, &v)
	return v
}

func (w world) post(t *testing.T, c *apptest.Client, run uuid.UUID, action string) apptest.Response {
	t.Helper()
	return c.Do(apptest.Request{Method: http.MethodPost, Path: w.runPath(run, "/"+action)})
}

func (w world) drain(t *testing.T, result runnerv1.AttemptResult) []string {
	t.Helper()
	var keys []string
	for {
		key, ok := w.finish(result)
		if !ok {
			return keys
		}
		keys = append(keys, key)
	}
}

func (w world) finishN(t *testing.T, n int, result runnerv1.AttemptResult) []string {
	t.Helper()
	keys := make([]string, 0, n)
	for range n {
		key, ok := w.finish(result)
		require.True(t, ok, "expected a queued job")
		keys = append(keys, key)
	}
	return keys
}

func replace(s, old, v string) string { return strings.ReplaceAll(s, old, v) }

func repeat(s string, n int) string { return strings.Repeat(s, n) }

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
