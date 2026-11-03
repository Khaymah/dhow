package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func TestAMaintainerPausesButNeverResumesOrEditsTheRules(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	maintainer, _ := w.member(t, "maintainer")

	requireError(t, w.create(maintainer, map[string]any{"Name": "staging"}), http.StatusForbidden, "FORBIDDEN")
	requireError(t, w.update(maintainer, "production", map[string]any{
		"Version": e.Version, "Rules": map[string]any{"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": "**"}}},
		"CurrentFactor": memberFactor(),
	}), http.StatusForbidden, "FORBIDDEN")
	requireError(t, w.post(maintainer, "production", "remove", map[string]any{"CurrentFactor": memberFactor()}), http.StatusForbidden, "FORBIDDEN")

	requireError(t, w.post(maintainer, "production", "pause", map[string]string{"Reason": "   "}), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	paused := decode(t, w.post(maintainer, "production", "pause", map[string]string{"Reason": "Database migration failed"}), http.StatusOK)
	assert.True(t, paused.Paused)
	require.NotNil(t, paused.PausedReason)
	assert.Equal(t, "Database migration failed", *paused.PausedReason)
	require.NotNil(t, paused.PausedAt)
	require.NotNil(t, paused.PausedByUserId)

	requireError(t, w.post(maintainer, "production", "resume", map[string]any{"CurrentFactor": memberFactor()}), http.StatusForbidden, "FORBIDDEN")
	assert.True(t, decode(t, w.get(maintainer, "production"), http.StatusOK).Paused, "the maintainer could not resume")

	requireError(t, w.post(w.owner, "production", "resume", map[string]any{"CurrentFactor": wrongFactor()}), http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	assert.True(t, decode(t, w.get(w.owner, "production"), http.StatusOK).Paused, "a wrong factor resumes nothing")

	admin, _ := w.member(t, "admin")
	resumed := decode(t, w.post(admin, "production", "resume", map[string]any{"CurrentFactor": memberFactor()}), http.StatusOK)
	assert.False(t, resumed.Paused)
	assert.Nil(t, resumed.PausedReason)
	assert.Nil(t, resumed.PausedAt)
}

func TestLowerRolesOnlyRead(t *testing.T) {
	w := newWorld(t)
	w.production(t)
	for _, role := range []string{"developer", "viewer"} {
		t.Run(role, func(t *testing.T) {
			c, _ := w.member(t, role)
			decode(t, w.get(c, "production"), http.StatusOK)
			requireError(t, w.create(c, map[string]any{"Name": "staging"}), http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.post(c, "production", "pause", map[string]string{"Reason": "x"}), http.StatusForbidden, "FORBIDDEN")
			requireError(t, w.post(c, "production", "resume", map[string]any{"CurrentFactor": memberFactor()}), http.StatusForbidden, "FORBIDDEN")
		})
	}
}

func TestAnAdminBindingManagesTheEnvironmentsOfThatProjectOnly(t *testing.T) {
	w := newWorld(t)
	other := w.env.SeedProject(w.org, w.ownerID)
	developer, id := w.member(t, "developer")
	w.env.SeedUserProjectBinding(w.org, w.project, id, "admin")

	e := decode(t, w.create(developer, map[string]any{"Name": "production"}), http.StatusCreated)
	decode(t, w.update(developer, "production", map[string]any{
		"Version": e.Version, "Rules": map[string]any{"WaitMinutes": 1}, "CurrentFactor": memberFactor(),
	}), http.StatusOK)
	res := developer.Do(apptest.Request{Method: http.MethodPost, Path: w.path(other, ""), Body: map[string]any{"Name": "production"}})
	requireError(t, res, http.StatusForbidden, "FORBIDDEN")

	maintainer, mid := w.member(t, "maintainer")
	w.env.SeedTeamProjectBinding(w.org, w.project, mid, "admin")
	decode(t, w.create(maintainer, map[string]any{"Name": "staging"}), http.StatusCreated)
	res = maintainer.Do(apptest.Request{Method: http.MethodPost, Path: w.path(other, ""), Body: map[string]any{"Name": "staging"}})
	requireError(t, res, http.StatusForbidden, "FORBIDDEN")
}

func TestAPITokensReadButNeverChangeEnvironments(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	token := w.env.SeedAPIToken(w.ownerID, w.org, "project.read", "project.manage")
	read := w.env.Bearer(token.Value, apptest.Request{Method: http.MethodGet, Path: w.path(w.project, "/production"), Headers: map[string]string{"Origin": ""}})
	require.Equal(t, http.StatusOK, read.Status, string(read.Raw))
	for _, req := range []apptest.Request{
		{Method: http.MethodPost, Path: w.path(w.project, ""), Body: map[string]any{"Name": "staging"}},
		{Method: http.MethodPut, Path: w.path(w.project, "/production"), Body: map[string]any{"Version": e.Version, "CurrentFactor": w.ownerFactor()}},
		{Method: http.MethodPost, Path: w.path(w.project, "/production/pause"), Body: map[string]any{"Reason": "token"}},
	} {
		req.Headers = map[string]string{"Origin": ""}
		res := w.env.Bearer(token.Value, req)
		requireError(t, res, http.StatusForbidden, "SESSION_REQUIRED")
	}
}
