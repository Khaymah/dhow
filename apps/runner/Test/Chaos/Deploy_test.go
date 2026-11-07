//go:build chaos

package chaos_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	healthWindowSeconds = 120
	deployTicks         = 2
)

func beaconScript(job string) string {
	url := fmt.Sprintf("http://%s:%s/?job=%s&attempt=$DHOW_ATTEMPT_ID&token=$DHOW_FENCING_TOKEN", sinkHost, shared.sink.port, job)
	return fmt.Sprintf("busybox wget -q -O /dev/null \"%s\" || true\n", url)
}

func deployShapedScript(switched, reported string) string {
	return beaconScript(switched) + fmt.Sprintf("busybox sleep %d\n", healthWindowSeconds) + beaconScript(reported)
}

func TestKillingTheRunnerRightAfterTheSwitchLosesTheDeployAndTheNextDeployRunsAlone(t *testing.T) {
	name := uniqueName("switch-kill")
	switched, reported := name+"-switched", name+"-reported"
	id := newJob(t, name, jobStep{Name: "deploy", Mutating: true, Run: deployShapedScript(switched, reported)})
	waitTicks(t, id, switched, 1, 3*time.Minute)

	shared.stack.killRunner()
	waitNothingRunning(t, 3*time.Second)
	require.NoError(t, shared.stack.startRunner())

	d := shared.api.waitJob(t, id, 4*time.Minute, finished)
	assert.Equal(t, "lost", d.Job.State)
	assert.Equal(t, "LOST_AFTER_MUTATION", d.Job.FailureReason)
	assert.Equal(t, 1, d.Job.AttemptCount, "a deploy that switched is never run again by itself")
	assert.Len(t, shared.sink.attempts(switched), 1, "the switch happened once")
	assert.Empty(t, shared.sink.of(reported), "the dead deploy never reported, so nobody claims it finished")
	waitNoSandboxes(t, 30*time.Second)

	next := uniqueName("after-switch-kill")
	nextID := newJob(t, next, jobStep{Name: "deploy", Mutating: true, Run: tickScript(next, deployTicks)})
	nd := shared.api.waitJob(t, nextID, 4*time.Minute, finished)
	assert.Equal(t, "succeeded", nd.Job.State, "the next deploy starts clean on the same runner")
	assert.Equal(t, 1, nd.Job.AttemptCount)
	assert.Empty(t, shared.sink.overlaps(next))
}
