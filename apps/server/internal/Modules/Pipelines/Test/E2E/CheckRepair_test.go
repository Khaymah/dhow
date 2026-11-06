package e2e_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestANewSuccessfulDeployWritesOverTheOlderUnknownRelease(t *testing.T) {
	d := newDeployWorld(t)
	old, h := checkJob{deployWorld: d}.unknownRelease(t)
	d.defs.Put(shaB, strings.ReplaceAll(pipeline(deployPipeline), "PLUGIN", sshPlugin))
	d.push("refs/heads/main", shaB)
	d.env.StartRuns()
	d.complete(t, "build")
	g := d.leaseDeploy(t)
	_, err := d.runner.Start(g)
	require.NoError(t, err)
	require.NoError(t, d.runner.BeginStep(g.Ref, 1, true))
	fresh := g.Grant.GetSpec().GetTargets()[0].GetReleaseId()
	require.NotEqual(t, old, fresh)

	report := func(events ...*runnerv1.DeployEvent) {
		require.NoError(t, d.runner.ReportRelease(g.Ref, 1, events...))
	}
	report(&runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: fresh, PreviousReleaseId: "11-aaaaaaaa-0"})
	for i, host := range h {
		sw := ev(kindSwitched, host)
		sw.Batch = int32(i + 1)
		report(ev(kindUploaded, host), sw, ev(kindVerified, host))
	}
	report(ev(kindVerified, uuid.Nil))

	assert.Equal(t, "succeeded", groupStateOf(t, d, fresh))
	assert.Equal(t, "rolled_back", groupStateOf(t, d, old), "the old group is closed: nothing of it is on a server any more")
	for host, state := range hostStatesOf(t, d, old) {
		assert.NotEqual(t, "unknown", state, host)
	}
}

func TestAJobFailsWithTheDeployReasonThePluginExitedWith(t *testing.T) {
	d := newDeployWorld(t)
	run := d.built(t, deployPipeline)
	g := d.leaseDeploy(t)
	_, err := d.runner.Start(g)
	require.NoError(t, err)
	require.NoError(t, d.runner.BeginStep(g.Ref, 1, true))
	require.NoError(t, d.runner.EndStep(g.Ref, 1, 10))
	require.NoError(t, d.runner.Complete(g, failed, 10, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_ROLLED_BACK))

	for _, j := range d.view(t, run).Jobs {
		if j.Key == "deploy" {
			assert.Equal(t, "failed", j.State)
			require.NotNil(t, j.FailureReason)
			assert.Equal(t, "DEPLOY_ROLLED_BACK", *j.FailureReason)
		}
	}
}
