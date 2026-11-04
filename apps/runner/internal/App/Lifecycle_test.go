//go:build dhowtest

package app_test

import (
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	executor "github.com/Khaymah/dhow/apps/runner/internal/Executor"
	logs "github.com/Khaymah/dhow/apps/runner/internal/Logs"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

func signals(events []fake.Event) []syscall.Signal {
	var out []syscall.Signal
	for _, e := range events {
		if e.Kind == fake.EventSignal {
			out = append(out, e.Signal)
		}
	}
	return out
}

func TestCancellationTerminatesTheStepGracefully(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{Output: []fake.Output{{Data: "working\n"}}, HangMillis: 120_000}}}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	srv.RequestCancel(srv.Leases()[0].ID)

	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED, out.Result)
	assert.Equal(t, []syscall.Signal{syscall.SIGTERM}, signals(running.Driver.Events()))
	l := srv.Leases()[0]
	assert.Equal(t, "FAILURE_REASON_CANCELED", l.Steps[0])
	require.Len(t, l.Completes, 1)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_CANCELED, l.Completes[0].GetFailureReason())
}

func TestAStepThatIgnoresSIGTERMIsKilledAfterTheGrace(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000, IgnoreTerm: true}}}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	srv.RequestCancel(srv.Leases()[0].ID)
	term := runnertest.WaitEvent(t, running.Driver, fake.EventSignal, finishWithin)
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED, out.Result)
	sigs := signals(running.Driver.Events())
	require.Equal(t, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}, sigs)
	var kill fake.Event
	for _, e := range running.Driver.Events() {
		if e.Kind == fake.EventSignal && e.Signal == syscall.SIGKILL {
			kill = e
		}
	}
	assert.InDelta(t, 1.0, kill.At.Sub(term.At).Seconds(), 0.3, "SIGKILL follows after the one second grace")
}

func TestTheJobTimeoutEndsTheAttemptTimedOut(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec()
	spec.TimeoutSeconds = 1
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000}}}, runnertest.StartOptions{})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_TIMED_OUT, out.Result)
	assert.Equal(t, "FAILURE_REASON_TIMED_OUT", srv.Leases()[0].Steps[0])
}

func TestAStepTimeoutFailsTheStep(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(&runnerv1.Step{Name: "slow", Run: "sleep", TimeoutSeconds: 1}), nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000}}}, runnertest.StartOptions{})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, out.Reason)
	l := srv.Leases()[0]
	assert.Equal(t, "FAILURE_REASON_TIMED_OUT", l.Steps[0])
	assert.Contains(t, logText(l), "ran past its timeout")
}

func TestALeaseTheServerDeclaresLostKillsTheJobWithoutAReport(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Configure(func(k *runnertest.Knobs) { k.LeaseTTL = 20 * time.Second })
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000}}}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	srv.ExpireLease(srv.Leases()[0].ID)
	runnertest.WaitEvent(t, running.Driver, fake.EventKill, finishWithin)
	out := running.Next(t, finishWithin)
	assert.False(t, out.SelfFenced)
	assert.Empty(t, srv.Leases()[0].Completes)
	assert.Zero(t, srv.Calls(runnerv1connect.LeaseServiceCompleteProcedure))
}

func TestLogShippingProblemsNeverFailTheJob(t *testing.T) {
	cases := map[string]func(*runnertest.Knobs){
		"truncated":      func(k *runnertest.Knobs) { k.LogCapBytes = 20 },
		"unimplemented":  func(k *runnertest.Knobs) { k.AppendUnimplemented = true },
		"flaky storage":  func(k *runnertest.Knobs) { k.AppendUnavailable = 3 },
		"flaky complete": func(k *runnertest.Knobs) { k.CompleteUnavailable = 2 },
	}
	for name, knob := range cases {
		t.Run(name, func(t *testing.T) {
			srv := runnertest.NewServer(t)
			r := runnertest.Register(t, srv, srv.URL)
			srv.Configure(knob)
			srv.Enqueue(runnertest.Spec(), nil)
			var output []fake.Output
			for range 50 {
				output = append(output, fake.Output{Data: "a line of output that keeps going\n"})
			}
			running := r.Start(fake.Recording{Steps: []fake.Step{{Output: output}}}, runnertest.StartOptions{})
			out := running.Next(t, finishWithin)
			assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result)
			assert.True(t, out.Reported)
			l := srv.Leases()[0]
			if name == "flaky storage" || name == "flaky complete" {
				assert.Equal(t, 50, countLines(logText(l), "a line of output"))
			}
		})
	}
}

