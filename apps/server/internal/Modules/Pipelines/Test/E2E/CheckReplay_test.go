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

func TestTheReportsOfTheRealPluginRunOfACheckAreStoredPerHostWithTheReasonItGave(t *testing.T) {
	dir := os.Getenv(replayEnv)
	if dir == "" {
		t.Skipf("set %s to the directory the deploy end to end run keeps the plugin reports in (make deploy-e2e does)", replayEnv)
	}
	files, err := filepath.Glob(filepath.Join(dir, checkScenarioPrefix+"*.jsonl"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "the deploy end to end run left no check report")
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".jsonl"), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			rawExit, err := os.ReadFile(strings.TrimSuffix(file, ".jsonl") + ".exit")
			require.NoError(t, err)
			exit, err := strconv.Atoi(strings.TrimSpace(string(rawExit)))
			require.NoError(t, err)

			c := newDeployWorld(t).checking(t)
			hosts := c.hosts()
			want := map[uuid.UUID]deployrelease.Event{}
			var events []*runnerv1.DeployEvent
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				e := decodeStrict(t, line)
				require.Equal(t, deployrelease.EventChecked, e.Event)
				host := hosts[int(e.HostID[len(e.HostID)-1]-'1')]
				want[host] = e
				events = append(events, &runnerv1.DeployEvent{
					Kind: kindChecked, HostId: host.String(), Reason: e.Reason, Message: e.Message, CurrentReleaseId: e.CurrentReleaseID,
					VerifiedReleaseId: e.VerifiedReleaseID, FreeBytes: e.FreeBytes, Locked: e.Locked,
				})
			}
			c.report(t, events...)

			got := map[string]struct {
				Reason, CurrentReleaseId, VerifiedReleaseId string
				FreeBytes                                   *int64
			}{}
			for _, h := range c.result(t) {
				got[h.HostId] = struct {
					Reason, CurrentReleaseId, VerifiedReleaseId string
					FreeBytes                                   *int64
				}{h.Reason, h.CurrentReleaseId, h.VerifiedReleaseId, h.FreeBytes}
			}
			for host, e := range want {
				stored := got[host.String()]
				assert.Equal(t, e.Reason, stored.Reason, "the server keeps the reason the plugin gave for host %s", host)
				assert.Equal(t, e.CurrentReleaseID, stored.CurrentReleaseId)
				assert.Equal(t, e.VerifiedReleaseID, stored.VerifiedReleaseId)
			}
			assert.Equal(t, deployrelease.ExitCheckFailed == exit, anyBlocking(want), "the plugin exits non-zero exactly when a server is not ready")
		})
	}
}

func anyBlocking(events map[uuid.UUID]deployrelease.Event) bool {
	for _, e := range events {
		switch e.Reason {
		case deployrelease.CheckOK, deployrelease.CheckDiverged:
		default:
			return true
		}
	}
	return false
}
