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

const memberPassword = "a long password for a project member"

type world struct {
	env     *apptest.Env
	owner   *apptest.Client
	ownerID uuid.UUID
	org     uuid.UUID
}

type projectView struct {
	Id, OrganizationId, Slug, Name, PipelinePath, ForkPolicy, ForkCacheRestore, State string
	ArchivedAt                                                                        *string
}

func newWorld(t *testing.T) world {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	return world{env: env, owner: owner, ownerID: uuid.MustParse(owner.UserID), org: env.OrganizationID(d.OrgSlug)}
}

func (w world) member(t *testing.T, role string) (*apptest.Client, uuid.UUID) {
	t.Helper()
	id := w.env.SeedUser(w.org, uuid.NewString()[:8]+"@example.com", memberPassword, role)
	return w.env.SeedSession(id), id
}

func (w world) path(rest string) string { return apptest.OrgPath(w.org, "/projects"+rest) }

func create(c *apptest.Client, w world, body map[string]any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPost, Path: w.path(""), Body: body})
}

func decode(t *testing.T, res apptest.Response, status int) projectView {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	var p projectView
	res.Decode(t, &p)
	return p
}

func requireError(t *testing.T, res apptest.Response, status int, code string) {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	require.NotNil(t, res.Error, string(res.Raw))
	assert.Equal(t, code, res.Error.Code)
}

func TestCreateAProjectWithDefaultsAndAUniqueSlug(t *testing.T) {
	w := newWorld(t)
	p := decode(t, create(w.owner, w, map[string]any{"Name": "Web shop", "Slug": "web-shop"}), http.StatusCreated)
	assert.Equal(t, "web-shop", p.Slug)
	assert.Equal(t, ".dhow.yml", p.PipelinePath)
	assert.Equal(t, "approval", p.ForkPolicy)
	assert.Equal(t, "active", p.State)
	assert.Equal(t, w.org.String(), p.OrganizationId)

	res := create(w.owner, w, map[string]any{"Name": "Again", "Slug": "web-shop"})
	requireError(t, res, http.StatusConflict, "CONFLICT")
	require.Len(t, res.Error.Details, 1)
	assert.Equal(t, "Slug", res.Error.Details[0].Field)

	other := w.env.SeedOrganization("Other", "other")
	outsider := w.env.SeedSession(w.env.SeedUser(other, "o@example.com", memberPassword, "owner"))
	decode(t, outsider.Do(apptest.Request{
		Method: http.MethodPost, Path: apptest.OrgPath(other, "/projects"),
		Body: map[string]any{"Name": "Web shop", "Slug": "web-shop"},
	}), http.StatusCreated)

	var events int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "AuditEvents" WHERE "Action" = 'project.created' AND "TargetId" = $1`, p.Id).Scan(&events))
	assert.Equal(t, 1, events)
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	w := newWorld(t)
	tests := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"an upper-case slug", map[string]any{"Name": "A", "Slug": "Web"}, "Slug"},
		{"a slug with a double hyphen", map[string]any{"Name": "A", "Slug": "a--b"}, "Slug"},
		{"a name with spaces around it", map[string]any{"Name": " A ", "Slug": "a"}, "Name"},
		{"a pipeline path that climbs out", map[string]any{"Name": "A", "Slug": "a", "PipelinePath": "../.dhow.yml"}, "PipelinePath"},
		{"an absolute pipeline path", map[string]any{"Name": "A", "Slug": "a", "PipelinePath": "/etc/dhow.yml"}, "PipelinePath"},
		{"a pipeline path that is not YAML", map[string]any{"Name": "A", "Slug": "a", "PipelinePath": "ci/build.sh"}, "PipelinePath"},
		{"an unknown fork policy", map[string]any{"Name": "A", "Slug": "a", "ForkPolicy": "always"}, "ForkPolicy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := create(w.owner, w, tt.body)
			requireError(t, res, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
			require.NotEmpty(t, res.Error.Details)
			assert.Equal(t, tt.field, res.Error.Details[0].Field)
		})
	}
}

func TestAViewerCannotCreateAProject(t *testing.T) {
	w := newWorld(t)
	for _, role := range []string{"viewer", "developer"} {
		c, _ := w.member(t, role)
		requireError(t, create(c, w, map[string]any{"Name": "Nope", "Slug": "nope-" + role}), http.StatusForbidden, "FORBIDDEN")
	}
	c, _ := w.member(t, "maintainer")
	decode(t, create(c, w, map[string]any{"Name": "Yes", "Slug": "yes"}), http.StatusCreated)
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Projects" WHERE "Slug" LIKE 'nope-%'`).Scan(&n))
	assert.Zero(t, n)
}

