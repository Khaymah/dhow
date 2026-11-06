//go:build dhowtest

package app_test

import (
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestTheExitCodeOfTheDeployStepBecomesTheFailureReasonTheManifestDeclares(t *testing.T) {
	cases := []struct {
		code int32
		want runnerv1.FailureReason
	}{
		{1, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_FAILED},
		{10, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_ROLLED_BACK},
		{11, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_HEALTH_FAILED},
		{12, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_ROLLBACK_FAILED},
		{13, runnerv1.FailureReason_FAILURE_REASON_HOST_KEY_MISMATCH},
		{15, runnerv1.FailureReason_FAILURE_REASON_TARGET_LOCKED},
		{18, runnerv1.FailureReason_FAILURE_REASON_TARGET_STATE_CORRUPT},
		{77, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED},
	}
	for _, tc := range cases {
		t.Run(tc.want.String(), func(t *testing.T) {
			srv := runnertest.NewServer(t)
			r := targetRunner(t, srv, "")
			job := newTargetJob(t, srv, "web-1.example.com:22")
			_, out := runTarget(t, r, srv, job, fake.Recording{Steps: []fake.Step{{}, {ExitCode: tc.code}}})
			assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
			assert.Equal(t, tc.want, out.Reason)
			assert.Equal(t, tc.code, out.ExitCode)
			assert.Equal(t, tc.want.String(), srv.Leases()[0].Steps[1], "the step is ended with the same reason")
		})
	}
}

func TestAnotherPluginStepKeepsTheGenericReasonWhateverItsExitCode(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	_, out := runTarget(t, r, srv, job, fake.Recording{Steps: []fake.Step{{}, {}, {ExitCode: 10}}})
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, out.Reason)
}

func TestANonOfficialPluginDoesNotGetDeployReasons(t *testing.T) {
	assert.False(t, jobspec.IsOfficialDeployImage("ghcr.io/khaymah/dhow-deploy-ssh-evil@sha256:"+zeros))
	assert.False(t, jobspec.IsOfficialDeployImage("ghcr.io/acme/deploy@sha256:"+zeros))
	assert.True(t, jobspec.IsOfficialDeployImage(deployImage))
}

const zeros = "0000000000000000000000000000000000000000000000000000000000000000"

func startDeployWithStep(t *testing.T, step fake.Step) (*runnertest.Running, *runnertest.Server) {
	t.Helper()
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	srv.Enqueue(job.spec, map[string][]byte{jobspec.TargetKeySecret(0): []byte(targetKeyText)})
	running := r.Start(fake.Recording{Images: targetImages(), Steps: []fake.Step{{}, step}}, runnertest.StartOptions{Workspace: t.TempDir()})
	require.Eventually(t, func() bool {
		_, ok := stepExecs(running.Driver.Events())[1]
		return ok
	}, finishWithin, 20*time.Millisecond)
	return running, srv
}

func TestADeployStepGetsTheTimeToRollBackAfterACancelInsteadOfTheShortGrace(t *testing.T) {
	slow := fake.Step{HangMillis: 120_000, TermMillis: 3_000, TermExit: 10}
	running, srv := startDeployWithStep(t, slow)
	begun := time.Now()
	srv.RequestCancel(srv.Leases()[0].ID)

	out := running.Next(t, finishWithin)

	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED, out.Result, "the job is canceled even though the plugin rolled back and exited with its own code")
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_CANCELED, out.Reason)
	assert.Equal(t, []syscall.Signal{syscall.SIGTERM}, signals(running.Driver.Events()), "no SIGKILL: the rollback finished inside the grace of the deploy step")
	assert.GreaterOrEqual(t, time.Since(begun), 3*time.Second, "the runner waited longer than the generic grace of one second")
}

func TestAStepThatIsNotADeployStepIsKilledAfterTheGenericGraceEvenIfItAsksForMore(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	srv.Enqueue(job.spec, map[string][]byte{jobspec.TargetKeySecret(0): []byte(targetKeyText)})
	running := r.Start(fake.Recording{Images: targetImages(), Steps: []fake.Step{{HangMillis: 120_000, IgnoreTerm: true}}}, runnertest.StartOptions{Workspace: t.TempDir()})
	runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	srv.RequestCancel(srv.Leases()[0].ID)

	out := running.Next(t, finishWithin)

	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED, out.Result)
	assert.Equal(t, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}, signals(running.Driver.Events()))
}

func TestTheGraceOfADeployStepCoversTheRollbackBudgetThePluginUsesAfterACancel(t *testing.T) {
	grace := jobspec.DeployCancelGrace()
	assert.Greater(t, grace, jobspec.DeployRollbackBudget+jobspec.DeployConnGrace, "the plugin keeps its connections open for the budget plus the connection grace")
	assert.GreaterOrEqual(t, grace, jobspec.DeployRollbackBudget+jobspec.DeployConnGrace+jobspec.DeployCleanupBudget+jobspec.DeployUnlockBudget)
}
