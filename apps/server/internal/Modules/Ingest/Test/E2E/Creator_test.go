package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const creatorPassword = "a long password for the creator"

type creatorView struct {
	Id, Name, Status    string
	CreatorAllowed      bool
	UnacknowledgedReach []string
}

func (w world) tokenBy(t *testing.T, role string) (apptest.IngestToken, uuid.UUID) {
	t.Helper()
	id := w.env.SeedUser(w.org, role+"-"+uuid.NewString()[:8]+"@example.com", creatorPassword, role)
	return w.env.SeedIngestToken(w.project, id, apptest.IngestSeed{}), id
}

func (w world) uploadWith(token apptest.IngestToken) apptest.Response {
	return w.upload(w.archive(), func(u *apptest.IngestUpload) { u.Token = token.Value })
}

func (w world) listView(t *testing.T, id uuid.UUID) creatorView {
	t.Helper()
	res := w.owner.Get(w.tokensPath(""))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var items []creatorView
	res.Decode(t, &items)
	for _, it := range items {
		if it.Id == id.String() {
			return it
		}
	}
	require.Fail(t, "the token is not in the list")
	return creatorView{}
}

func requireRefusal(t *testing.T, res apptest.Response, status int, code string) {
	t.Helper()
	assert.Equal(t, status, res.Status, string(res.Raw))
	require.NotNil(t, res.Error, string(res.Raw))
	assert.Equal(t, code, res.Error.Code)
}

func TestATokenStopsWhenItsCreatorIsRemovedFromTheOrganization(t *testing.T) {
	w := newWorld(t)
	token, creator := w.tokenBy(t, "maintainer")
	require.Equal(t, http.StatusCreated, w.uploadWith(token).Status)
	assert.True(t, w.listView(t, token.ID).CreatorAllowed)

	gone := w.owner.Do(apptest.Request{Method: http.MethodDelete, Path: apptest.OrgPath(w.org, "/members/"+creator.String())})
	require.Equal(t, http.StatusOK, gone.Status, string(gone.Raw))

	requireRefusal(t, w.uploadWith(token), http.StatusUnauthorized, "INGEST_TOKEN_REVOKED")
	view := w.listView(t, token.ID)
	assert.Equal(t, "revoked", view.Status)
	assert.False(t, view.CreatorAllowed)
	assert.Equal(t, 1, w.env.AuditCount("ingest_token.revoked"), "the revocation is in the audit log")
	assert.Equal(t, http.StatusOK, w.upload(w.archive(), nil).Status, "the tokens of other people keep working: the file is a duplicate")
}

func TestATokenStopsWhenItsCreatorLeavesTheOrganization(t *testing.T) {
	w := newWorld(t)
	token, creator := w.tokenBy(t, "maintainer")
	require.Equal(t, http.StatusCreated, w.uploadWith(token).Status)

	left := w.env.SeedSession(creator).Do(apptest.Request{Method: http.MethodPost, Path: apptest.OrgPath(w.org, "/leave")})
	require.Equal(t, http.StatusOK, left.Status, string(left.Raw))

	requireRefusal(t, w.uploadWith(token), http.StatusUnauthorized, "INGEST_TOKEN_REVOKED")
}

func TestATokenWhoseCreatorIsGoneWithoutAnEventStillStops(t *testing.T) {
	w := newWorld(t)
	token, creator := w.tokenBy(t, "maintainer")
	before := w.env.IngestedCount(w.project)
	_, err := w.env.Pool.Exec(context.Background(),
		`UPDATE "Memberships" SET "IsDeleted" = true WHERE "UserId" = $1 AND "OrganizationId" = $2`, creator, w.org)
	require.NoError(t, err)

	requireRefusal(t, w.uploadWith(token), http.StatusForbidden, "INGEST_CREATOR_NOT_ALLOWED")
	view := w.listView(t, token.ID)
	assert.Equal(t, "active", view.Status, "nothing revoked it, the check at use time refused")
	assert.False(t, view.CreatorAllowed)
	assert.Equal(t, before, w.env.IngestedCount(w.project), "nothing was stored by the refused upload")
}

func TestATokenStopsWhenItsCreatorIsDemotedBelowTheRoleThatManagesIngest(t *testing.T) {
	w := newWorld(t)
	token, creator := w.tokenBy(t, "maintainer")
	require.Equal(t, http.StatusCreated, w.uploadWith(token).Status)
	before := w.env.IngestedCount(w.project)

	demoted := w.owner.ChangeRole(w.org, creator.String(), "viewer", nil)
	require.Equal(t, http.StatusOK, demoted.Status, string(demoted.Raw))

	requireRefusal(t, w.uploadWith(token), http.StatusForbidden, "INGEST_CREATOR_NOT_ALLOWED")
	assert.Equal(t, before, w.env.IngestedCount(w.project), "the refused upload stored nothing")
	status := w.env.IngestStatus(token.Value, "0000000000000000000000000000000000000000000000000000000000000000")
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, status.Status)
	view := w.listView(t, token.ID)
	assert.Equal(t, "active", view.Status)
	assert.False(t, view.CreatorAllowed, "the list tells an admin that the creator is no longer allowed")

	promoted := w.owner.ChangeRole(w.org, creator.String(), "maintainer", nil)
	require.Equal(t, http.StatusOK, promoted.Status, string(promoted.Raw))
	assert.Equal(t, http.StatusOK, w.uploadWith(token).Status, "a creator who holds the role again is allowed again: the file is a duplicate")
}

func TestAProjectRoleCountsAndLosingItStopsTheToken(t *testing.T) {
	w := newWorld(t)
	dev := w.env.SeedUser(w.org, "dev-"+uuid.NewString()[:8]+"@example.com", creatorPassword, "developer")
	binding := w.env.SeedUserProjectBinding(w.org, w.project, dev, "maintainer")
	token := w.env.SeedIngestToken(w.project, dev, apptest.IngestSeed{})
	require.Equal(t, http.StatusCreated, w.uploadWith(token).Status, "a project maintainer binding allows ingest.manage")
	assert.True(t, w.listView(t, token.ID).CreatorAllowed)

	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "RoleBindings" SET "IsDeleted" = true WHERE "Id" = $1`, binding)
	require.NoError(t, err)

	requireRefusal(t, w.uploadWith(token), http.StatusForbidden, "INGEST_CREATOR_NOT_ALLOWED")
	assert.False(t, w.listView(t, token.ID).CreatorAllowed)
}

func TestARefusalForTheCreatorHasBothMessages(t *testing.T) {
	w := newWorld(t)
	token, creator := w.tokenBy(t, "maintainer")
	_, err := w.env.Pool.Exec(context.Background(),
		`UPDATE "Memberships" SET "IsDeleted" = true WHERE "UserId" = $1 AND "OrganizationId" = $2`, creator, w.org)
	require.NoError(t, err)

	en := w.env.IngestPut(apptest.IngestUpload{
		Token: token.Value, Body: w.archive(), Name: "web", Ref: "main", Sha: shaNext, Headers: map[string]string{"Accept-Language": "en"},
	})
	ar := w.env.IngestPut(apptest.IngestUpload{
		Token: token.Value, Body: w.archive(), Name: "web", Ref: "main", Sha: shaNext, Headers: map[string]string{"Accept-Language": "ar"},
	})
	require.NotNil(t, en.Error)
	require.NotNil(t, ar.Error)
	assert.Contains(t, en.Error.Message, "no longer a member")
	assert.NotEqual(t, en.Error.Message, ar.Error.Message)
	assert.NotEqual(t, "errors.ingestCreatorNotAllowed", ar.Error.Message, "the key is translated")
}
