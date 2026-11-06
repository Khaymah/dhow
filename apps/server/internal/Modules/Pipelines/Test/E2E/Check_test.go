package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const kindChecked = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_CHECKED

type checkJob struct {
	deployWorld
	run   uuid.UUID
	grant *apptest.Grant
}

func (d deployWorld) checkPath() string {
	return apptest.OrgPath(d.org, "/projects/"+d.project.String()+"/environments/production/targets/web/check")
}

func (d deployWorld) askForCheck(t *testing.T, c *apptest.Client) apptest.Response {
	t.Helper()
	return c.Do(apptest.Request{Method: http.MethodPost, Path: d.checkPath()})
}

func (d deployWorld) checking(t *testing.T) checkJob {
	t.Helper()
	res := d.askForCheck(t, d.owner)
	require.Equal(t, http.StatusAccepted, res.Status, string(res.Raw))
	var started struct {
		RunId     uuid.UUID
		RunNumber int64
	}
	res.Decode(t, &started)
	d.env.StartRuns()
	g, err := d.runner.Lease(context.Background(), 1, 0)
	require.NoError(t, err)
	require.NotNil(t, g, "the check run queued a job for the deploy runner")
	require.Equal(t, "check", d.env.RunJobKey(uuid.MustParse(g.Grant.GetJobId())))
	_, err = d.runner.Start(g)
	require.NoError(t, err)
	require.NoError(t, d.runner.BeginStep(g.Ref, 0, true))
	return checkJob{deployWorld: d, run: started.RunId, grant: g}
}

func checked(host uuid.UUID, reason, current, verified string, free int64) *runnerv1.DeployEvent {
	return &runnerv1.DeployEvent{
		Kind: kindChecked, HostId: host.String(), Reason: reason, Message: "a fixed sentence", CurrentReleaseId: current,
		VerifiedReleaseId: verified, FreeBytes: free,
	}
}

func (c checkJob) hosts() []uuid.UUID {
	out := make([]uuid.UUID, 0, 3)
	for _, h := range c.target.Hosts {
		out = append(out, h.ID)
	}
	return out
}

func (c checkJob) report(t *testing.T, events ...*runnerv1.DeployEvent) {
	t.Helper()
	require.NoError(t, c.runner.ReportRelease(c.grant.Ref, 0, events...))
}

func (c checkJob) result(t *testing.T) []struct {
	HostId, Reason, CurrentReleaseId, VerifiedReleaseId string
	Checked, Repaired, Locked                           bool
	FreeBytes                                           *int64
} {
	t.Helper()
	res := c.owner.Get(c.checkPath())
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var data struct {
		Hosts []struct {
			HostId, Reason, CurrentReleaseId, VerifiedReleaseId string
			Checked, Repaired, Locked                           bool
			FreeBytes                                           *int64
		}
	}
	res.Decode(t, &data)
	return data.Hosts
}

func TestACheckRunGoesThroughTheGateWithTheEgressOfTheTargetAndNoArtifact(t *testing.T) {
	d := newDeployWorld(t)
	c := d.checking(t)

	spec := c.grant.Grant.GetSpec()
	require.Len(t, spec.GetSteps(), 1)
	step := spec.GetSteps()[0]
	assert.Equal(t, sshPlugin, step.GetPlugin().GetImage(), "the image comes from the digest pinned in the server")
	assert.Equal(t, map[string]string{"target": "web", "mode": "check"}, step.GetPlugin().GetInputs())
	assert.True(t, step.GetMutating())
	assert.Empty(t, spec.GetInputs(), "a check takes no artifact")
	require.Len(t, spec.GetTargets(), 1)
	assert.Len(t, spec.GetTargets()[0].GetHosts(), 3)
	assert.ElementsMatch(t, []string{hostOne + ":22", hostTwo + ":22", hostThree + ":22"}, spec.GetNetwork().GetExactEndpoints())
	assert.Nil(t, spec.GetCheckout(), "a check run has no repository to check out")
	view := c.view(t, c.run)
	assert.Equal(t, "check", view.Run.Trigger)
	assert.Nil(t, view.Run.Sha)
	assert.Equal(t, 1, c.env.AuditCount("deployment.target_check_requested"))
	var deployments int
	require.NoError(t, c.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Deployments" WHERE "RunId" = $1`, c.run).Scan(&deployments))
	assert.Equal(t, 1, deployments, "the run went through the gate like any deployment")
}

func TestACheckOfThreeServersKeepsTheReasonOfEachAndFailsTheJob(t *testing.T) {
	d := newDeployWorld(t)
	c := d.checking(t)
	h := c.hosts()
	c.report(t,
		checked(h[0], "ok", "10-aaaaaaaa-0", "10-aaaaaaaa-0", 5<<30),
		checked(h[1], "auth_failed", "", "", 0),
		checked(h[2], "disk_low", "10-aaaaaaaa-0", "10-aaaaaaaa-0", 1<<20),
	)
	require.NoError(t, c.runner.EndStep(c.grant.Ref, 0, 19))
	require.NoError(t, c.runner.Complete(c.grant, failed, 19, runnerv1.FailureReason_FAILURE_REASON_TARGET_CHECK_FAILED))

	got := c.result(t)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"ok", "auth_failed", "disk_low"}, []string{got[0].Reason, got[1].Reason, got[2].Reason})
	assert.Equal(t, h[1].String(), got[1].HostId)
	assert.True(t, got[2].FreeBytes != nil && *got[2].FreeBytes == 1<<20)
	assert.Equal(t, "10-aaaaaaaa-0", got[0].CurrentReleaseId)
	view := c.view(t, c.run)
	require.Len(t, view.Jobs, 1)
	assert.Equal(t, "failed", view.Jobs[0].State)
	require.NotNil(t, view.Jobs[0].FailureReason)
	assert.Equal(t, "TARGET_CHECK_FAILED", *view.Jobs[0].FailureReason)
	assert.Equal(t, 3, c.env.AuditCount("deployment.target_checked"))
}

