//go:build dhowtest

package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestAResultReportedAfterARestartKeepsTheJobOutputs(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	srv.Configure(func(k *runnertest.Knobs) { k.CompleteUnavailable = 1 << 20 })
	srv.Enqueue(runnertest.Spec(), nil)
	first := r.Start(fake.Recording{Steps: []fake.Step{{Outputs: "version=1.2.3\ndhow_found=true\n"}}}, runnertest.StartOptions{})
	runnertest.Eventually(t, finishWithin, func() bool {
		raw, _ := os.ReadFile(filepath.Join(r.StateDir, "journal"))
		return strings.Contains(string(raw), "Completion")
	}, "the result in the journal")
	require.NoError(t, first.Stop())
	l := srv.Leases()[0]
	got, _ := srv.Lease(l.ID)
	require.Empty(t, got.Completes, "the server never took the first report")

	srv.Configure(func(k *runnertest.Knobs) { k.CompleteUnavailable = 0 })
	r.Start(fake.Recording{}, runnertest.StartOptions{})
	runnertest.Eventually(t, finishWithin, func() bool {
		got, _ := srv.Lease(l.ID)
		return len(got.Completes) > 0
	}, "the report after the restart")
	got, _ = srv.Lease(l.ID)
	c := got.Completes[0]
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, c.GetResult())
	assert.Equal(t, map[string]string{"version": "1.2.3", "dhow_found": "true"}, c.GetOutputs(), "the outputs survive the restart")
	runnertest.Eventually(t, finishWithin, func() bool {
		raw, _ := os.ReadFile(filepath.Join(r.StateDir, "journal"))
		return !strings.Contains(string(raw), "1.2.3")
	}, "the journal to forget the outputs with the lease")
}
