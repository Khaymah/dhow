//go:build dhowtest

package app_test

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	executor "github.com/Khaymah/dhow/apps/runner/internal/Executor"
	logs "github.com/Khaymah/dhow/apps/runner/internal/Logs"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

func TestAGrantSignedWithAnotherKeyIsRefusedAndNeverRuns(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Configure(func(k *runnertest.Knobs) { k.WrongKey = true })
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{}, runnertest.StartOptions{})

	out := running.Next(t, finishWithin)
	assert.True(t, out.Refused)
	assert.Equal(t, []fake.EventKind{fake.EventCleanup}, kinds(running.Driver.Events()), "no sandbox is prepared or started")
	for _, p := range []string{
		runnerv1connect.LeaseServiceStartProcedure, runnerv1connect.LeaseServiceHeartbeatProcedure,
		runnerv1connect.LeaseServiceBeginStepProcedure, runnerv1connect.LeaseServiceCompleteProcedure, runnerv1connect.LogServiceAppendProcedure,
	} {
		assert.Zero(t, srv.Calls(p), p)
	}
	journal, err := os.ReadFile(filepath.Join(r.StateDir, "journal"))
	if err == nil {
		assert.NotContains(t, string(journal), srv.Leases()[0].ID)
	}
	l := srv.Leases()[0]
	assert.False(t, l.Started)
	assert.Empty(t, l.Completes)
}

func TestTheRunnerKillsTheSandboxTenSecondsBeforeTheLeaseExpires(t *testing.T) {
	srv := runnertest.NewServer(t)
	srv.Configure(func(k *runnertest.Knobs) { k.LeaseTTL = 20 * time.Second })
	proxy := runnertest.NewProxy(t, srv.HTTP.Listener.Addr().String())
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000}}}, runnertest.StartOptions{URL: proxy.URL()})

	runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	beats := srv.Calls(runnerv1connect.LeaseServiceHeartbeatProcedure)
	runnertest.Eventually(t, 10*time.Second, func() bool {
		return srv.Calls(runnerv1connect.LeaseServiceHeartbeatProcedure) > beats
	}, "a heartbeat")
	time.Sleep(time.Second)
	proxy.Blackhole(true)
	cut := time.Now()
	kill := runnertest.WaitEvent(t, running.Driver, fake.EventKill, 30*time.Second)
	l := srv.Leases()[0]
	early := l.ExpiresAt.Sub(kill.At)
	t.Logf("cut at %s, killed %s later, %s before the lease expired", cut.Format(time.StampMilli), kill.At.Sub(cut), early)
	assert.InDelta(t, (10 * time.Second).Seconds(), early.Seconds(), 1.0, "the kill lands 10s before ExpiresAt")

	out := running.Next(t, 30*time.Second)
	assert.True(t, out.SelfFenced)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_LOST, out.Result)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST, out.Reason)
	assert.False(t, out.Reported, "the server is unreachable, so the result waits in the journal")
	assert.Empty(t, l.Completes)
	journal, err := os.ReadFile(filepath.Join(r.StateDir, "journal"))
	require.NoError(t, err)
	assert.Contains(t, string(journal), l.ID)
}

const secretValue = `sk-test/9f8e+7d6c"5b4a`

func secretForms() map[string]string {
	raw := []byte(secretValue)
	jsonForm, _ := json.Marshal(secretValue)
	return map[string]string{
		"raw": secretValue, "base64": base64.StdEncoding.EncodeToString(raw), "base64url": base64.RawURLEncoding.EncodeToString(raw),
		"hex": hex.EncodeToString(raw), "url": url.QueryEscape(secretValue), "json": strings.Trim(string(jsonForm), `"`),
	}
}

func scanTree(t *testing.T, dir string, forms map[string]string) int {
	t.Helper()
	files := 0
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		files++
		for name, form := range forms {
			assert.NotContains(t, string(raw), form, "%s form in %s", name, path)
		}
		return nil
	}))
	return files
}

