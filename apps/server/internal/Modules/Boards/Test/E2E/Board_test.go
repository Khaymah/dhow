package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

type progress struct {
	ReleaseId                                   string
	Hosts, HostsDone, BatchSize, Batch, Batches int
}

type release struct{ ReleaseId, Kind, State, RollbackReason string }

type boardTarget struct {
	Id, Name, Kind, CurrentReleaseId, CurrentDigest string
	Hosts                                           int
	Last                                            *release
	Running                                         *progress
	Partial, AutoRolledBack, RollbackFailed         bool
}

type boardEnvironment struct {
	Id, Name         string
	Paused           bool
	PendingApprovals int
	Targets          []boardTarget
}

type board struct {
	Environments []boardEnvironment
	Truncated    bool
}

type world struct {
	env     *apptest.Env
	org     uuid.UUID
	owner   *apptest.Client
	project uuid.UUID
}

func newWorld(t *testing.T) world {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	return world{env: env, org: org, owner: owner, project: env.SeedProject(org, uuid.MustParse(owner.UserID))}
}

func (w world) path() string {
	return "/organizations/" + w.org.String() + "/projects/" + w.project.String() + "/deploy-board"
}

func (w world) get(t *testing.T, headers map[string]string) (apptest.Response, board) {
	t.Helper()
	res := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: w.path(), Headers: headers})
	var out board
	if res.Status == http.StatusOK {
		require.NoError(t, json.Unmarshal(res.Data, &out), string(res.Raw))
	}
	return res, out
}

func (w world) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := w.env.Pool.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
}

func (w world) deployment(t *testing.T, environment uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`INSERT INTO "Deployments" ("OrganizationId", "EnvironmentId", "RunId", "RunNumber", "RunJobId", "JobId", "JobKey", "Ref", "RuleVersion")
		 VALUES ($1, $2, gen_random_uuid(), 1, gen_random_uuid(), gen_random_uuid(), 'deploy', 'main', 1) RETURNING "Id"`, w.org, environment).Scan(&id))
	return id
}

func (w world) release(t *testing.T, target, environment uuid.UUID, id, state, kind string, started time.Time, digest []byte) uuid.UUID {
	t.Helper()
	deployment := w.deployment(t, environment)
	var finished *time.Time
	if state != "running" {
		f := started.Add(time.Minute)
		finished = &f
	}
	var row uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`INSERT INTO "DeploymentReleases" ("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "Kind", "ReleaseId",
		        "ArtifactDigest", "BatchSize", "State", "StartedAt", "FinishedAt")
		 VALUES ($1, $2, gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), $3, 0, $4, $5, $6, 1, $7, $8, $9) RETURNING "Id"`,
		w.org, deployment, target, kind, id, digest, state, started, finished).Scan(&row))
	return row
}

func (w world) releaseHost(t *testing.T, row, host uuid.UUID, release, state string, position int, batch *int) {
	t.Helper()
	w.exec(t, `INSERT INTO "DeploymentReleaseHosts" ("OrganizationId", "ReleaseRowId", "HostId", "ReleaseId", "Position", "State", "Batch")
	           VALUES ($1, $2, $3, $4, $5, $6, $7)`, w.org, row, host, release, position, state, batch)
}

func (w world) seedTarget(t *testing.T, name string) (target, environment uuid.UUID) {
	t.Helper()
	target, _ = w.env.SeedDeployTarget(w.project, "production", name)
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "EnvironmentId" FROM "DeployTargets" WHERE "Id" = $1`, target).Scan(&environment))
	return target, environment
}

func targetOf(t *testing.T, b board, name string) boardTarget {
	t.Helper()
	for _, e := range b.Environments {
		for _, tg := range e.Targets {
			if tg.Name == name {
				return tg
			}
		}
	}
	require.FailNow(t, "target not on the board", name)
	return boardTarget{}
}

func TestAnEnvironmentWithoutTargetsIsStillARow(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "staging")
	res, b := w.get(t, nil)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	require.Len(t, b.Environments, 1)
	assert.Equal(t, "staging", b.Environments[0].Name)
	assert.NotNil(t, b.Environments[0].Targets)
	assert.Empty(t, b.Environments[0].Targets)
	assert.False(t, b.Truncated)
	assert.Equal(t, `"`, res.Header.Get("ETag")[:1], "the answer carries a strong ETag")
}

func TestTheBoardShowsWhatIsDeployedAndHowTheLastReleaseEnded(t *testing.T) {
	w := newWorld(t)
	target, environment := w.seedTarget(t, "web")
	digest := make([]byte, 32)
	digest[0], digest[31] = 0xab, 0xcd
	base := time.Now().UTC().Add(-time.Hour)
	w.release(t, target, environment, "rel-1", "succeeded", "deploy", base, digest)
	w.release(t, target, environment, "rel-2", "rolled_back", "deploy", base.Add(10*time.Minute), nil)

	res, b := w.get(t, nil)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	got := targetOf(t, b, "web")
	assert.Equal(t, "rel-1", got.CurrentReleaseId)
	assert.Len(t, got.CurrentDigest, 64)
	assert.Equal(t, "ab", got.CurrentDigest[:2])
	require.NotNil(t, got.Last)
	assert.Equal(t, "rel-2", got.Last.ReleaseId)
	assert.Equal(t, "rolled_back", got.Last.State)
	assert.True(t, got.AutoRolledBack)
	assert.False(t, got.RollbackFailed)
	assert.False(t, got.Partial)
	assert.Nil(t, got.Running)
}