func TestADeveloperWithAMaintainerBindingEditsOnlyThatProject(t *testing.T) {
	w := newWorld(t)
	bound, other := w.env.SeedProject(w.org, w.ownerID), w.env.SeedProject(w.org, w.ownerID)
	c, dev := w.member(t, "developer")
	w.env.SeedUserProjectBinding(w.org, bound, dev, "maintainer")

	patch := func(project uuid.UUID, body map[string]any) apptest.Response {
		return c.Do(apptest.Request{Method: http.MethodPatch, Path: w.path("/" + project.String()), Body: body})
	}
	p := decode(t, patch(bound, map[string]any{"Name": "Renamed by the developer", "PipelinePath": "ci/dhow.yaml"}), http.StatusOK)
	assert.Equal(t, "Renamed by the developer", p.Name)
	assert.Equal(t, "ci/dhow.yaml", p.PipelinePath)
	requireError(t, patch(other, map[string]any{"Name": "Not mine"}), http.StatusForbidden, "FORBIDDEN")
	requireError(t, c.Do(apptest.Request{Method: http.MethodPost, Path: w.path("/" + other.String() + "/archive")}), http.StatusForbidden, "FORBIDDEN")
	requireError(t, create(c, w, map[string]any{"Name": "New", "Slug": "new"}), http.StatusForbidden, "FORBIDDEN")
	requireError(t, patch(bound, map[string]any{"ForkPolicy": "automatic"}), http.StatusForbidden, "FORK_POLICY_NOT_ALLOWED")

	var name string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Name" FROM "Projects" WHERE "Id" = $1`, other).Scan(&name))
	assert.NotEqual(t, "Not mine", name)
}

func TestOnlyAnAdminRunsForkPullRequestsWithoutApproval(t *testing.T) {
	w := newWorld(t)
	maintainer, _ := w.member(t, "maintainer")
	requireError(t, create(maintainer, w, map[string]any{"Name": "A", "Slug": "a", "ForkPolicy": "automatic"}),
		http.StatusForbidden, "FORK_POLICY_NOT_ALLOWED")
	p := decode(t, create(maintainer, w, map[string]any{"Name": "A", "Slug": "a"}), http.StatusCreated)
	requireError(t, maintainer.Do(apptest.Request{Method: http.MethodPatch, Path: w.path("/" + p.Id), Body: map[string]any{"ForkPolicy": "automatic"}}),
		http.StatusForbidden, "FORK_POLICY_NOT_ALLOWED")

	admin, _ := w.member(t, "admin")
	p = decode(t, admin.Do(apptest.Request{Method: http.MethodPatch, Path: w.path("/" + p.Id), Body: map[string]any{"ForkPolicy": "automatic"}}), http.StatusOK)
	assert.Equal(t, "automatic", p.ForkPolicy)
	p = decode(t, maintainer.Do(apptest.Request{Method: http.MethodPatch, Path: w.path("/" + p.Id), Body: map[string]any{"ForkPolicy": "approval"}}), http.StatusOK)
	assert.Equal(t, "approval", p.ForkPolicy, "anyone who manages the project may make it stricter")
}

func TestArchiveIsIdempotentAndFreezesTheProject(t *testing.T) {
	w := newWorld(t)
	p := decode(t, create(w.owner, w, map[string]any{"Name": "Old", "Slug": "old"}), http.StatusCreated)
	archive := apptest.Request{Method: http.MethodPost, Path: w.path("/" + p.Id + "/archive")}
	first := decode(t, w.owner.Do(archive), http.StatusOK)
	assert.Equal(t, "archived", first.State)
	require.NotNil(t, first.ArchivedAt)
	second := decode(t, w.owner.Do(archive), http.StatusOK)
	assert.Equal(t, *first.ArchivedAt, *second.ArchivedAt)
	requireError(t, w.owner.Do(apptest.Request{Method: http.MethodPatch, Path: w.path("/" + p.Id), Body: map[string]any{"Name": "New"}}),
		http.StatusConflict, "PROJECT_ARCHIVED")
	requireError(t, create(w.owner, w, map[string]any{"Name": "Old again", "Slug": "old"}), http.StatusConflict, "CONFLICT")

	var events int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "AuditEvents" WHERE "Action" = 'project.archived' AND "TargetId" = $1`, p.Id).Scan(&events))
	assert.Equal(t, 1, events)
}

func TestListPagesAndFiltersByState(t *testing.T) {
	w := newWorld(t)
	for _, slug := range []string{"charlie", "alpha", "bravo"} {
		decode(t, create(w.owner, w, map[string]any{"Name": slug, "Slug": slug}), http.StatusCreated)
	}
	var bravo string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Id"::text FROM "Projects" WHERE "Slug" = 'bravo'`).Scan(&bravo))
	decode(t, w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.path("/" + bravo + "/archive")}), http.StatusOK)

	viewer, _ := w.member(t, "viewer")
	res := viewer.Get(w.path("?Limit=2"))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var page []projectView
	res.Decode(t, &page)
	require.Len(t, page, 2)
	assert.Equal(t, []string{"alpha", "bravo"}, []string{page[0].Slug, page[1].Slug})
	assert.Contains(t, string(res.Raw), `"Total":3`)

	res = viewer.Get(w.path("?State=active"))
	page = nil
	res.Decode(t, &page)
	require.Len(t, page, 2)
	assert.Equal(t, []string{"alpha", "charlie"}, []string{page[0].Slug, page[1].Slug})

	requireError(t, viewer.Get(w.path("?State=deleted")), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	decode(t, viewer.Get(w.path("/"+bravo)), http.StatusOK)
	requireError(t, viewer.Get(w.path("/"+uuid.NewString())), http.StatusNotFound, "NOT_FOUND")
}