func TestSecretsInEveryEncodingNeverLeaveTheRunner(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec(&runnerv1.Step{Name: "print", Run: "env"})
	spec.Secrets = []*runnerv1.SecretRef{{Name: "API_TOKEN", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "API_TOKEN"}}}
	srv.Enqueue(spec, map[string][]byte{"API_TOKEN": []byte(secretValue)})
	srv.Configure(func(k *runnertest.Knobs) { k.AppendUnavailable = 1 << 20 })
	outputs := []fake.Output{{Data: "start\n", DelayMillis: 600}}
	for _, enc := range []string{"raw", "base64", "base64url", "hex", "url", "json"} {
		outputs = append(outputs, fake.Output{Data: enc + "=", FromEnv: "API_TOKEN", Encoding: enc}, fake.Output{Data: "\n"})
	}
	running := r.Start(fake.Recording{Steps: []fake.Step{{Output: outputs}}}, runnertest.StartOptions{
		Logs: func(o *logs.Options) { o.MemoryBytes, o.BatchChunks = 1, 1 },
	})

	exec := runnertest.WaitEvent(t, running.Driver, fake.EventExec, finishWithin)
	assert.Contains(t, exec.EnvNames, "API_TOKEN", "the secret reaches the step environment")
	var spill string
	runnertest.Eventually(t, finishWithin, func() bool {
		matches, _ := filepath.Glob(filepath.Join(r.StateDir, "attempts", "*", "logs.spill"))
		if len(matches) == 0 {
			return false
		}
		spill = matches[0]
		raw, err := os.ReadFile(spill)
		return err == nil && strings.Contains(string(raw), "json=")
	}, "a spill file with the step output")
	info, err := os.Stat(spill)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	raw, err := os.ReadFile(spill)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "base64=***", "the spill file holds masked output")
	assert.Positive(t, scanTree(t, r.StateDir, secretForms()), "the spill file, the journal and the identity are scanned")

	srv.Configure(func(k *runnertest.Knobs) { k.AppendUnavailable = 0 })
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result)
	text := logText(srv.Leases()[0])
	for _, enc := range []string{"raw", "base64", "base64url", "hex", "url"} {
		assert.Contains(t, text, enc+"=***\n", enc)
	}
	assert.Contains(t, text, `json="***"`+"\n")
	for name, form := range secretForms() {
		assert.NotContains(t, text, form, name)
	}
	scanTree(t, r.StateDir, secretForms())
	entries, err := os.ReadDir(filepath.Join(r.StateDir, "attempts"))
	require.NoError(t, err)
	assert.Empty(t, entries, "the spill file goes away with the attempt")
}

func TestARestartMidJobReportsTheOldLeaseAsLost(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	first := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000}}}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, first.Driver, fake.EventExec, finishWithin)
	srv.Configure(func(k *runnertest.Knobs) { k.CompleteUnavailable = 1 << 20 })
	require.NoError(t, first.Stop())
	l := srv.Leases()[0]
	require.Empty(t, l.Completes, "the server could not hear the shutdown report")
	journal, err := os.ReadFile(filepath.Join(r.StateDir, "journal"))
	require.NoError(t, err)
	require.Contains(t, string(journal), l.ID)

	srv.Configure(func(k *runnertest.Knobs) { k.CompleteUnavailable = 0 })
	second := r.Start(fake.Recording{}, runnertest.StartOptions{})
	runnertest.Eventually(t, finishWithin, func() bool {
		got, _ := srv.Lease(l.ID)
		return len(got.Completes) > 0
	}, "the restart report")
	got, _ := srv.Lease(l.ID)
	c := got.Completes[0]
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_LOST, c.GetResult())
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED, c.GetFailureReason())
	assert.Equal(t, l.Fencing, c.GetLease().GetFencingToken(), "the old fencing token")
	assert.Contains(t, kinds(second.Driver.Events()), fake.EventCleanup)
	runnertest.Eventually(t, finishWithin, func() bool {
		raw, _ := os.ReadFile(filepath.Join(r.StateDir, "journal"))
		return !strings.Contains(string(raw), l.ID)
	}, "the journal to forget the lease")
}