func TestARollbackThatFailedStaysOnTheBoardAndMarksTheHostsAsMixed(t *testing.T) {
	w := newWorld(t)
	target, environment := w.seedTarget(t, "web")
	base := time.Now().UTC().Add(-time.Hour)
	w.release(t, target, environment, "rel-1", "succeeded", "deploy", base, nil)
	w.release(t, target, environment, "rel-2", "rollback_failed", "deploy", base.Add(10*time.Minute), nil)

	_, b := w.get(t, nil)
	got := targetOf(t, b, "web")
	assert.True(t, got.RollbackFailed)
	assert.True(t, got.Partial, "no check has confirmed the hosts yet")

	w.release(t, target, environment, "rel-3", "succeeded", "deploy", base.Add(30*time.Minute), nil)
	_, b = w.get(t, nil)
	got = targetOf(t, b, "web")
	assert.False(t, got.RollbackFailed, "a release that finishes after it clears the badge")
	assert.Equal(t, "rel-3", got.CurrentReleaseId)
}

func TestARunningDeployReportsItsBatches(t *testing.T) {
	w := newWorld(t)
	target, environment := w.seedTarget(t, "web")
	var first uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Id" FROM "DeployTargetHosts" WHERE "TargetId" = $1`, target).Scan(&first))
	var second uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath")
		 SELECT "OrganizationId", "TargetId", 2, '10.0.0.6', 22, "DeployPath" FROM "DeployTargetHosts" WHERE "Id" = $1 RETURNING "Id"`, first).Scan(&second))

	row := w.release(t, target, environment, "rel-9", "running", "deploy", time.Now().UTC().Add(-time.Minute), nil)
	one := 1
	w.releaseHost(t, row, first, "rel-9", "verified", 1, &one)
	w.releaseHost(t, row, second, "rel-9", "pending", 2, nil)

	_, b := w.get(t, nil)
	got := targetOf(t, b, "web")
	require.NotNil(t, got.Running)
	assert.Equal(t, progress{ReleaseId: "rel-9", Hosts: 2, HostsDone: 1, BatchSize: 1, Batch: 1, Batches: 2}, *got.Running)
	assert.Equal(t, 2, got.Hosts)
}

func TestTheBoardAnswers304ToTheSameETagAndANewOneAfterAChange(t *testing.T) {
	w := newWorld(t)
	target, environment := w.seedTarget(t, "web")
	res, _ := w.get(t, nil)
	require.Equal(t, http.StatusOK, res.Status)
	etag := res.Header.Get("ETag")
	require.NotEmpty(t, etag)

	again, _ := w.get(t, map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, again.Status)
	assert.Empty(t, again.Raw, "a 304 has no body")

	list, _ := w.get(t, map[string]string{"If-None-Match": `"other", ` + etag})
	assert.Equal(t, http.StatusNotModified, list.Status, "a list of validators is read")

	w.release(t, target, environment, "rel-1", "succeeded", "deploy", time.Now().UTC().Add(-time.Hour), nil)
	changed, _ := w.get(t, map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusOK, changed.Status)
	assert.NotEqual(t, etag, changed.Header.Get("ETag"))
}

func TestOneBoardIsTheSameForTheSameData(t *testing.T) {
	w := newWorld(t)
	w.seedTarget(t, "web")
	a, _ := w.get(t, nil)
	b, _ := w.get(t, nil)
	assert.Equal(t, a.Header.Get("ETag"), b.Header.Get("ETag"))
}

func (w world) seedManyTargets(t *testing.T, environments int) {
	t.Helper()
	w.exec(t, `
		WITH envs AS (
		    INSERT INTO "Environments" ("OrganizationId", "ProjectId", "Name", "AllowedRefs", "CreatedByUserId")
		    SELECT p."OrganizationId", p."Id", 'env-' || lpad(g::text, 3, '0'), '[{"Kind":"branch","Pattern":"main"}]', p."CreatedByUserId"
		    FROM "Projects" p, generate_series(1, $2::int) g WHERE p."Id" = $1
		    RETURNING "Id", "Name"
		), targets AS (
		    INSERT INTO "DeployTargets" ("OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", "DeployPath", "CreatedByUserId")
		    SELECT p."OrganizationId", p."Id", e."Id", 'app', 'ssh', 'deploy', '/srv/' || e."Name", p."CreatedByUserId"
		    FROM envs e, "Projects" p WHERE p."Id" = $1
		    RETURNING "Id", "OrganizationId", "DeployPath"
		)
		INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath")
		SELECT t."OrganizationId", t."Id", 1, 'host-' || substr(t."Id"::text, 25) || '.example.test', 22, t."DeployPath" FROM targets t`, w.project, environments)
}

func TestABoardOfTwoHundredEnvironmentsComesInOneRequest(t *testing.T) {
	w := newWorld(t)
	w.seedManyTargets(t, 200)
	start := time.Now()
	res, b := w.get(t, nil)
	took := time.Since(start)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	require.Len(t, b.Environments, 200)
	assert.Equal(t, "app", b.Environments[0].Targets[0].Name)
	assert.Equal(t, 1, b.Environments[199].Targets[0].Hosts)
	assert.Less(t, took, 2*time.Second, "one request builds the whole board")
}
