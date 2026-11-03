package e2e_test

import (
	"context"
	"errors"
	"strconv"
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
	shaA = "1111111111111111111111111111111111111111"
	shaB = "2222222222222222222222222222222222222222"
	shaC = "3333333333333333333333333333333333333333"

	buildAndCheck = `version: 1
on:
  push:
    branches: ["**"]
  pull_request: {}
  manual: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: make build
    artifacts:
      - name: web
        paths: [dist]
    cache:
      key: deps-v1
      paths: [.npm]
  deploy-check:
    needs: [build]
    image: IMAGE
    steps:
      - run: ls dist
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
}

func (w world) tenant(t *testing.T) world {
	t.Helper()
	org := w.env.SeedOrganization("Other tenant", "other-"+strings.ToLower(uuid.NewString()[:8]))
	owner := w.env.SeedUser(org, "owner-"+uuid.NewString()[:8]+"@example.com", "a long password for the other owner", "owner")
	project := w.env.SeedProject(org, owner)
	runner := w.env.SeedConnectedRunner(org, owner, apptest.SeedRunnerSpec{
		Capacity: 8, Features: []string{"artifacts", "cache", "checkout", "plugins"},
	})
	return world{
		env: w.env, owner: w.env.SeedSession(owner), ownerID: owner, org: org, project: project,
		repo: w.env.SeedRepository(org, project, owner), defs: w.defs, runner: runner,
	}
}

func (w world) scope(s string) string { return "repo:" + w.repo.String() + "/" + s }

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
		defs: defs, runner: runner, password: d.Password,
	}
	w.defs.Put(shaA, pipeline(buildAndCheck))
	w.defs.Put(shaB, pipeline(buildAndCheck))
	w.defs.Put(shaC, pipeline(buildAndCheck))
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

func (w world) pullRequest(t *testing.T, number int, sha string, origin triggers.PullOrigin) uuid.UUID {
	t.Helper()
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Projects" SET "ForkPolicy" = 'automatic' WHERE "Id" = $1`, w.project)
	require.NoError(t, err)
	got := w.env.Deliver(triggers.Trigger{
		OrganizationID: w.org, ProjectID: w.project, RepositoryID: w.repo, DeliveryID: uuid.New(), Provider: "generic",
		Event: triggers.EventPullRequest, Ref: "refs/pull/" + strconv.Itoa(number) + "/head", Sha: sha, Actor: "sara",
		BaseRef: "refs/heads/main", BaseSha: shaA, PrNumber: number, Origin: origin,
	})
	require.True(t, got.Created)
	w.env.StartRuns()
	return got.RunID
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

func (w world) drain(t *testing.T) {
	t.Helper()
	for {
		g := w.lease(t)
		if g == nil {
			return
		}
		_, err := w.runner.Start(g)
		require.NoError(t, err)
		w.succeed(t, g)
	}
}

func reason(err error) string {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Message()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}