func countLines(text, needle string) int {
	n := 0
	for i := 0; i+len(needle) <= len(text); i++ {
		if text[i:i+len(needle)] == needle {
			n++
		}
	}
	return n
}

func TestAFullLogQueueNeverHangsTheAttempt(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Configure(func(k *runnertest.Knobs) { k.AppendUnavailable = 1 << 30 })
	spec := runnertest.Spec()
	spec.TimeoutSeconds = 2
	srv.Enqueue(spec, nil)
	var output []fake.Output
	for range 64 {
		output = append(output, fake.Output{Data: strings.Repeat("x", 4095) + "\n"})
	}
	running := r.Start(fake.Recording{Steps: []fake.Step{{Output: output, HangMillis: 120_000}}}, runnertest.StartOptions{
		Logs: func(o *logs.Options) { o.MemoryBytes, o.SpillBytes, o.BatchChunks = 8<<10, 32<<10, 1 },
	})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_TIMED_OUT, out.Result, "the output passed the queue and the spill limits")
	assert.True(t, out.Reported)
	require.Len(t, srv.Leases()[0].Completes, 1)
}

func TestAGrantThatArrivesLateInTheLongPollIsHeartbeatAtOnce(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Configure(func(k *runnertest.Knobs) { k.LeaseTTL, k.GrantDelay = 20*time.Second, 8*time.Second })
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{PrepareDelayMillis: 3000, Steps: []fake.Step{{HangMillis: 4000}}}, runnertest.StartOptions{})
	out := running.Next(t, 40*time.Second)
	assert.False(t, out.SelfFenced, "the deadline counts from the poll, so the first heartbeat cannot wait a full interval")
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result)
}

func TestAStartThatOutlivesTheDeadlineNeverStartsTheSandbox(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Configure(func(k *runnertest.Knobs) {
		k.LeaseTTL, k.StartDelay, k.HeartbeatFail = 20*time.Second, 12*time.Second, true
	})
	spec := runnertest.Spec(&runnerv1.Step{Name: "print", Run: "env"})
	spec.Secrets = []*runnerv1.SecretRef{{Name: "API_TOKEN", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "API_TOKEN"}}}
	srv.Enqueue(spec, map[string][]byte{"API_TOKEN": []byte("a-secret-value")})
	running := r.Start(fake.Recording{}, runnertest.StartOptions{})
	out := running.Next(t, 40*time.Second)
	assert.True(t, out.SelfFenced)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_LOST, out.Result)
	assert.NotContains(t, kinds(running.Driver.Events()), fake.EventStart, "nothing runs once the runner fenced itself")
	assert.NotContains(t, kinds(running.Driver.Events()), fake.EventExec)
}

func TestASuccessIsNeverReportedAfterTheRunnerFencedItself(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Configure(func(k *runnertest.Knobs) { k.LeaseTTL = 20 * time.Second })
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 500}}}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	srv.Configure(func(k *runnertest.Knobs) { k.HeartbeatFail, k.EndStepFail = true, true })
	out := running.Next(t, 40*time.Second)
	assert.True(t, out.SelfFenced)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_LOST, out.Result, "the last step finished, but the lease was given up meanwhile")
	for _, c := range srv.Leases()[0].Completes {
		assert.NotEqual(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, c.GetResult())
	}
}

