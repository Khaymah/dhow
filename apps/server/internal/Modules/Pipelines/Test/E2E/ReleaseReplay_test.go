package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const replayEnv = "DHOW_E2E_REPORTS"

var replayKinds = map[string]runnerv1.DeployEventKind{
	deployrelease.EventStarted:        kindStarted,
	deployrelease.EventUploaded:       kindUploaded,
	deployrelease.EventSwitched:       kindSwitched,
	deployrelease.EventHealth:         kindHealth,
	deployrelease.EventVerified:       kindVerified,
	deployrelease.EventRolledBack:     kindRolled,
	deployrelease.EventRollbackFailed: kindRbFailed,
	deployrelease.EventPruned:         kindPruned,
	deployrelease.EventFailed:         kindFailed,
	deployrelease.EventSkipped:        kindSkipped,
	deployrelease.EventBatchPassed:    kindBatch,
}

var hostStateOfEvent = map[string]string{
	deployrelease.EventVerified:       "verified",
	deployrelease.EventFailed:         "failed",
	deployrelease.EventSkipped:        "skipped",
	deployrelease.EventRolledBack:     "rolled_back",
	deployrelease.EventRollbackFailed: "rollback_failed",
}

const (
	groupScenarioPrefix = "rolling-"
	checkScenarioPrefix = "check-"
)

func (d deployWorld) releasingOneHost(t *testing.T) releaseJob {
	t.Helper()
	d.sql(t, `UPDATE "DeployTargetHosts" SET "IsDeleted" = true WHERE "Host" IN ($1, $2)`, hostTwo, hostThree)
	d.sql(t, `UPDATE "DeployTargets" SET "Settings" = jsonb_set("Settings", '{Rolling,BatchSize}', '1')`)
	run := d.built(t, deployPipeline)
	g := d.leaseDeploy(t)
	_, err := d.runner.Start(g)
	require.NoError(t, err)
	require.NoError(t, d.runner.BeginStep(g.Ref, 1, true))
	return releaseJob{deployWorld: d, run: run, grant: g, release: g.Grant.GetSpec().GetTargets()[0].GetReleaseId()}
}

func (j releaseJob) replayEvent(t *testing.T, e deployrelease.Event, host uuid.UUID) *runnerv1.DeployEvent {
	t.Helper()
	kind, ok := replayKinds[e.Event]
	require.True(t, ok, "the plugin wrote the event %q that the report protocol does not have", e.Event)
	out := &runnerv1.DeployEvent{Kind: kind, Batch: int32(e.Batch), Mode: e.Mode, Reason: e.Reason, Message: e.Message}
	if e.HostID != "" {
		out.HostId = host.String()
	}
	if e.ReleaseID != "" {
		out.ReleaseId = j.release
	}
	if e.PreviousReleaseID != "" {
		out.PreviousReleaseId = "11-aaaaaaaa-0"
	}
	if h := e.Health; h != nil {
		out.Health = &runnerv1.DeployHealth{
			Attempts: int32(h.Attempts), Passed: h.Passed, LastStatus: int32(h.LastStatus), LastError: h.LastError,
			DurationMs: h.DurationMs, StableChecks: int32(h.StableChecks),
		}
	}
	return out
}

func decodeStrict(t *testing.T, line string) deployrelease.Event {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(line)))
	dec.DisallowUnknownFields()
	var e deployrelease.Event
	require.NoError(t, dec.Decode(&e), line)
	return e
}

func expectedGroup(exit int) string {
	switch exit {
	case deployrelease.ExitOK:
		return "succeeded"
	case deployrelease.ExitRolledBack:
		return "rolled_back"
	case deployrelease.ExitRollbackFailed:
		return "rollback_failed"
	default:
		return "failed"
	}
}

func TestTheReportsOfTheRealPluginRunAreAcceptedByTheServerAndEndInTheSameState(t *testing.T) {
	dir := os.Getenv(replayEnv)
	if dir == "" {
		t.Skipf("set %s to the directory the deploy end to end run keeps the plugin reports in (make deploy-e2e does)", replayEnv)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	files = slices.DeleteFunc(files, func(f string) bool { return strings.HasPrefix(filepath.Base(f), checkScenarioPrefix) })
	require.NotEmpty(t, files, "the deploy end to end run left no reports")
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			rawExit, err := os.ReadFile(strings.TrimSuffix(file, ".jsonl") + ".exit")
			require.NoError(t, err)
			exit, err := strconv.Atoi(strings.TrimSpace(string(rawExit)))
			require.NoError(t, err)

			world := newDeployWorld(t)
			var j releaseJob
			if strings.HasPrefix(name, groupScenarioPrefix) {
				j = world.releasing(t)
			} else {
				j = world.releasingOneHost(t)
			}
			j.seedPrevious(t)
			hosts := j.hosts()
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			require.NotEmpty(t, lines)
			last := map[uuid.UUID]string{}
			for _, line := range lines {
				e := decodeStrict(t, line)
				var host uuid.UUID
				if e.HostID != "" {
					host = hosts[int(e.HostID[len(e.HostID)-1]-'1')]
					if state, ok := hostStateOfEvent[e.Event]; ok {
						last[host] = state
					}
				}
				j.report(t, j.replayEvent(t, e, host))
			}
			assert.Equal(t, expectedGroup(exit), j.releaseState(t), "the server ends the release in the state the plugin exited with")
			assertHostsAgree(t, j, last, exit)
			assertHealthKept(t, j, hosts, lines)
		})
	}
}

func assertHostsAgree(t *testing.T, j releaseJob, last map[uuid.UUID]string, exit int) {
	t.Helper()
	got := j.hostStates(t)
	for host, want := range last {
		if exit == deployrelease.ExitRollbackFailed && want == "verified" {
			want = "unknown"
		}
		assert.Equal(t, want, got[host], "the server keeps the last state the plugin reported for host %s", host)
	}
	for host, state := range got {
		assert.NotContains(t, []string{"pending", "uploaded", "switched"}, state, "host %s is left open after the release ended", host)
	}
}

func assertHealthKept(t *testing.T, j releaseJob, hosts []uuid.UUID, lines []string) {
	t.Helper()
	latest := map[string]deployrelease.Event{}
	for _, line := range lines {
		if e := decodeStrict(t, line); e.Event == deployrelease.EventHealth {
			latest[e.HostID] = e
		}
	}
	for id, e := range latest {
		host := hosts[int(id[len(id)-1]-'1')]
		var stored string
		require.NoError(t, j.env.Pool.QueryRow(context.Background(),
			`SELECT COALESCE("Health"::text, '') FROM "DeploymentReleaseHosts" WHERE "ReleaseId" = $1 AND "HostId" = $2`, j.release, host).Scan(&stored))
		var got struct {
			Passed     bool
			LastStatus int
		}
		require.NoError(t, json.Unmarshal([]byte(stored), &got), stored)
		assert.Equal(t, e.Health.Passed, got.Passed, "the health result of the last check is what the server keeps")
		assert.Equal(t, e.Health.LastStatus, got.LastStatus)
	}
}
