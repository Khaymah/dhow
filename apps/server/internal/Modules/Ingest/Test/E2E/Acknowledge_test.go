package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func (w world) acknowledge(c *apptest.Client, id string, environments []string, factor map[string]any) apptest.Response {
	return c.Do(apptest.Request{
		Method: http.MethodPost, Path: w.tokensPath("/" + id + "/acknowledge-reach"),
		Body: map[string]any{"CurrentFactor": factor, "Environments": environments},
	})
}

func (w world) adminClient(t *testing.T, role string) (*apptest.Client, map[string]any) {
	t.Helper()
	password := "a long password for " + role
	user := w.env.SeedUser(w.org, role+"-ack@example.com", password, role)
	return w.env.SeedSession(user), apptest.PasswordFactor(password)
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
	assert.NotContains(t, string(refused.Raw), "production", "the refusal does not name the environments")
	assert.Equal(t, before+1, w.runsOfKind(t, "artifact"), "no run started into the protected environment")
	assert.Equal(t, 1, w.env.IngestedCount(w.project), "the refused upload stored nothing")

	view := w.listView(t, w.token.ID)
	assert.Equal(t, []string{"production"}, view.UnacknowledgedReach)
	assert.True(t, view.CreatorAllowed)
	assert.Equal(t, "active", view.Status)

	wrong := w.acknowledge(w.owner, w.token.ID.String(), []string{"production"}, apptest.PasswordFactor("not the password"))
	assert.GreaterOrEqual(t, wrong.Status, http.StatusBadRequest, string(wrong.Raw))
	assert.Equal(t, []string{"production"}, w.listView(t, w.token.ID).UnacknowledgedReach, "a wrong factor confirms nothing")

	ok := w.acknowledge(w.owner, w.token.ID.String(), []string{"production"}, apptest.PasswordFactor(w.password))
	require.Equal(t, http.StatusOK, ok.Status, string(ok.Raw))
	assert.Equal(t, "ingest.token.reachAcknowledged", ok.Message)
	assert.Empty(t, w.listView(t, w.token.ID).UnacknowledgedReach)
	assert.Equal(t, 1, w.env.AuditCount("ingest_token.reach_acknowledged"))

	allowed := w.upload(other, func(u *apptest.IngestUpload) { u.Sha = shaTag })
	assert.Equal(t, http.StatusCreated, allowed.Status, string(allowed.Raw))
}

func TestAConfirmationOfADifferentSetThanTheServerComputesIsRefused(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	for name, shown := range map[string][]string{"nothing": {}, "another name": {"staging"}, "too many": {"production", "staging"}} {
		res := w.acknowledge(w.owner, w.token.ID.String(), shown, apptest.PasswordFactor(w.password))
		requireRefusal(t, res, http.StatusConflict, "INGEST_REACH_CHANGED")
		assert.Equal(t, []string{"production"}, w.listView(t, w.token.ID).UnacknowledgedReach, name)
	}
}

func TestAnEnvironmentThatBecomesProtectedLaterIsNotCoveredByAnOlderConfirmation(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	require.Equal(t, http.StatusOK, w.acknowledge(w.owner, w.token.ID.String(), []string{"production"}, apptest.PasswordFactor(w.password)).Status)
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
	assert.Equal(t, []string{"staging"}, w.listView(t, w.token.ID).UnacknowledgedReach, "only the new environment is flagged")
}

