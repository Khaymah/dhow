package e2e_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func (w world) projectPath(rest string) string {
	return apptest.OrgPath(w.org, "/projects/"+w.project.String()+rest)
}

func requireCode(t *testing.T, res apptest.Response, status int, code string) {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	require.NotNil(t, res.Error, string(res.Raw))
	assert.Equal(t, code, res.Error.Code)
}

func TestUnlinkCancelsTheOpenRunsAndBlocksNewOnes(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(simplePipeline))
	pending := w.push("refs/heads/main", shaA)
	awaiting := w.env.SeedRun(w.project, "awaiting_approval")
	done := w.env.SeedRun(w.project, "pending")
	_, err := w.env.Pool.Exec(t.Context(), `UPDATE "PipelineRuns" SET "State" = 'skipped', "FinishedAt" = now() WHERE "Id" = $1`, done)
	require.NoError(t, err)

	res := w.owner.Do(apptest.Request{Method: http.MethodDelete, Path: w.projectPath("/repository")})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	for _, id := range []uuid.UUID{pending, awaiting} {
		state, reason := w.env.RunState(id)
		assert.Equal(t, []string{"canceled", "REPOSITORY_UNLINKED"}, []string{state, reason})
	}
	state, _ := w.env.RunState(done)
	assert.Equal(t, "skipped", state, "a finished run keeps its result")

	later := w.env.SeedRun(w.project, "awaiting_approval")
	requireCode(t, w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/runs/" + later.String() + "/approve")}),
		http.StatusNotFound, "REPOSITORY_NOT_LINKED")
	requireCode(t, w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/runs/" + pending.String() + "/rerun-failed")}),
		http.StatusNotFound, "REPOSITORY_NOT_LINKED")
	requireCode(t, w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/runs"), Body: map[string]any{"Ref": "main"}}),
		http.StatusNotFound, "REPOSITORY_NOT_LINKED")
	state, _ = w.env.RunState(later)
	assert.Equal(t, "awaiting_approval", state)
}
