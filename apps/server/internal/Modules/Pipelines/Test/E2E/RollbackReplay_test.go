package e2e_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const rollbackScenarioPrefix = "rollback-"

func (r rollbackJob) replayEvent(t *testing.T, e deployrelease.Event, host uuid.UUID) *runnerv1.DeployEvent {
	t.Helper()
	kind, ok := replayKinds[e.Event]
	require.True(t, ok, "the plugin wrote the event %q that the report protocol does not have", e.Event)
	out := &runnerv1.DeployEvent{Kind: kind, Batch: int32(e.Batch), Mode: e.Mode, Reason: e.Reason, Message: e.Message}
	if e.HostID != "" {
		out.HostId = host.String()
	}
	if e.ReleaseID != "" {
		out.ReleaseId = r.release()
	}
	if e.PreviousReleaseID != "" {
		out.PreviousReleaseId = r.current
	}
	if h := e.Health; h != nil {
		out.Health = &runnerv1.DeployHealth{
			Attempts: int32(h.Attempts), Passed: h.Passed, LastStatus: int32(h.LastStatus), LastError: h.LastError,
			DurationMs: h.DurationMs, StableChecks: int32(h.StableChecks),
		}
	}
	return out
}

func TestTheReportsOfTheRealPluginRunOfAManualRollbackAreAcceptedByTheServerAndEndInTheSameState(t *testing.T) {
	dir := os.Getenv(replayEnv)
	if dir == "" {
		t.Skipf("set %s to the directory the deploy end to end run keeps the plugin reports in (make deploy-e2e does)", replayEnv)
	}
	files, err := filepath.Glob(filepath.Join(dir, rollbackScenarioPrefix+"*.jsonl"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "the deploy end to end run left no report of a manual rollback")
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			rawExit, err := os.ReadFile(strings.TrimSuffix(file, ".jsonl") + ".exit")
			require.NoError(t, err)
			exit, err := strconv.Atoi(strings.TrimSpace(string(rawExit)))
			require.NoError(t, err)

			r := newDeployWorld(t).shipped(t).rollingBack(t, earlier)
			hosts := r.hosts()
			last := map[uuid.UUID]string{}
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				e := decodeStrict(t, line)
				var host uuid.UUID
				if e.HostID != "" {
					host = hosts[int(e.HostID[len(e.HostID)-1]-'1')]
					if state, ok := hostStateOfEvent[e.Event]; ok {
						last[host] = state
					}
				}
				r.report(t, r.replayEvent(t, e, host))
			}
			kind, state, restored := r.rows(t)
			assert.Equal(t, "rollback", kind, "the release is recorded as a rollback")
			assert.Equal(t, earlier, restored)
			assert.Equal(t, expectedGroup(exit), state, "the server ends the release in the state the plugin exited with")
			got := hostStatesOf(t, r.deployWorld, r.release())
			for host, want := range last {
				if exit == deployrelease.ExitRollbackFailed && want == "verified" {
					want = "unknown"
				}
				assert.Equal(t, want, got[host], "the server keeps the last state the plugin reported for host %s", host)
			}
			for host, s := range got {
				assert.NotContains(t, []string{"pending", "uploaded", "switched"}, s, "host %s is left open after the release ended", host)
			}
		})
	}
}