func TestARecreatedEnvironmentWithTheSameNameDoesNotInheritTheConfirmation(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	require.Equal(t, http.StatusOK, w.acknowledge(w.owner, w.token.ID.String(), []string{"production"}, apptest.PasswordFactor(w.password)).Status)
	assert.Empty(t, w.listView(t, w.token.ID).UnacknowledgedReach)

	for _, q := range []string{
		`UPDATE "DeployTargetHosts" SET "IsDeleted" = true WHERE "TargetId" IN (SELECT "Id" FROM "DeployTargets" WHERE "ProjectId" = $1)`,
		`UPDATE "DeployTargets" SET "IsDeleted" = true WHERE "ProjectId" = $1`,
		`UPDATE "Environments" SET "IsDeleted" = true WHERE "ProjectId" = $1 AND "Name" = 'production'`,
	} {
		_, err := w.env.Pool.Exec(context.Background(), q, w.project)
		require.NoError(t, err)
	}
	assert.Empty(t, w.listView(t, w.token.ID).UnacknowledgedReach, "with the environment gone nothing is reached")
	w.protectedProduction(t)

	assert.Equal(t, []string{"production"}, w.listView(t, w.token.ID).UnacknowledgedReach, "the new environment is not the one that was confirmed")
	other := apptest.TarGz(map[string]string{"index.html": "<h1>again</h1>"})
	refused := w.upload(other, func(u *apptest.IngestUpload) { u.Sha = shaTag })
	requireRefusal(t, refused, http.StatusForbidden, "INGEST_REACH_NOT_ACKNOWLEDGED")
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

func TestOnlyWhoManageEnvironmentsConfirmAProtectedReach(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	shown := []string{"production"}
	wide := map[string]any{
		"Name": "wide", "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": "**"}}, "AcknowledgeProtectedReach": true,
	}
	for _, role := range []string{"maintainer", "developer", "viewer"} {
		c, factor := w.adminClient(t, role)
		res := w.acknowledge(c, w.token.ID.String(), shown, factor)
		if role == "maintainer" {
			requireRefusal(t, res, http.StatusForbidden, "INGEST_REACH_NEEDS_ENVIRONMENT_ADMIN")
		} else {
			requireRefusal(t, res, http.StatusForbidden, "FORBIDDEN")
		}
		wide["Name"] = "wide-" + role
		wide["CurrentFactor"] = factor
		create := c.Do(apptest.Request{Method: http.MethodPost, Path: w.tokensPath(""), Body: wide})
		if role == "maintainer" {
			requireRefusal(t, create, http.StatusForbidden, "INGEST_REACH_NEEDS_ENVIRONMENT_ADMIN")
		}
		assert.Equal(t, shown, w.listView(t, w.token.ID).UnacknowledgedReach, role)
	}
	assert.Equal(t, 1, w.countTokens(t), "the maintainer's refused request made no token")

	api := w.env.SeedAPIToken(w.ownerID, w.org, "project.read", "project.manage")
	res := w.env.Bearer(api.Value, apptest.Request{
		Method: http.MethodPost, Path: w.tokensPath("/" + w.token.ID.String() + "/acknowledge-reach"),
		Body: map[string]any{"CurrentFactor": apptest.PasswordFactor(w.password), "Environments": shown},
	})
	assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, res.Status, string(res.Raw))

	revoked := w.env.SeedIngestToken(w.project, w.ownerID, apptest.IngestSeed{Revoked: true})
	conflict := w.acknowledge(w.owner, revoked.ID.String(), shown, apptest.PasswordFactor(w.password))
	assert.Equal(t, http.StatusConflict, conflict.Status, string(conflict.Raw))
}

func TestAMaintainerStillMakesATokenThatReachesOnlyUnprotectedEnvironments(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	c, factor := w.adminClient(t, "maintainer")
	res := c.Do(apptest.Request{Method: http.MethodPost, Path: w.tokensPath(""), Body: map[string]any{
		"Name": "docs", "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": "docs/*"}}, "CurrentFactor": factor,
	}})
	assert.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
}

func TestAnAdminAndAProjectAdminBindingConfirmAProtectedReach(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	shown := []string{"production"}
	admin, adminFactor := w.adminClient(t, "admin")
	assert.Equal(t, http.StatusOK, w.acknowledge(admin, w.token.ID.String(), shown, adminFactor).Status)

	second := w.env.SeedIngestToken(w.project, w.ownerID, apptest.IngestSeed{})
	viewerPassword := "a long password for the bound viewer"
	viewer := w.env.SeedUser(w.org, "bound-viewer@example.com", viewerPassword, "viewer")
	w.env.SeedUserProjectBinding(w.org, w.project, viewer, "admin")
	bound := w.acknowledge(w.env.SeedSession(viewer), second.ID.String(), shown, apptest.PasswordFactor(viewerPassword))
	assert.Equal(t, http.StatusOK, bound.Status, "a project admin binding is enough: %s", bound.Raw)
}

func TestACreatorDemotedDuringASlowUploadRefusesTheCommit(t *testing.T) {
	w := newWorld(t)
	token, creator := w.tokenBy(t, "maintainer")
	before, runs := w.env.IngestedCount(w.project), w.runsOfKind(t, "artifact")

	res := w.env.IngestPut(apptest.IngestUpload{
		Token: token.Value, Body: w.archive(), Name: "web", Ref: "main", Sha: shaNext,
		AfterBody: func() {
			demoted := w.owner.ChangeRole(w.org, creator.String(), "viewer", nil)
			require.Equal(t, http.StatusOK, demoted.Status, string(demoted.Raw))
		},
	})

	requireRefusal(t, res, http.StatusForbidden, "INGEST_CREATOR_NOT_ALLOWED")
	assert.Equal(t, before, w.env.IngestedCount(w.project), "nothing was stored")
	assert.Equal(t, runs, w.runsOfKind(t, "artifact"), "no run started")
}

func TestAnEnvironmentProtectedDuringASlowUploadRefusesTheCommit(t *testing.T) {
	w := newWorld(t)
	before, runs := w.env.IngestedCount(w.project), w.runsOfKind(t, "artifact")

	res := w.upload(w.archive(), func(u *apptest.IngestUpload) {
		u.AfterBody = func() { w.protectedProduction(t) }
	})

	requireRefusal(t, res, http.StatusForbidden, "INGEST_REACH_NOT_ACKNOWLEDGED")
	assert.Equal(t, before, w.env.IngestedCount(w.project), "nothing was stored")
	assert.Equal(t, runs, w.runsOfKind(t, "artifact"), "no run started")
}
