package e2e_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	shaHead = "1111111111111111111111111111111111111111"
	shaNext = "2222222222222222222222222222222222222222"
	shaTag  = "3333333333333333333333333333333333333333"

	shipOnArtifact = `version: 1
on:
  push:
    branches: [main]
  artifact:
    names: [web]
    refs: [main, "release/**", "v*"]
jobs:
  ship:
    image: IMAGE
    ingested: [web]
    steps:
      - run: ls
`
	pushOnly = `version: 1
on:
  push:
    branches: [main]
jobs:
  build:
    image: IMAGE
    steps:
      - run: make
`
)

type world struct {
	env      *apptest.Env
	owner    *apptest.Client
	ownerID  uuid.UUID
	org      uuid.UUID
	project  uuid.UUID
	repo     uuid.UUID
	defs     *apptest.FakeDefinitions
	runner   *apptest.TestRunner
	password string
	token    apptest.IngestToken
}

func newWorld(t *testing.T) world {
	t.Helper()
	return newWorldWith(t, nil)
}

func newWorldWith(t *testing.T, profile func(*config.Config)) world {
	t.Helper()
	defs := apptest.NewFakeDefinitions()
	env := apptest.NewWithOptions(t, apptest.Options{Definitions: defs, Profile: profile})
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	ownerID := uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	runner := env.SeedConnectedRunner(org, ownerID, apptest.SeedRunnerSpec{
		Capacity: 8, Features: []string{"artifacts", "cache", "checkout", "plugins"},
	})
	w := world{
		env: env, owner: owner, ownerID: ownerID, org: org, project: project, repo: env.SeedRepository(org, project, ownerID),
		defs: defs, runner: runner, password: d.Password, token: env.SeedIngestToken(project, ownerID, apptest.IngestSeed{}),
	}
	w.defs.Put(shaHead, pipeline(shipOnArtifact))
	w.defs.Put(shaNext, pipeline(pushOnly))
	w.push(t, "refs/heads/main", shaHead)
	return w
}

func pipeline(body string) string { return strings.ReplaceAll(body, "IMAGE", apptest.TestImage) }

func (w world) push(t *testing.T, ref, sha string) uuid.UUID {
	t.Helper()
	got := w.env.Deliver(triggers.Trigger{
		OrganizationID: w.org, ProjectID: w.project, RepositoryID: w.repo, DeliveryID: uuid.New(), Provider: "generic",
		Event: triggers.EventPush, Ref: ref, Sha: sha, Actor: "sara",
	})
	require.True(t, got.Created)
	w.env.StartRuns()
	return got.RunID
}

func (w world) upload(body []byte, edit func(*apptest.IngestUpload)) apptest.Response {
	u := apptest.IngestUpload{Token: w.token.Value, Body: body, Name: "web", Ref: "main", Sha: shaNext}
	if edit != nil {
		edit(&u)
	}
	return w.env.IngestPut(u)
}

func (w world) archive() []byte {
	return apptest.TarGz(map[string]string{"index.html": "<h1>hello</h1>", "assets/app.js": "console.log(1)"})
}

func (w world) runsOfKind(t *testing.T, trigger string) int {
	t.Helper()
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "PipelineRuns" WHERE "ProjectId" = $1 AND "Trigger" = $2`, w.project, trigger).Scan(&n))
	return n
}

func (w world) lease(t *testing.T) *apptest.Grant {
	t.Helper()
	for range 50 {
		g, err := w.runner.Lease(context.Background(), 1, 0)
		if reason(err) == "LEASE_RATE_LIMITED" {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		require.NoError(t, err)
		return g
	}
	t.Fatal("the runner stayed rate limited")
	return nil
}

func (w world) next(t *testing.T) *apptest.Grant {
	t.Helper()
	g := w.lease(t)
	require.NotNil(t, g, "expected a lease")
	_, err := w.runner.Start(g)
	require.NoError(t, err)
	return g
}

func (w world) succeed(t *testing.T, g *apptest.Grant) {
	t.Helper()
	require.NoError(t, w.runner.Complete(g, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED))
	w.env.StartRuns()
}

func reason(err error) string {
	var ce *connect.Error
	if err == nil {
		return ""
	}
	if errors.As(err, &ce) {
		return ce.Message()
	}
	return err.Error()
}