func TestACrashLeavesAJournalEntryThatTheNextStartReports(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	first := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000}}}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, first.Driver, fake.EventExec, finishWithin)
	journal, err := os.ReadFile(filepath.Join(r.StateDir, "journal"))
	require.NoError(t, err)
	snapshot := append([]byte(nil), journal...)
	srv.Configure(func(k *runnertest.Knobs) { k.CompleteUnavailable = 1 << 20 })
	require.NoError(t, first.Stop())
	require.NoError(t, os.WriteFile(filepath.Join(r.StateDir, "journal"), snapshot, 0o600))
	srv.Configure(func(k *runnertest.Knobs) { k.CompleteUnavailable = 0 })

	r.Start(fake.Recording{}, runnertest.StartOptions{})
	l := srv.Leases()[0]
	runnertest.Eventually(t, finishWithin, func() bool {
		got, _ := srv.Lease(l.ID)
		return len(got.Completes) > 0
	}, "the restart report")
	got, _ := srv.Lease(l.ID)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED, got.Completes[0].GetFailureReason())
	assert.Equal(t, l.Fencing, got.Completes[0].GetLease().GetFencingToken())
}

var _ executor.Outcome

func TestAReplayedGrantIsRefusedBeforeAnyCall(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	srv.Enqueue(runnertest.Spec(), map[string][]byte{})
	running := r.Start(fake.Recording{}, runnertest.StartOptions{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	leases := srv.Leases()
	require.Len(t, leases, 2)
	before := map[string]int{}
	procedures := []string{
		runnerv1connect.LeaseServiceStartProcedure, runnerv1connect.LeaseServiceHeartbeatProcedure,
		runnerv1connect.LeaseServiceCompleteProcedure, runnerv1connect.LogServiceAppendProcedure,
	}
	for _, p := range procedures {
		before[p] = srv.Calls(p)
	}
	prepared := len(running.Driver.Events())

	srv.Replay(leases[1].ID)
	assert.True(t, running.Next(t, finishWithin).Refused, "the same grant a second time")
	srv.Replay(leases[0].ID)
	assert.True(t, running.Next(t, finishWithin).Refused, "an older grant after a newer one")

	for _, p := range procedures {
		assert.Equal(t, before[p], srv.Calls(p), p)
	}
	assert.Len(t, running.Driver.Events(), prepared, "no sandbox for a replay")
	raw, err := os.ReadFile(filepath.Join(r.StateDir, "journal"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), fmt.Sprintf(`"HighWater":%d`, leases[1].Fencing))
}

func TestAnUnreportableOldAttemptDoesNotStopTheRunnerFromWorking(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Enqueue(runnertest.Spec(), nil)
	first := r.Start(fake.Recording{Steps: []fake.Step{{HangMillis: 120_000}}}, runnertest.StartOptions{})
	runnertest.WaitEvent(t, first.Driver, fake.EventExec, finishWithin)
	old := srv.Leases()[0]
	srv.Configure(func(k *runnertest.Knobs) { k.CompleteFailLease = old.ID })
	require.NoError(t, first.Stop())

	srv.Enqueue(runnertest.Spec(), nil)
	second := r.Start(fake.Recording{}, runnertest.StartOptions{})
	out := second.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, "a new job runs although the old attempt cannot be reported")
	srv.Configure(func(k *runnertest.Knobs) { k.CompleteFailLease = "" })
	runnertest.Eventually(t, finishWithin, func() bool {
		got, _ := srv.Lease(old.ID)
		return len(got.Completes) > 0
	}, "the old attempt to be reported in the background")
	require.NoError(t, second.Stop())
	before := stateListing(t, r.StateDir)
	time.Sleep(400 * time.Millisecond)
	assert.Equal(t, before, stateListing(t, r.StateDir), "a stopped runner has no background writer left in its state directory")
}

func stateListing(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