func TestAPITokensRespectTheProjectScopes(t *testing.T) {
	w := newWorld(t)
	project := w.env.SeedProject(w.org, w.ownerID)
	developer, devID := w.member(t, "developer")
	w.env.SeedUserProjectBinding(w.org, project, devID, "maintainer")
	res := developer.CreateAPIToken(w.org, "CI", []string{"project.manage"}, 30, apptest.PasswordFactor(memberPassword))
	requireError(t, res, http.StatusForbidden, "SCOPE_NOT_ALLOWED")

	maintainer, _ := w.member(t, "maintainer")
	reader := maintainer.MustCreateAPIToken(w.org, []string{"project.read"}, memberPassword)
	writer := maintainer.MustCreateAPIToken(w.org, []string{"project.read", "project.manage"}, memberPassword)
	get := apptest.Request{Method: http.MethodGet, Path: w.path("/" + project.String())}
	patch := apptest.Request{Method: http.MethodPatch, Path: w.path("/" + project.String()), Body: map[string]any{"Name": "From CI"}}
	decode(t, w.env.Bearer(reader.Value, get), http.StatusOK)
	requireError(t, w.env.Bearer(reader.Value, patch), http.StatusForbidden, "FORBIDDEN")
	decode(t, w.env.Bearer(writer.Value, patch), http.StatusOK)
	var via, viaID string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "Data"->>'Via', "Data"->>'ViaId' FROM "AuditEvents" WHERE "Action" = 'project.updated' AND "TargetId" = $1`, project.String()).Scan(&via, &viaID))
	assert.Equal(t, "api_token", via, "the audit log tells a token from a session")
	assert.Equal(t, writer.ID.String(), viaID)
	assert.NotContains(t, viaID, writer.Value)
	decode(t, w.env.Bearer(writer.Value, apptest.Request{
		Method: http.MethodPost, Path: w.path(""),
		Body: map[string]any{"Name": "From CI", "Slug": "from-ci"},
	}), http.StatusCreated)

	jobsOnly := maintainer.MustCreateAPIToken(w.org, []string{"job.read"}, memberPassword)
	requireError(t, w.env.Bearer(jobsOnly.Value, get), http.StatusForbidden, "FORBIDDEN")
}

func TestOnlyAnAdminLetsForksRestoreTheCache(t *testing.T) {
	w := newWorld(t)
	maintainer, _ := w.member(t, "maintainer")
	p := decode(t, create(maintainer, w, map[string]any{"Name": "C", "Slug": "c"}), http.StatusCreated)
	path := w.path("/" + p.Id)
	patch := func(c *apptest.Client, v string) apptest.Response {
		return c.Do(apptest.Request{Method: http.MethodPatch, Path: path, Body: map[string]any{"ForkCacheRestore": v}})
	}
	requireError(t, patch(maintainer, "default_branch"), http.StatusForbidden, "FORK_CACHE_NOT_ALLOWED")
	requireError(t, patch(w.owner, "everything"), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	opened := decode(t, patch(w.owner, "default_branch"), http.StatusOK)
	assert.Equal(t, "default_branch", opened.ForkCacheRestore)
	closed := decode(t, patch(maintainer, "none"), http.StatusOK)
	assert.Equal(t, "none", closed.ForkCacheRestore, "anyone who manages the project may close it again")
}