func TestAHostThatWasNeverCheckedIsSaidSo(t *testing.T) {
	d := newDeployWorld(t)
	got := checkJob{deployWorld: d}.result(t)
	require.Len(t, got, 3)
	for _, host := range got {
		assert.False(t, host.Checked)
		assert.Empty(t, host.Reason)
	}
}

func TestACheckStepWritesOnlyChecksAndOnlyForHostsOfItsTarget(t *testing.T) {
	c := newDeployWorld(t).checking(t)
	h := c.hosts()
	refuse := func(code connect.Code, message string, events ...*runnerv1.DeployEvent) {
		t.Helper()
		gotCode, gotMessage := rejected(t, c.runner.ReportRelease(c.grant.Ref, 0, events...))
		assert.Equal(t, code, gotCode)
		assert.True(t, strings.HasPrefix(gotMessage, message), gotMessage)
	}
	refuse(connect.CodeFailedPrecondition, "RELEASE_ILLEGAL_TRANSITION", ev(kindSwitched, h[0]))
	refuse(connect.CodeFailedPrecondition, "RELEASE_ILLEGAL_TRANSITION", &runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: "x-1"})
	refuse(connect.CodePermissionDenied, "RELEASE_UNKNOWN_HOST", checked(uuid.New(), "ok", "", "", 1))
	refuse(connect.CodeInvalidArgument, "VALIDATION_FAILED", checked(h[0], "surprise", "", "", 1))
	refuse(connect.CodeInvalidArgument, "VALIDATION_FAILED", checked(h[0], "ok", "../etc", "", 1))
	refuse(connect.CodeInvalidArgument, "VALIDATION_FAILED", checked(h[0], "ok", "", "", -1))
	refuse(connect.CodeInvalidArgument, "VALIDATION_FAILED", &runnerv1.DeployEvent{Kind: kindChecked, Reason: "ok"})
	var releases int
	require.NoError(t, c.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "DeploymentReleases"`).Scan(&releases))
	assert.Zero(t, releases, "a check never creates a release")
}

func TestADeployStepCannotWriteAChecksReport(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_ILLEGAL_TRANSITION", 1, checked(j.hosts()[0], "ok", j.release, j.release, 1))
}

func TestANewStatusEventKeepsItsKindOutOfTheDeployEvents(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	bad := ev(kindUploaded, j.hosts()[0])
	bad.CurrentReleaseId = "x-1"
	j.refuse(t, connect.CodeInvalidArgument, "VALIDATION_FAILED", 1, bad)
}

func TestOnlyAMaintainerOrMoreStartsACheck(t *testing.T) {
	d := newDeployWorld(t)
	for _, role := range []string{"developer", "viewer"} {
		c, _ := d.env.SignedIn(d.org, role+"@example.com", role)
		assert.Equal(t, http.StatusForbidden, d.askForCheck(t, c).Status, role)
	}
	maintainer, _ := d.env.SignedIn(d.org, "maintainer@example.com", "maintainer")
	assert.Equal(t, http.StatusAccepted, d.askForCheck(t, maintainer).Status)
}

func (c checkJob) unknownRelease(t *testing.T) (string, []uuid.UUID) {
	t.Helper()
	j := c.deployWorld.releasing(t)
	h := j.hosts()
	j.start(t)
	j.report(t, ev(kindUploaded, h[0]), ev(kindSwitched, h[0]), ev(kindUploaded, h[1]))
	j.env.ExpireLease(j.grant.Ref.GetLeaseId())
	j.env.Reconcile()
	j.env.StartRuns()
	j.sweepReleases(t)
	require.Equal(t, "unknown", j.releaseState(t))
	return j.release, h
}

func TestACheckWritesWhatIsReallyDeployedOverAnUnknownRelease(t *testing.T) {
	t.Run("a server that finished the release is verified on it and the others went back", func(t *testing.T) {
		d := newDeployWorld(t)
		release, h := checkJob{deployWorld: d}.unknownRelease(t)
		c := d.checking(t)
		c.report(t,
			checked(h[0], "ok", release, release, 5<<30),
			checked(h[1], "ok", "11-aaaaaaaa-0", "11-aaaaaaaa-0", 5<<30),
			checked(h[2], "ok", "11-aaaaaaaa-0", "11-aaaaaaaa-0", 5<<30),
		)
		states := hostStatesOf(t, d, release)
		assert.Equal(t, "verified", states[h[0]])
		assert.Equal(t, "rolled_back", states[h[1]])
		assert.Equal(t, "pending", states[h[2]], "a server the release never reached keeps its state")
		assert.Equal(t, "unknown", groupStateOf(t, d, release), "servers on different releases stay a mixed group")
		got := c.result(t)
		assert.True(t, got[0].Repaired)
		assert.True(t, got[1].Repaired)
		assert.False(t, got[2].Repaired)
	})
	t.Run("every server back on the old release closes the group as rolled back", func(t *testing.T) {
		d := newDeployWorld(t)
		release, h := checkJob{deployWorld: d}.unknownRelease(t)
		c := d.checking(t)
		c.report(t,
			checked(h[0], "diverged", "11-aaaaaaaa-0", "11-aaaaaaaa-0", 5<<30),
			checked(h[1], "ok", "11-aaaaaaaa-0", "11-aaaaaaaa-0", 5<<30),
		)
		assert.Equal(t, "rolled_back", groupStateOf(t, d, release))
		assert.Equal(t, 1, d.env.AuditCount("deployment.release_repaired"))
	})
	t.Run("a server that is itself in doubt is not repaired", func(t *testing.T) {
		d := newDeployWorld(t)
		release, h := checkJob{deployWorld: d}.unknownRelease(t)
		c := d.checking(t)
		c.report(t,
			checked(h[0], "diverged", release, "11-aaaaaaaa-0", 5<<30),
			checked(h[1], "unreachable", "", "", 0),
		)
		states := hostStatesOf(t, d, release)
		assert.Equal(t, "unknown", states[h[0]], "current and verified differ, so nothing is written")
		assert.Equal(t, "unknown", states[h[1]])
		assert.Equal(t, "unknown", groupStateOf(t, d, release))
	})
}

func hostStatesOf(t *testing.T, d deployWorld, release string) map[uuid.UUID]string {
	t.Helper()
	rows, err := d.env.Pool.Query(context.Background(), `SELECT "HostId", "State" FROM "DeploymentReleaseHosts" WHERE "ReleaseId" = $1`, release)
	require.NoError(t, err)
	defer rows.Close()
	out := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var s string
		require.NoError(t, rows.Scan(&id, &s))
		out[id] = s
	}
	require.NoError(t, rows.Err())
	return out
}

func groupStateOf(t *testing.T, d deployWorld, release string) string {
	t.Helper()
	var s string
	require.NoError(t, d.env.Pool.QueryRow(context.Background(), `SELECT "State" FROM "DeploymentReleases" WHERE "ReleaseId" = $1`, release).Scan(&s))
	return s
}

const twoDeploys = `version: 1
on:
  push: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: make
    artifacts:
      - name: web
        paths: [dist/]
  deploy:
    image: IMAGE
    needs: [build]
    environment: production
    steps:
      - uses: PLUGIN
        with:
          artifact: web
          target: web
  deploy_again:
    image: IMAGE
    needs: [build]
    environment: production
    steps:
      - uses: PLUGIN
        with:
          artifact: web
          target: web
`

func TestASecondJobOnTheSameTargetWaitsInsteadOfFailing(t *testing.T) {
	d := newDeployWorld(t)
	run := d.built(t, twoDeploys)

	first := d.leaseDeploy(t)
	gate, _ := d.gate(t, run, "deploy_again")
	assert.Equal(t, "target_busy", gate)
	view := d.view(t, run)
	states := map[string]string{}
	for _, j := range view.Jobs {
		states[j.Key] = j.State
	}
	assert.Equal(t, "active", states["deploy"])
	assert.Equal(t, "waiting", states["deploy_again"], "the second job waits, it is not failed")
	assert.Nil(t, func() *string {
		for _, j := range view.Jobs {
			if j.Key == "deploy_again" {
				return j.FailureReason
			}
		}
		return nil
	}())

	_, err := d.runner.Start(first)
	require.NoError(t, err)
	require.NoError(t, d.runner.Complete(first, succeeded, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED))

	gate, _ = d.gate(t, run, "deploy_again")
	assert.Empty(t, gate, "the gate opens when the first job is done")
	after := map[string]string{}
	for _, j := range d.view(t, run).Jobs {
		after[j.Key] = j.State
	}
	assert.Equal(t, "active", after["deploy_again"])
}
