package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const maxAuditValue = 256

func assertBounded(t *testing.T, data map[string]string) {
	t.Helper()
	for k, v := range data {
		assert.LessOrEqual(t, len(v), maxAuditValue, "%s would be cut by the audit sanitizer", k)
		assert.True(t, utf8.ValidString(v), "%s", k)
		if strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") {
			assert.True(t, json.Valid([]byte(v)), "%s is broken JSON: %s", k, v)
		}
	}
}

func TestAHiddenWideningOfTheRefsShowsInTheAudit(t *testing.T) {
	w := newWorld(t)
	long := "release/" + strings.Repeat("a", 240)
	e := decode(t, w.create(w.owner, map[string]any{
		"Name": "production", "Rules": map[string]any{"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": long}}},
	}), http.StatusCreated)
	decode(t, w.update(w.owner, "production", map[string]any{
		"Version": e.Version, "CurrentFactor": w.ownerFactor(),
		"Rules": map[string]any{"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": long}, {"Kind": "branch", "Pattern": "**"}}},
	}), http.StatusOK)

	updated := auditOf(t, w, "environment.updated")
	require.Len(t, updated, 1)
	data := updated[0].Data
	assertBounded(t, data)
	assert.Equal(t, "1 -> 2", data["AllowedRefsCount"])
	from, to, ok := strings.Cut(data["AllowedRefsDigest"], " -> ")
	require.True(t, ok, data["AllowedRefsDigest"])
	assert.Len(t, from, 64)
	assert.NotEqual(t, from, to, "the digest shows the change")
	assert.Equal(t, "true", data["Truncated"], "the values do not fit, and the event says so")
	assert.NotContains(t, data, "AllowedRefsTo")

	created := auditOf(t, w, "environment.created")
	require.Len(t, created, 1)
	assert.Equal(t, from, created[0].Data["AllowedRefsDigest"], "the create event carries the digest the update starts from")
}

func TestCreateAuditsTheInitialRulesAndNeedsAFactor(t *testing.T) {
	w := newWorld(t)
	body := map[string]any{"Name": "production", "Rules": map[string]any{"AllowSelfApproval": true, "AllowAdminBypass": true}}
	requireError(t, w.createRaw(w.owner, body), http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	body["CurrentFactor"] = wrongFactor()
	requireError(t, w.createRaw(w.owner, body), http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	requireError(t, w.get(w.owner, "production"), http.StatusNotFound, "NOT_FOUND")

	delete(body, "CurrentFactor")
	decode(t, w.create(w.owner, body), http.StatusCreated)
	created := auditOf(t, w, "environment.created")
	require.Len(t, created, 1)
	data := created[0].Data
	assertBounded(t, data)
	assert.Equal(t, "true", data["AllowSelfApproval"])
	assert.Equal(t, "true", data["AllowAdminBypass"])
	assert.Equal(t, "1", data["RequiredApprovals"])
	assert.Equal(t, "72", data["ApprovalTimeoutHours"])
	assert.Equal(t, "1", data["AllowedRefsCount"])
	assert.JSONEq(t, `[{"Kind":"branch","Pattern":"main"}]`, data["AllowedRefsTo"])
	assert.Equal(t, "0", data["ReviewersCount"])
	assert.Len(t, data["AllowedRefsDigest"], 64)
}

func TestReviewerChangesAreAuditedAsAddedAndRemoved(t *testing.T) {
	w := newWorld(t)
	_, one := w.member(t, "developer")
	_, two := w.member(t, "developer")
	e := decode(t, w.create(w.owner, map[string]any{
		"Name": "production", "Rules": map[string]any{"Reviewers": map[string]any{"UserIds": []string{one.String()}, "TeamIds": []string{}}},
	}), http.StatusCreated)
	decode(t, w.update(w.owner, "production", map[string]any{
		"Version": e.Version, "CurrentFactor": w.ownerFactor(),
		"Rules": map[string]any{"Reviewers": map[string]any{"UserIds": []string{two.String()}, "TeamIds": []string{}}},
	}), http.StatusOK)
	data := auditOf(t, w, "environment.updated")[0].Data
	assertBounded(t, data)
	assert.Equal(t, "user:"+two.String(), data["ReviewersAdded"])
	assert.Equal(t, "user:"+one.String(), data["ReviewersRemoved"])
	assert.Equal(t, "1 -> 1", data["ReviewersCount"])
	assert.Contains(t, data["ReviewersDigest"], " -> ")
}

func TestALongPauseReasonIsCutWithADigest(t *testing.T) {
	w := newWorld(t)
	w.production(t)
	reason := strings.Repeat("ع", 400)
	decode(t, w.post(w.owner, "production", "pause", map[string]string{"Reason": reason}), http.StatusOK)
	data := auditOf(t, w, "environment.paused")[0].Data
	assertBounded(t, data)
	assert.Equal(t, "true", data["Truncated"])
	assert.Len(t, data["ReasonDigest"], 64)
	assert.True(t, strings.HasPrefix(reason, data["Reason"]))
}

func TestAFailedAuditRollsTheChangeBack(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	ctx := context.Background()
	_, err := w.env.Pool.Exec(ctx, `
		CREATE FUNCTION "TestRefuseEnvironmentAudit"() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW."Action" LIKE 'environment.%' THEN RAISE EXCEPTION 'audit refused by the test'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER "TestRefuseEnvironmentAudit" BEFORE INSERT ON "AuditEvents"
			FOR EACH ROW EXECUTE FUNCTION "TestRefuseEnvironmentAudit"();`)
	require.NoError(t, err)

	res := w.update(w.owner, "production", map[string]any{"Version": e.Version, "Rules": map[string]any{"WaitMinutes": 9}, "CurrentFactor": w.ownerFactor()})
	assert.Equal(t, http.StatusInternalServerError, res.Status, string(res.Raw))
	res = w.post(w.owner, "production", "pause", map[string]string{"Reason": "x"})
	assert.Equal(t, http.StatusInternalServerError, res.Status, string(res.Raw))
	res = w.create(w.owner, map[string]any{"Name": "staging"})
	assert.Equal(t, http.StatusInternalServerError, res.Status, string(res.Raw))
	res = w.post(w.owner, "production", "remove", map[string]any{"CurrentFactor": w.ownerFactor()})
	assert.Equal(t, http.StatusInternalServerError, res.Status, string(res.Raw))

	_, err = w.env.Pool.Exec(ctx, `DROP TRIGGER "TestRefuseEnvironmentAudit" ON "AuditEvents"`)
	require.NoError(t, err)
	assert.Equal(t, e, decode(t, w.get(w.owner, "production"), http.StatusOK), "nothing changed without its audit event")
	requireError(t, w.get(w.owner, "staging"), http.StatusNotFound, "NOT_FOUND")
}

func TestServerRefusesInvisibleCharacters(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	for _, tt := range []struct {
		name string
		res  apptest.Response
	}{
		{"a name with a bidi override", w.create(w.owner, map[string]any{"Name": "prod‮"})},
		{"a pattern with a bidi override", w.create(w.owner, map[string]any{"Name": "p1", "Rules": map[string]any{
			"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": "ma‮in"}},
		}})},
		{"a pattern with a zero-width space", w.create(w.owner, map[string]any{"Name": "p2", "Rules": map[string]any{
			"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": "ma​in"}},
		}})},
		{"a description with a bidi isolate", w.update(w.owner, "production", map[string]any{
			"Version": e.Version, "Description": "live ⁦site⁩", "CurrentFactor": w.ownerFactor(),
		})},
		{"a reason with a zero-width joiner", w.post(w.owner, "production", "pause", map[string]string{"Reason": "bad‍release"})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requireError(t, tt.res, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
		})
	}
	assert.Equal(t, e, decode(t, w.get(w.owner, "production"), http.StatusOK))
}

