//go:build dhowtest

package app_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

func TestTheLeaseIsKeptAliveWhileADeployStepRollsBackAfterACancel(t *testing.T) {
	srv := runnertest.NewServer(t)
	srv.Configure(func(k *runnertest.Knobs) { k.LeaseTTL = 20 * time.Second })
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	srv.Enqueue(job.spec, map[string][]byte{jobspec.TargetKeySecret(0): []byte(targetKeyText)})
	running := r.Start(fake.Recording{Images: targetImages(), Steps: []fake.Step{{}, {HangMillis: 120_000, TermMillis: 23_000, TermExit: 10}}}, runnertest.StartOptions{Workspace: t.TempDir()})
	require.Eventually(t, func() bool {
		_, ok := stepExecs(running.Driver.Events())[1]
		return ok
	}, finishWithin, 20*time.Millisecond)
	srv.RequestCancel(srv.Leases()[0].ID)
	runnertest.WaitEvent(t, running.Driver, fake.EventSignal, finishWithin)
	during := srv.Calls(runnerv1connect.LeaseServiceHeartbeatProcedure)

	out := running.Next(t, 2*finishWithin)

	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED, out.Result, "the lease did not expire while the plugin rolled back")
	assert.False(t, out.SelfFenced)
	assert.Greater(t, srv.Calls(runnerv1connect.LeaseServiceHeartbeatProcedure), during, "heartbeats went on during the long grace")
}
