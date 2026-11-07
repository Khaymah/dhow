package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const (
	reloadHook = "systemctl reload shop"
	preHook    = "/opt/shop/bin/warm-cache --quiet"
)

func (w world) hookedTarget(t *testing.T) targetView {
	t.Helper()
	w.production(t)
	body := w.targetBody("web")
	body["Settings"] = map[string]any{"Ssh": map[string]any{"ReloadCommand": reloadHook, "PreSwitchCommand": preHook}}
	return decodeTarget(t, w.createTarget(w.owner, "production", body), http.StatusCreated)
}

func requireHooksHidden(t *testing.T, who string, res apptest.Response) {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Status, "%s: %s", who, res.Raw)
	assert.NotContains(t, string(res.Raw), reloadHook, who)
	assert.NotContains(t, string(res.Raw), preHook, who)
	assert.NotContains(t, string(res.Raw), "warm-cache", who)
}

func requireFlags(t *testing.T, who string, v targetView) {
	t.Helper()
	require.NotNil(t, v.Settings.Ssh, who)
	assert.Empty(t, v.Settings.Ssh.ReloadCommand, who)
	assert.Empty(t, v.Settings.Ssh.PreSwitchCommand, who)
	assert.True(t, v.Settings.Ssh.ReloadCommandSet, who)
	assert.True(t, v.Settings.Ssh.PreSwitchCommandSet, who)
	assert.False(t, v.Settings.Ssh.PostSwitchCommandSet, who)
}

func TestHookCommandsAreShownOnlyToWhoCanManageTheEnvironment(t *testing.T) {
	w := newTargetWorld(t)
	created := w.hookedTarget(t)
	require.NotNil(t, created.Settings.Ssh)
	assert.Equal(t, reloadHook, created.Settings.Ssh.ReloadCommand, "the admin who wrote the hook sees it")
	assert.True(t, created.Settings.Ssh.ReloadCommandSet)

	admin, _ := w.member(t, "admin")
	for _, rest := range []string{"", "/web"} {
		res := w.targetCall(admin, http.MethodGet, "production", rest, nil)
		require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
		assert.Contains(t, string(res.Raw), reloadHook, "an admin keeps seeing the text: %q", rest)
	}

	for _, role := range []string{"maintainer", "developer", "viewer"} {
		t.Run(role, func(t *testing.T) {
			c, _ := w.member(t, role)
			list := w.targetCall(c, http.MethodGet, "production", "", nil)
			requireHooksHidden(t, role+" list", list)
			one := w.targetCall(c, http.MethodGet, "production", "/web", nil)
			requireHooksHidden(t, role+" target", one)
			requireFlags(t, role, decodeTarget(t, one, http.StatusOK))
			for _, rest := range []string{"/web/check", "/web/releases"} {
				res := w.targetCall(c, http.MethodGet, "production", rest, nil)
				assert.NotContains(t, string(res.Raw), reloadHook, "%s %s", role, rest)
				assert.NotContains(t, string(res.Raw), preHook, "%s %s", role, rest)
			}
		})
	}
}

func TestAnAPITokenNeverSeesHookCommands(t *testing.T) {
	w := newTargetWorld(t)
	w.hookedTarget(t)
	token := w.env.SeedAPIToken(w.ownerID, w.org, "project.read", "project.manage")
	for _, rest := range []string{"", "/web"} {
		res := w.env.Bearer(token.Value, apptest.Request{
			Method: http.MethodGet, Path: w.targetsPath("production", rest), Headers: map[string]string{"Origin": ""},
		})
		requireHooksHidden(t, "an owner's API token "+rest, res)
	}
	one := w.env.Bearer(token.Value, apptest.Request{
		Method: http.MethodGet, Path: w.targetsPath("production", "/web"), Headers: map[string]string{"Origin": ""},
	})
	requireFlags(t, "token", decodeTarget(t, one, http.StatusOK))
}

func TestAProjectAdminBindingShowsTheHooksToAnOrganizationViewer(t *testing.T) {
	w := newTargetWorld(t)
	w.hookedTarget(t)
	viewer, id := w.member(t, "viewer")
	hidden := w.targetCall(viewer, http.MethodGet, "production", "/web", nil)
	requireHooksHidden(t, "a viewer", hidden)

	w.env.SeedUserProjectBinding(w.org, w.project, id, "admin")
	shown := w.targetCall(viewer, http.MethodGet, "production", "/web", nil)
	require.Equal(t, http.StatusOK, shown.Status, string(shown.Raw))
	assert.Contains(t, string(shown.Raw), reloadHook, "the project role decides, as for every project action")
}
