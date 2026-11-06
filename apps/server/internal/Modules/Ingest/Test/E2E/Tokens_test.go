package e2e_test

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

type tokenView struct {
	Id, Name, Prefix, Status string
	RefPatterns              []struct{ Kind, Pattern string }
}

func (w world) tokensPath(suffix string) string {
	return apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/ingest-tokens"+suffix)
}

func (w world) createToken(body map[string]any) apptest.Response {
	if _, ok := body["CurrentFactor"]; !ok {
		body["CurrentFactor"] = apptest.PasswordFactor(w.password)
	}
	return w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.tokensPath(""), Body: body})
}

func mainOnly() []map[string]string { return []map[string]string{{"Kind": "branch", "Pattern": "main"}} }

func TestATokenIsShownOnceAndOnlyItsHashIsKept(t *testing.T) {
	w := newWorld(t)
	res := w.createToken(map[string]any{"Name": "github", "RefPatterns": mainOnly(), "ExpiresInDays": 30})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	assert.Equal(t, "ingest.token.created", res.Message)
	var created struct {
		Token tokenView
		Value string
	}
	res.Decode(t, &created)
	assert.True(t, strings.HasPrefix(created.Value, config.IngestTokenPrefix))
	assert.Len(t, created.Value, len(config.IngestTokenPrefix)+43)
	assert.Equal(t, created.Value[:len(config.IngestTokenPrefix)+8], created.Token.Prefix)
	assert.Equal(t, "active", created.Token.Status)

	var hash []byte
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "TokenHash" FROM "IngestTokens" WHERE "Id" = $1`, created.Token.Id).Scan(&hash))
	sum := sha256.Sum256([]byte(created.Value))
	assert.Equal(t, sum[:], hash)
	var stored int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "IngestTokens" t WHERE t::text LIKE '%' || $1 || '%'`, created.Value).Scan(&stored))
	assert.Zero(t, stored, "the value is not stored")

	list := w.owner.Get(w.tokensPath(""))
	require.Equal(t, http.StatusOK, list.Status, string(list.Raw))
	assert.NotContains(t, string(list.Raw), created.Value, "a list never shows a value")
	var items []tokenView
	list.Decode(t, &items)
	assert.Len(t, items, 2, "the seeded token and the new one")

	up := w.upload(w.archive(), func(u *apptest.IngestUpload) { u.Token = created.Value })
	assert.Equal(t, http.StatusCreated, up.Status, string(up.Raw))
}

func TestARevokedTokenStopsAtOnce(t *testing.T) {
	w := newWorld(t)
	res := w.owner.Do(apptest.Request{
		Method: http.MethodPost, Path: w.tokensPath("/" + w.token.ID.String() + "/revoke"),
		Body: map[string]any{"CurrentFactor": apptest.PasswordFactor(w.password)},
	})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, "ingest.token.revoked", res.Message)
	var got tokenView
	res.Decode(t, &got)
	assert.Equal(t, "revoked", got.Status)

	up := w.upload(w.archive(), nil)
	assert.Equal(t, http.StatusUnauthorized, up.Status, string(up.Raw))
	require.NotNil(t, up.Error)
	assert.Equal(t, "INGEST_TOKEN_REVOKED", up.Error.Code)
	again := w.owner.Do(apptest.Request{
		Method: http.MethodPost, Path: w.tokensPath("/" + w.token.ID.String() + "/revoke"),
		Body: map[string]any{"CurrentFactor": apptest.PasswordFactor(w.password)},
	})
	assert.Equal(t, http.StatusOK, again.Status, "revoking twice changes nothing")
}

func TestATokenNeedsAGoodNamePatternsAndExpiry(t *testing.T) {
	w := newWorld(t)
	tooMany := make([]map[string]string, 0, 21)
	for range 21 {
		tooMany = append(tooMany, map[string]string{"Kind": "branch", "Pattern": "main"})
	}
	tests := map[string]map[string]any{
		"no name":              {"Name": "", "RefPatterns": mainOnly()},
		"a name with a tab":    {"Name": "bad\tname", "RefPatterns": mainOnly()},
		"no pattern":           {"Name": "a", "RefPatterns": []map[string]string{}},
		"a bracket pattern":    {"Name": "b", "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": "[a-z]"}}},
		"an empty pattern":     {"Name": "c", "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": ""}}},
		"a kind that is wrong": {"Name": "d", "RefPatterns": []map[string]string{{"Kind": "pull", "Pattern": "main"}}},
		"too many patterns":    {"Name": "e", "RefPatterns": tooMany},
		"a year and a day":     {"Name": "f", "RefPatterns": mainOnly(), "ExpiresInDays": 366},
		"a negative expiry":    {"Name": "g", "RefPatterns": mainOnly(), "ExpiresInDays": -1},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			res := w.createToken(body)
			assert.Contains(t, []int{http.StatusUnprocessableEntity, http.StatusBadRequest}, res.Status, string(res.Raw))
			require.NotNil(t, res.Error)
		})
	}
	first := w.createToken(map[string]any{"Name": "same", "RefPatterns": mainOnly()})
	require.Equal(t, http.StatusCreated, first.Status, string(first.Raw))
	second := w.createToken(map[string]any{"Name": "same", "RefPatterns": mainOnly()})
	assert.Equal(t, http.StatusConflict, second.Status, string(second.Raw))
	require.NotNil(t, second.Error)
	assert.Equal(t, "INGEST_TOKEN_NAME_TAKEN", second.Error.Code)
}

