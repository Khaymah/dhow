package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func TestSecretLimits(t *testing.T) {
	w := newWorld(t)
	base := w.projectPath("/secrets/")
	requireError(t, put(w.owner, base+"SHORT", map[string]string{"Value": "1234567"}), http.StatusUnprocessableEntity, "SECRET_TOO_SHORT")
	assert.Equal(t, http.StatusOK, put(w.owner, base+"EXACT", map[string]string{"Value": "12345678"}).Status, "8 bytes is enough")
	requireError(t, put(w.owner, base+"LARGE", map[string]string{"Value": strings.Repeat("x", 32*1024+1)}), http.StatusUnprocessableEntity, "SECRET_TOO_LARGE")
	assert.Equal(t, http.StatusOK, put(w.owner, base+"LARGEST", map[string]string{"Value": strings.Repeat("x", 32*1024)}).Status)
	requireError(t, put(w.owner, base+"DHOW_TOKEN", map[string]string{"Value": "a long value"}), http.StatusUnprocessableEntity, "SECRET_NAME_RESERVED")
	requireError(t, put(w.owner, base+"dhow_token", map[string]string{"Value": "a long value"}), http.StatusUnprocessableEntity, "SECRET_NAME_RESERVED")
	requireError(t, put(w.owner, base+"WITH_NUL", map[string]string{"Value": "a long\x00value"}), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	requireError(t, put(w.owner, base+"1NAME", map[string]string{"Value": "a long value"}), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	requireError(t, put(w.owner, base+"HIDDEN", map[string]any{"Value": "a long value", "Description": "bad\u202etext"}), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	requireError(t, put(w.owner, base+"PICKED", map[string]any{"Value": "a long value", "Visibility": "selected_projects"}), http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	first := secretOf(t, put(w.owner, base+"Token", map[string]string{"Value": "first value"}), http.StatusOK)
	second := secretOf(t, put(w.owner, base+"TOKEN", map[string]string{"Value": "second value"}), http.StatusOK)
	assert.Equal(t, "Token", second.Name, "the name keeps the case it was made with")
	assert.Equal(t, first.Version+1, second.Version, "Token and TOKEN are the same secret")

	_, err := w.env.Pool.Exec(context.Background(),
		`INSERT INTO "Secrets" ("OrganizationId", "ProjectId", "Scope", "Name", "CreatedByUserId", "UpdatedByUserId")
		 SELECT $1, $2, 'project', 'FILL_' || n, $3, $3 FROM generate_series(1, 200) AS n`, w.org, w.project, w.ownerID)
	require.NoError(t, err)
	requireError(t, put(w.owner, base+"ONE_MORE", map[string]string{"Value": "a long value"}), http.StatusConflict, "SECRET_LIMIT_REACHED")
	assert.Equal(t, http.StatusOK, put(w.owner, base+"Token", map[string]string{"Value": "third value"}).Status, "a new version is not a new secret")
}

func TestSecretPermissions(t *testing.T) {
	w := newWorld(t)
	_ = w.env.SeedEnvironment(w.project, "production")
	maintainer, developer, viewer := w.member(t, "maintainer"), w.member(t, "developer"), w.member(t, "viewer")
	body := map[string]string{"Value": "a long value"}
	assert.Equal(t, http.StatusOK, put(maintainer, w.projectPath("/secrets/A_SECRET"), body).Status, "a maintainer writes project secrets")
	requireError(t, put(maintainer, w.orgPath("/secrets/A_SECRET"), body), http.StatusForbidden, "FORBIDDEN")
	requireError(t, put(maintainer, w.envPath("production", "/secrets/A_SECRET"), body), http.StatusForbidden, "FORBIDDEN")
	requireError(t, put(developer, w.projectPath("/secrets/A_SECRET"), body), http.StatusForbidden, "FORBIDDEN")
	assert.Equal(t, http.StatusOK, developer.Get(w.projectPath("/secrets")).Status, "a developer reads the names")
	requireError(t, viewer.Get(w.projectPath("/secrets")), http.StatusForbidden, "FORBIDDEN")
	assert.Equal(t, http.StatusOK, viewer.Get(w.projectPath("/variables")).Status, "variables are readable with project.read")

	token := w.env.SeedAPIToken(w.ownerID, w.org, "secret.read", "project.read")
	headers := map[string]string{"Authorization": "Bearer " + token.Value, "Origin": ""}
	listed := w.env.Do(apptest.Request{Method: http.MethodGet, Path: w.projectPath("/secrets"), Headers: headers})
	assert.Equal(t, http.StatusOK, listed.Status, string(listed.Raw))
	written := w.env.Do(apptest.Request{Method: http.MethodPut, Path: w.projectPath("/secrets/BY_TOKEN"), Headers: headers, Body: body})
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, written.Status, "a token never writes a secret: %s", written.Raw)
}

func TestTheArchivedProjectKeepsItsSecretsReadOnly(t *testing.T) {
	w := newWorld(t)
	assert.Equal(t, http.StatusOK, put(w.owner, w.projectPath("/secrets/KEPT"), map[string]string{"Value": "a long value"}).Status)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Projects" SET "State" = 'archived', "ArchivedAt" = now() WHERE "Id" = $1`, w.project)
	require.NoError(t, err)
	requireError(t, put(w.owner, w.projectPath("/secrets/KEPT"), map[string]string{"Value": "another value"}), http.StatusConflict, "PROJECT_ARCHIVED")
	assert.Equal(t, http.StatusOK, w.owner.Get(w.projectPath("/secrets")).Status)
}
