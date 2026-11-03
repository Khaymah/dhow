package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func TestUnlinkStopsTheWebhookAndARelinkMakesNewCredentials(t *testing.T) {
	w := newWorld(t, false)
	r, secret := linkHTTPS(t, w)

	done := w.owner.Do(apptest.Request{Method: http.MethodDelete, Path: w.path("")})
	assert.Equal(t, "sources.repository.unlinked", done.Message)
	assert.Equal(t, r.Id, repo(t, done).Id)
	assert.NotContains(t, string(done.Raw), secret)
	requireError(t, w.owner.Get(w.path("")), http.StatusNotFound, "REPOSITORY_NOT_LINKED")
	requireError(t, w.owner.Do(apptest.Request{Method: http.MethodDelete, Path: w.path("")}), http.StatusNotFound, "REPOSITORY_NOT_LINKED")

	body := giteaPush("refs/heads/main", sha)
	res := w.send(hookPath(r), giteaHeaders(secret, "delivery-after-unlink", body), body)
	assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
	assert.Empty(t, w.triggers.Received())

	again, newSecret := linkHTTPS(t, w)
	assert.NotEqual(t, r.Id, again.Id)
	assert.NotEqual(t, r.WebhookUrl, again.WebhookUrl)
	assert.NotEqual(t, secret, newSecret)

	audit := auditData(t, w)
	assert.Contains(t, audit, "project.repository_unlinked")
	assert.NotContains(t, audit, secret)
	assert.NotContains(t, audit, newSecret)
}

func TestOnlyAManagerMayUnlinkAndAnArchivedProjectRefuses(t *testing.T) {
	w := newWorld(t, false)
	linkHTTPS(t, w)
	developer := w.env.SeedSession(w.env.SeedUser(w.org, "dev-unlink@example.com", "a long password for the developer", "developer"))
	requireError(t, developer.Do(apptest.Request{Method: http.MethodDelete, Path: w.path("")}), http.StatusForbidden, "FORBIDDEN")

	archived := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/archive")})
	require.Equal(t, http.StatusOK, archived.Status, string(archived.Raw))
	requireError(t, w.owner.Do(apptest.Request{Method: http.MethodDelete, Path: w.path("")}), http.StatusConflict, "PROJECT_ARCHIVED")
}
