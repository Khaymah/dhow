package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (d deployWorld) askRun(t *testing.T, c *apptest.Client) (int, uuid.UUID) {
	t.Helper()
	res := d.askForCheck(t, c)
	var started struct{ RunId uuid.UUID }
	if res.Status == http.StatusAccepted {
		res.Decode(t, &started)
	}
	return res.Status, started.RunId
}

func (d deployWorld) checkRuns(t *testing.T) int {
	t.Helper()
	return d.env.CountRows(`SELECT count(*) FROM "PipelineRuns" WHERE "Trigger" = 'check'`)
}

func TestAskingForACheckWhileOneIsPendingGivesTheSameRun(t *testing.T) {
	d := newDeployWorld(t)
	status, first := d.askRun(t, d.owner)
	require.Equal(t, http.StatusAccepted, status)
	for range 15 {
		again, run := d.askRun(t, d.owner)
		assert.Equal(t, http.StatusAccepted, again, "asking again is not limited, it answers with the run that is waiting")
		assert.Equal(t, first, run)
	}
	assert.Equal(t, 1, d.checkRuns(t))

	require.Equal(t, http.StatusOK, d.post(t, d.owner, first, "cancel").Status)
	_, next := d.askRun(t, d.owner)
	assert.NotEqual(t, first, next, "once the run ended, a new check is a new run")
	assert.Equal(t, 2, d.checkRuns(t))
}

func TestChecksHaveTheirOwnSmallLimit(t *testing.T) {
	d := newDeployWorld(t)
	require.Less(t, config.PipelineCheckPerUser, config.PipelineTriggerPerUser)
	for i := range config.PipelineCheckPerUser {
		status, run := d.askRun(t, d.owner)
		require.Equal(t, http.StatusAccepted, status, "check %d", i+1)
		require.Equal(t, http.StatusOK, d.post(t, d.owner, run, "cancel").Status)
	}
	status, _ := d.askRun(t, d.owner)
	assert.Equal(t, http.StatusTooManyRequests, status)
	assert.Equal(t, config.PipelineCheckPerUser, d.checkRuns(t))

	manual := d.owner.Do(apptest.Request{Method: http.MethodPost, Path: apptest.OrgPath(d.org, "/projects/"+d.project.String()+"/runs"), Body: map[string]any{"Ref": "main"}})
	assert.NotEqual(t, http.StatusTooManyRequests, manual.Status, "the limit of checks does not use the quota of normal runs")
}

func TestARefusedCheckUsesNoQuota(t *testing.T) {
	d := newDeployWorld(t)
	d.sql(t, `UPDATE "Projects" SET "State" = 'archived', "ArchivedAt" = now() WHERE "Id" = $1`, d.project)
	for range config.PipelineCheckPerUser + 3 {
		status, _ := d.askRun(t, d.owner)
		assert.NotEqual(t, http.StatusTooManyRequests, status, "an archived project is refused before the quota is charged")
		assert.NotEqual(t, http.StatusAccepted, status)
	}
	d.sql(t, `UPDATE "Projects" SET "State" = 'active', "ArchivedAt" = NULL WHERE "Id" = $1`, d.project)
	status, _ := d.askRun(t, d.owner)
	assert.Equal(t, http.StatusAccepted, status, "the refused requests left the quota untouched")
}

func TestACheckRunCannotBeRunAgainByAnyone(t *testing.T) {
	d := newDeployWorld(t)
	c := d.checking(t)
	require.NoError(t, d.runner.EndStep(c.grant.Ref, 0, 19))
	require.NoError(t, d.runner.Complete(c.grant, failed, 19, runnerv1.FailureReason_FAILURE_REASON_TARGET_CHECK_FAILED))

	developer, _ := d.env.SignedIn(d.org, "developer@example.com", "developer")
	for _, who := range []*apptest.Client{d.owner, developer} {
		res := d.post(t, who, c.run, "rerun-failed")
		assert.Equal(t, http.StatusConflict, res.Status, string(res.Raw))
		assert.Contains(t, string(res.Raw), "CHECK_RUN_NOT_RERUNNABLE")
	}
}

func TestACheckIsNotInTheDeploymentsOfTheEnvironmentOrItsLastRef(t *testing.T) {
	d := newDeployWorld(t)
	c := d.checking(t)
	list := d.owner.Get(apptest.OrgPath(d.org, "/projects/"+d.project.String()+"/environments/production/deployments"))
	require.Equal(t, http.StatusOK, list.Status, string(list.Raw))
	assert.NotContains(t, string(list.Raw), c.run.String(), "a check is not a deployment in the history of the environment")
	var inTable int
	require.NoError(t, d.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Deployments" WHERE "RunId" = $1`, c.run).Scan(&inTable))
	assert.Equal(t, 1, inTable, "it still went through the gate")
}

func TestOnlyAJobWithATargetMayEndWithADeployReason(t *testing.T) {
	d := newDeployWorld(t)
	d.start(deployPipeline)
	r, g := d.leaseKey(t, "build")
	_, err := r.Start(g)
	require.NoError(t, err)
	require.NoError(t, r.BeginStep(g.Ref, 0, false))
	for _, reason := range []runnerv1.FailureReason{
		runnerv1.FailureReason_FAILURE_REASON_DEPLOY_ROLLED_BACK, runnerv1.FailureReason_FAILURE_REASON_TARGET_LOCKED,
		runnerv1.FailureReason_FAILURE_REASON_CURRENT_NOT_A_SYMLINK, runnerv1.FailureReason_FAILURE_REASON_TARGET_CHECK_FAILED,
	} {
		code, message := rejected(t, r.EndStepWith(g.Ref, 0, 10, reason))
		assert.Equal(t, 3, int(code), reason.String())
		assert.True(t, strings.HasPrefix(message, "VALIDATION_FAILED"), message)
		code, _ = rejected(t, r.Complete(g, failed, 10, reason))
		assert.Equal(t, 3, int(code), reason.String())
	}
	require.NoError(t, r.Complete(g, failed, 1, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED), "the ordinary reasons still work")
}

func TestABakedImageThatIsNotPinnedMakesTheCheckUnavailable(t *testing.T) {
	for _, image := range []string{"ghcr.io/khaymah/dhow-deploy-ssh:latest", "ghcr.io/acme/deploy@sha256:" + strings.Repeat("a", 64)} {
		d := newDeployWorldWith(t, image)
		res := d.askForCheck(t, d.owner)
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, image)
		assert.Contains(t, string(res.Raw), "TARGET_CHECK_UNAVAILABLE")
	}
}

func TestACheckThatSaysDivergedOrIsNotIdleWritesNothing(t *testing.T) {
	d := newDeployWorld(t)
	release, h := checkJob{deployWorld: d}.unknownRelease(t)
	c := d.checking(t)
	c.report(t, checked(h[0], "diverged", release, release, 5<<30), checked(h[1], "locked", release, release, 5<<30))
	states := hostStatesOf(t, d, release)
	assert.Equal(t, "unknown", states[h[0]], "a state that is not idle is not enough to write over an unknown release")
	assert.Equal(t, "unknown", states[h[1]])
	assert.Equal(t, "unknown", groupStateOf(t, d, release))
}