func TestFullRefNamesAreRefused(t *testing.T) {
	w := newWorld(t)
	for _, p := range []string{"refs/heads/main", "heads/main", "tags/v1"} {
		res := w.create(w.owner, map[string]any{"Name": "production", "Rules": map[string]any{
			"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": p}},
		}})
		requireError(t, res, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
		assert.Contains(t, detailFields(res), "Rules.AllowedRefs[0].Pattern", p)
	}
}

func TestOnlyManagersSeeReviewerIds(t *testing.T) {
	w := newWorld(t)
	_, dev := w.member(t, "developer")
	decode(t, w.create(w.owner, map[string]any{
		"Name": "production", "Rules": map[string]any{"Reviewers": map[string]any{"UserIds": []string{dev.String()}, "TeamIds": []string{}}},
	}), http.StatusCreated)
	admin, _ := w.member(t, "admin")
	shown := decode(t, w.get(admin, "production"), http.StatusOK)
	assert.True(t, shown.Rules.Reviewers.IdsShown)
	assert.Equal(t, []string{dev.String()}, shown.Rules.Reviewers.UserIds)
	assert.Equal(t, 1, shown.Rules.Reviewers.UserCount)

	for _, role := range []string{"maintainer", "developer", "viewer"} {
		c, _ := w.member(t, role)
		hidden := decode(t, w.get(c, "production"), http.StatusOK)
		assert.False(t, hidden.Rules.Reviewers.IdsShown, role)
		assert.Empty(t, hidden.Rules.Reviewers.UserIds, role)
		assert.Equal(t, 1, hidden.Rules.Reviewers.UserCount, role)
	}
	token := w.env.SeedAPIToken(w.ownerID, w.org, "project.read")
	res := w.env.Bearer(token.Value, apptest.Request{Method: http.MethodGet, Path: w.path(w.project, ""), Headers: map[string]string{"Origin": ""}})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var list []environmentView
	res.Decode(t, &list)
	require.Len(t, list, 1)
	assert.Empty(t, list[0].Rules.Reviewers.UserIds, "a token of the owner still sees counts only")
	assert.Equal(t, 1, list[0].Rules.Reviewers.UserCount)
}
