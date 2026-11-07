package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func (w world) acknowledge(c *apptest.Client, id string, factor map[string]any) apptest.Response {
	return c.Do(apptest.Request{
		Method: http.MethodPost, Path: w.tokensPath("/" + id + "/acknowledge-reach"), Body: map[string]any{"CurrentFactor": factor},
	})
}

func (w world) reached(t *testing.T, res apptest.Response) []string {
	t.Helper()
	var details []string
	require.NotNil(t, res.Error, string(res.Raw))
	for _, d := range res.Error.Details {
		if d.Field == "ProtectedReach" {
			details = append(details, d.Message)
		}
	}
	return details
}

func TestATokenThatLaterReachesAProtectedEnvironmentCannotUploadUntilConfirmed(t *testing.T) {
	w := newWorld(t)
	before := w.runsOfKind(t, "artifact")
	require.Equal(t, http.StatusCreated, w.upload(w.archive(), nil).Status, "no protected environment yet")
	assert.Equal(t, before+1, w.runsOfKind(t, "artifact"))

	w.protectedProduction(t)

	other := apptest.TarGz(map[string]string{"index.html": "<h1>second</h1>"})
	refused := w.upload(other, func(u *apptest.IngestUpload) { u.Sha = shaTag })
	requireRefusal(t, refused, http.StatusForbidden, "INGEST_REACH_NOT_ACKNOWLEDGED")
	assert.Equal(t, []string{"production"}, w.reached(t, refused))
	assert.Equal(t, before+1, w.runsOfKind(t, "artifact"), "no run started into the protected environment")
	assert.Equal(t, 1, w.env.IngestedCount(w.project), "the refused upload stored nothing")

	view := w.listView(t, w.token.ID)
	assert.Equal(t, []string{"production"}, view.UnacknowledgedReach)
	assert.True(t, view.CreatorAllowed)
	assert.Equal(t, "active", view.Status)

	wrong := w.acknowledge(w.owner, w.token.ID.String(), apptest.PasswordFactor("not the password"))
	assert.GreaterOrEqual(t, wrong.Status, http.StatusBadRequest, string(wrong.Raw))
	assert.Equal(t, []string{"production"}, w.listView(t, w.token.ID).UnacknowledgedReach, "a wrong factor confirms nothing")

	ok := w.acknowledge(w.owner, w.token.ID.String(), apptest.PasswordFactor(w.password))
	require.Equal(t, http.StatusOK, ok.Status, string(ok.Raw))
	assert.Equal(t, "ingest.token.reachAcknowledged", ok.Message)
	assert.Empty(t, w.listView(t, w.token.ID).UnacknowledgedReach)
	assert.Equal(t, 1, w.env.AuditCount("ingest_token.reach_acknowledged"))

	allowed := w.upload(other, func(u *apptest.IngestUpload) { u.Sha = shaTag })
	assert.Equal(t, http.StatusCreated, allowed.Status, string(allowed.Raw))
}

func TestAnEnvironmentThatBecomesProtectedLaterIsNotCoveredByAnOlderConfirmation(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	require.Equal(t, http.StatusOK, w.acknowledge(w.owner, w.token.ID.String(), apptest.PasswordFactor(w.password)).Status)
	assert.Equal(t, http.StatusCreated, w.upload(w.archive(), nil).Status)

	staging := w.env.SeedEnvironment(w.project, "staging")
	_, err := w.env.Pool.Exec(context.Background(),
		`INSERT INTO "DeployTargets" ("OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", "DeployPath", "CreatedByUserId")
		 SELECT p."OrganizationId", p."Id", $2, 'stage', 'ssh', 'deploy', '/srv/stage', p."CreatedByUserId" FROM "Projects" p WHERE p."Id" = $1`,
		w.project, staging)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(context.Background(),
		`UPDATE "Environments" SET "AllowedRefs" = '[{"Kind":"branch","Pattern":"main"}]' WHERE "Id" = $1`, staging)
	require.NoError(t, err)

	other := apptest.TarGz(map[string]string{"index.html": "<h1>third</h1>"})
	refused := w.upload(other, func(u *apptest.IngestUpload) { u.Sha = shaTag })
	requireRefusal(t, refused, http.StatusForbidden, "INGEST_REACH_NOT_ACKNOWLEDGED")
	assert.Equal(t, []string{"staging"}, w.reached(t, refused), "only the new environment is named")
	assert.Equal(t, []string{"staging"}, w.listView(t, w.token.ID).UnacknowledgedReach)
}

func TestATokenMadeWithTheYesIsAcknowledgedFromTheStart(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	res := w.createToken(map[string]any{
		"Name": "wide", "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": "**"}}, "AcknowledgeProtectedReach": true,
	})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var created struct {
		Token struct {
			Id                  string
			UnacknowledgedReach []string
		}
		Value string
	}
	res.Decode(t, &created)
	assert.Empty(t, created.Token.UnacknowledgedReach)

	up := w.upload(w.archive(), func(u *apptest.IngestUpload) { u.Token = created.Value })
	assert.Equal(t, http.StatusCreated, up.Status, string(up.Raw))
}

func TestOnlyWhoManageIngestConfirmsAndNotARevokedToken(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	for role, want := range map[string]int{"developer": http.StatusForbidden, "viewer": http.StatusForbidden} {
		user := w.env.SeedUser(w.org, role+"-ack@example.com", "a long password for "+role, role)
		res := w.acknowledge(w.env.SeedSession(user), w.token.ID.String(), apptest.PasswordFactor("a long password for "+role))
		assert.Equal(t, want, res.Status, "%s: %s", role, res.Raw)
	}
	api := w.env.SeedAPIToken(w.ownerID, w.org, "project.read", "project.manage")
	res := w.env.Bearer(api.Value, apptest.Request{
		Method: http.MethodPost, Path: w.tokensPath("/" + w.token.ID.String() + "/acknowledge-reach"),
		Body: map[string]any{"CurrentFactor": apptest.PasswordFactor(w.password)},
	})
	assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, res.Status, string(res.Raw))
	assert.Equal(t, []string{"production"}, w.listView(t, w.token.ID).UnacknowledgedReach)

	revoked := w.env.SeedIngestToken(w.project, w.ownerID, apptest.IngestSeed{Revoked: true})
	conflict := w.acknowledge(w.owner, revoked.ID.String(), apptest.PasswordFactor(w.password))
	assert.Equal(t, http.StatusConflict, conflict.Status, string(conflict.Raw))
}
