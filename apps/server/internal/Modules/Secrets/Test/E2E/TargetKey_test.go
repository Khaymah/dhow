package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const (
	targetKeyName  = "PROD_DEPLOY_KEY"
	firstKeyValue  = "first value of the target key"
	secondKeyValue = "second value of the target key"
)

func apptestRequest(method, path string, body any, headers map[string]string) apptest.Request {
	return apptest.Request{Method: method, Path: path, Body: body, Headers: headers}
}

func (w world) attachKey(t *testing.T, secret string) {
	t.Helper()
	target, _ := w.env.SeedDeployTarget(w.project, "production", "prod")
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "DeployTargets" SET "KeySecretName" = $2 WHERE "Id" = $1`, target, secret)
	require.NoError(t, err)
}

func (w world) factor() map[string]any {
	return map[string]any{"Method": "password", "Password": w.password}
}

func (w world) putKey(t *testing.T, name string, body map[string]any, headers map[string]string) (status int, code string, version int) {
	t.Helper()
	res := w.owner.Do(apptestRequest(http.MethodPut, w.envPath("production", "/secrets/"+name), body, headers))
	if res.Error != nil {
		return res.Status, res.Error.Code, 0
	}
	return res.Status, "", secretOf(t, res, http.StatusOK).Version
}

func TestOverwritingTheKeyOfADeployTargetNeedsAFreshFactor(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "production")
	status, _, version := w.putKey(t, targetKeyName, map[string]any{"Value": firstKeyValue}, nil)
	require.Equal(t, http.StatusOK, status, "a secret nobody uses as a key needs no factor")
	require.Equal(t, 1, version)
	w.attachKey(t, targetKeyName)

	status, code, _ := w.putKey(t, targetKeyName, map[string]any{"Value": secondKeyValue}, nil)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "TARGET_KEY_NEEDS_FACTOR", code)

	status, code, _ = w.putKey(t, targetKeyName, map[string]any{"Value": secondKeyValue, "CurrentFactor": map[string]any{"Method": "password", "Password": "not the password"}}, nil)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "INVALID_CURRENT_FACTOR", code)

	listed := w.owner.Do(apptestRequest(http.MethodGet, w.envPath("production", "/secrets"), nil, nil))
	assertNoValue(t, listed.Raw, firstKeyValue, secondKeyValue)
	assert.Contains(t, string(listed.Raw), `"Version":1`, "a refused write must not make a version")

	status, code, version = w.putKey(t, targetKeyName, map[string]any{"Value": secondKeyValue, "CurrentFactor": w.factor()}, nil)
	assert.Equal(t, http.StatusOK, status, code)
	assert.Equal(t, 2, version)
}

func TestRestoringTheKeyOfADeployTargetNeedsAFreshFactorToo(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "production")
	for _, value := range []string{firstKeyValue, secondKeyValue} {
		status, _, _ := w.putKey(t, targetKeyName, map[string]any{"Value": value}, nil)
		require.Equal(t, http.StatusOK, status)
	}
	w.attachKey(t, targetKeyName)
	path := w.envPath("production", "/secrets/"+targetKeyName+"/restore-version")

	requireError(t, post(w.owner, path, map[string]any{"Version": 1}), http.StatusForbidden, "TARGET_KEY_NEEDS_FACTOR")
	res := post(w.owner, path, map[string]any{"Version": 1, "CurrentFactor": w.factor()})
	assert.Equal(t, 3, secretOf(t, res, http.StatusOK).Version)
	assertNoValue(t, res.Raw, firstKeyValue, secondKeyValue)
}

func TestOnlyTheKeyOfADeployTargetIsGuarded(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "production")
	w.attachKey(t, targetKeyName)

	for _, name := range []string{"DATABASE_URL", "PROD_DEPLOY_KEY_BACKUP"} {
		status, code, _ := w.putKey(t, name, map[string]any{"Value": firstKeyValue}, nil)
		assert.Equal(t, http.StatusOK, status, "%s: %s", name, code)
		status, code, _ = w.putKey(t, name, map[string]any{"Value": secondKeyValue}, nil)
		assert.Equal(t, http.StatusOK, status, "%s overwritten: %s", name, code)
	}
	res := put(w.owner, w.projectPath("/secrets/"+targetKeyName), map[string]any{"Value": firstKeyValue})
	assert.Equal(t, http.StatusOK, res.Status, "a project secret with the same name is another secret: %s", res.Raw)
	res = put(w.owner, w.orgPath("/secrets/"+targetKeyName), map[string]any{"Value": firstKeyValue})
	assert.Equal(t, http.StatusOK, res.Status, "an organization secret with the same name is another secret: %s", res.Raw)
}

func TestAKeyNameThatIsFreeButReservedByATargetAlsoNeedsTheFactor(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "production")
	w.attachKey(t, targetKeyName)

	status, code, _ := w.putKey(t, targetKeyName, map[string]any{"Value": firstKeyValue}, nil)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "TARGET_KEY_NEEDS_FACTOR", code, "re-creating the secret of a target is writing its key")
	status, code, version := w.putKey(t, targetKeyName, map[string]any{"Value": firstKeyValue, "CurrentFactor": w.factor()}, nil)
	assert.Equal(t, http.StatusOK, status, code)
	assert.Equal(t, 1, version)
}

func TestTheRefusalIsTranslatedByTheServer(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "production")
	w.attachKey(t, targetKeyName)
	for lang, want := range map[string]string{"ar": "مفتاح هدف نشر", "en": "key of a deploy target"} {
		res := w.owner.Do(apptestRequest(http.MethodPut, w.envPath("production", "/secrets/"+targetKeyName), map[string]any{"Value": firstKeyValue}, map[string]string{"Accept-Language": lang}))
		require.NotNil(t, res.Error, string(res.Raw))
		assert.Contains(t, res.Error.Message, want, lang)
		assertNoValue(t, res.Raw, firstKeyValue)
	}
}

func TestRemovingTheKeyOfADeployTargetNeedsAFreshFactorToo(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "production")
	for _, name := range []string{targetKeyName, "DATABASE_URL"} {
		status, _, _ := w.putKey(t, name, map[string]any{"Value": firstKeyValue}, nil)
		require.Equal(t, http.StatusOK, status)
	}
	w.attachKey(t, targetKeyName)
	path := w.envPath("production", "/secrets/"+targetKeyName+"/remove")

	requireError(t, post(w.owner, path, nil), http.StatusForbidden, "TARGET_KEY_NEEDS_FACTOR")
	requireError(t, post(w.owner, path, map[string]any{"CurrentFactor": map[string]any{"Method": "password", "Password": "not the password"}}),
		http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	listed := w.owner.Do(apptestRequest(http.MethodGet, w.envPath("production", "/secrets"), nil, nil))
	assert.Contains(t, string(listed.Raw), targetKeyName, "a refused removal must remove nothing")

	res := post(w.owner, path, map[string]any{"CurrentFactor": w.factor()})
	assert.Equal(t, targetKeyName, secretOf(t, res, http.StatusOK).Name)
	listed = w.owner.Do(apptestRequest(http.MethodGet, w.envPath("production", "/secrets"), nil, nil))
	assert.NotContains(t, string(listed.Raw), targetKeyName)

	other := post(w.owner, w.envPath("production", "/secrets/DATABASE_URL/remove"), nil)
	assert.Equal(t, http.StatusOK, other.Status, "a secret nobody uses as a key needs no factor: %s", other.Raw)
}
