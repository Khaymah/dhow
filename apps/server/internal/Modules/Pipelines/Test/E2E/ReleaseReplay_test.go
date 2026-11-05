package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
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
}

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

			j := newDeployWorld(t).releasingOneHost(t)
			j.seedPrevious(t)
			host := j.hosts()[0]
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			require.NotEmpty(t, lines)
			for _, line := range lines {
				j.report(t, j.replayEvent(t, decodeStrict(t, line), host))
			}
			assert.Equal(t, expectedGroup(exit), j.releaseState(t), "the server ends the release in the state the plugin exited with")
			assertHealthKept(t, j, lines)
		})
	}
}

func assertHealthKept(t *testing.T, j releaseJob, lines []string) {
	t.Helper()
	for _, line := range lines {
		e := decodeStrict(t, line)
		if e.Event != deployrelease.EventHealth {
			continue
		}
		var stored string
		require.NoError(t, j.env.Pool.QueryRow(context.Background(),
			`SELECT COALESCE("Health"::text, '') FROM "DeploymentReleaseHosts" WHERE "ReleaseId" = $1`, j.release).Scan(&stored))
		var got struct {
			Passed     bool
			LastStatus int
		}
		require.NoError(t, json.Unmarshal([]byte(stored), &got), stored)
		assert.Equal(t, e.Health.Passed, got.Passed, "the health result of the failed check is what the server keeps")
		assert.Equal(t, e.Health.LastStatus, got.LastStatus)
	}
}