func TestASecretTooLargeToMaskStopsTheJob(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec()
	spec.Secrets = []*runnerv1.SecretRef{{Name: "BLOB", Target: &runnerv1.SecretRef_AsFile{AsFile: "blob"}}}
	srv.Enqueue(spec, map[string][]byte{"BLOB": []byte(strings.Repeat("k", 64<<10+1))})
	running := r.Start(fake.Recording{}, runnertest.StartOptions{})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_SPEC_REJECTED, out.Reason)
	assert.NotContains(t, kinds(running.Driver.Events()), fake.EventExec)
	assert.Contains(t, logText(srv.Leases()[0]), "the secret BLOB is too large to mask")
}

func TestSecretFilesAreZeroedAfterTheStep(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec()
	spec.Secrets = []*runnerv1.SecretRef{{Name: "KEY", Target: &runnerv1.SecretRef_AsFile{AsFile: "key"}}}
	srv.Enqueue(spec, map[string][]byte{"KEY": []byte("file-secret-value")})
	running := r.Start(fake.Recording{}, runnertest.StartOptions{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	exec := runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	require.Equal(t, []string{"key"}, exec.FileNames)
	require.Len(t, exec.FileData, 1)
	assert.Equal(t, make([]byte, len("file-secret-value")), exec.FileData[0], "the bytes the step saw are wiped, not just dropped")
}

func TestTheRunnerEnvironmentNeverReachesAJob(t *testing.T) {
	t.Setenv("DHOW_RUNNER_LEAK_CANARY", "runner-only")
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec(&runnerv1.Step{Name: "a", Run: "env", Env: map[string]string{"STEP_VALUE": "1"}})
	spec.Env = map[string]string{"JOB_VALUE": "1"}
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{}, runnertest.StartOptions{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	exec := runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	assert.ElementsMatch(t, []string{"JOB_VALUE", "STEP_VALUE", "DHOW_FENCING_TOKEN", "DHOW_JOB_ID", "DHOW_ATTEMPT_ID", "DHOW_STEP_INDEX", "DHOW_OUTPUT", "DHOW_SECRETS_DIR"},
		exec.EnvNames, "a step sees the job's env and the DHOW_ attempt values, never the runner's own environment")
	for name := range exec.Env {
		assert.False(t, strings.HasPrefix(name, "DHOW_RUNNER_"), name)
	}
}

func TestAJobWithEgressGetsTheProxyVariablesItCannotOverride(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec(&runnerv1.Step{Name: "a", Run: "env", Env: map[string]string{"HTTPS_PROXY": "http://attacker.example.com:8080"}})
	spec.Network = &runnerv1.Network{Egress: []string{"registry.npmjs.org"}}
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{}, runnertest.StartOptions{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	exec := runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		assert.Equal(t, executor.ProxyURL, exec.Env[name], name)
	}
	assert.Equal(t, executor.NoProxy, exec.Env["NO_PROXY"])
}

func TestProcessesThatSurviveTheKillFailTheAttempt(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{
		Steps:         []fake.Step{{HangMillis: 120_000, IgnoreTerm: true, IgnoreKill: true}},
		KillSurvivors: true,
	}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	srv.RequestCancel(srv.Leases()[0].ID)
	runnertest.WaitEvent(t, running.Driver, fake.EventKill, finishWithin)
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, out.Reason, "a canceled job whose processes outlive the kill is a sandbox failure")
	assert.Equal(t, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}, signals(running.Driver.Events()), "the kill comes after SIGTERM and SIGKILL")
}

func TestProcessesLeftAfterTheJobEndedFailTheAttempt(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{DestroySurvivors: true}, runnertest.StartOptions{})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, out.Reason, "a job that left processes behind its last step is a sandbox failure")
	assert.True(t, out.Survivors)
	assert.ErrorIs(t, running.Ended(t, finishWithin), sandbox.ErrSurvivors, "the runner takes no more work and stops")
	srv.Enqueue(runnertest.Spec(), nil)
	assert.Len(t, srv.Leases(), 1, "the stopped runner leased nothing else")
}