func TestAWrongFactorMakesNoToken(t *testing.T) {
	w := newWorld(t)
	res := w.createToken(map[string]any{
		"Name": "x", "RefPatterns": mainOnly(), "CurrentFactor": map[string]string{"Method": "password", "Password": "not the password"},
	})
	assert.Equal(t, http.StatusForbidden, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "INVALID_CURRENT_FACTOR", res.Error.Code)
	assert.Equal(t, 1, w.countTokens(t), "only the seeded one")
}

func (w world) countTokens(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "IngestTokens" WHERE "ProjectId" = $1`, w.project).Scan(&n))
	return n
}

func TestOnlyMaintainersAndAboveManageTokensAndAnAPITokenNever(t *testing.T) {
	w := newWorld(t)
	for role, want := range map[string]int{"maintainer": http.StatusOK, "developer": http.StatusForbidden, "viewer": http.StatusForbidden} {
		user := w.env.SeedUser(w.org, role+"-ingest@example.com", "a long password for "+role, role)
		res := w.env.SeedSession(user).Get(w.tokensPath(""))
		assert.Equal(t, want, res.Status, "%s: %s", role, res.Raw)
	}
	api := w.env.SeedAPIToken(w.ownerID, w.org, "project.read", "project.manage")
	res := w.env.Bearer(api.Value, apptest.Request{Method: http.MethodGet, Path: w.tokensPath("")})
	assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, res.Status, string(res.Raw))
	res = w.env.Bearer(api.Value, apptest.Request{Method: http.MethodPost, Path: w.tokensPath(""), Body: map[string]any{}})
	assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, res.Status, string(res.Raw))
}

func TestTheListOfUploadedFilesShowsWhatTheUploaderDeclared(t *testing.T) {
	w := newWorld(t)
	prov := `{"Provider":"github","Repository":"team/app","RunUrl":"https://ci.example.com/runs/7","Actor":"sara"}`
	res := w.upload(w.archive(), func(u *apptest.IngestUpload) { u.Provenance = prov })
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	list := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/ingested-artifacts"))
	require.Equal(t, http.StatusOK, list.Status, string(list.Raw))
	var items []struct {
		Name, Digest, Ref, Sha, RefKind, TokenName, RunId string
		Provenance                                      map[string]string
		ProvenanceVerified                              bool
	}
	list.Decode(t, &items)
	require.Len(t, items, 1)
	assert.Equal(t, "web", items[0].Name)
	assert.Equal(t, "refs/heads/main", items[0].Ref)
	assert.Equal(t, "sara", items[0].Provenance["Actor"])
	assert.False(t, items[0].ProvenanceVerified, "what the CI says about itself is declared, not verified")
	assert.NotEmpty(t, items[0].RunId)
	assert.NotEmpty(t, items[0].TokenName)
}

func TestNeitherTheTokenNorTheContentReachesALogOrAnAuditEvent(t *testing.T) {
	w := newWorld(t)
	secretContent := "console.log('content-that-must-not-leave-the-blob-store')"
	body := apptest.TarGz(map[string]string{"app.js": secretContent})
	require.Equal(t, http.StatusCreated, w.upload(body, nil).Status)
	require.Equal(t, http.StatusUnprocessableEntity, w.upload(apptest.TarGz(map[string]string{"../x": secretContent}), nil).Status)
	require.Equal(t, http.StatusForbidden, w.upload(body, func(u *apptest.IngestUpload) { u.Ref = "other" }).Status)
	created := w.createToken(map[string]any{"Name": "leaky", "RefPatterns": mainOnly()})
	require.Equal(t, http.StatusCreated, created.Status)
	var made struct{ Value string }
	created.Decode(t, &made)
	require.Equal(t, http.StatusCreated, w.upload(apptest.TarGz(map[string]string{"b.js": "2"}), func(u *apptest.IngestUpload) {
		u.Token = made.Value
		u.NoTrigger = true
	}).Status)

	rows, err := w.env.Pool.Query(context.Background(), `SELECT "Action", "Data"::text, coalesce("TargetId", '') FROM "AuditEvents" ORDER BY "CreatedAt"`)
	require.NoError(t, err)
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var action, data, target string
		require.NoError(t, rows.Scan(&action, &data, &target))
		seen[action] = true
		for _, needle := range []string{w.token.Value, made.Value, secretContent, "content-that-must-not-leave", config.IngestTokenPrefix} {
			assert.NotContains(t, data, needle, "%s", action)
		}
	}
	require.NoError(t, rows.Err())
	for _, action := range []string{"ingest_token.created", "artifact.ingested", "ingest.rejected"} {
		assert.True(t, seen[action], "%s is in the audit log", action)
	}
	logs := w.env.Logs.String()
	for _, needle := range []string{w.token.Value, made.Value, strings.TrimPrefix(made.Value, config.IngestTokenPrefix), "content-that-must-not-leave"} {
		assert.NotContains(t, logs, needle)
	}
}

func TestOneRejectionAuditEventPerMinutePerToken(t *testing.T) {
	w := newWorld(t)
	for range 5 {
		res := w.upload(w.archive(), func(u *apptest.IngestUpload) { u.Ref = "forbidden" })
		require.Equal(t, http.StatusForbidden, res.Status)
	}
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "AuditEvents" WHERE "Action" = 'ingest.rejected'`).Scan(&n))
	assert.Equal(t, 1, n, "a stolen token cannot flood the audit log")
}
