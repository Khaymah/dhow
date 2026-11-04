package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	secrets "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets"
)

const memberPassword = "a long password for a secrets member"

type world struct {
	env      *apptest.Env
	owner    *apptest.Client
	ownerID  uuid.UUID
	password string
	org      uuid.UUID
	project  uuid.UUID
}

type secretView struct {
	Name, Scope, Description, Visibility, UpdatedByUserId string
	Version, SizeBytes                                    int
	AvailableToPullRequests                               bool
	ProjectIds, Overrides                                 []string
	LastUsedAt                                            *string
}

type variableView struct {
	Name, Scope, Value string
	Overrides          []string
}

type registryView struct{ Id, Scope, Host, Username string }

func newWorld(t *testing.T) world {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	ownerID := uuid.MustParse(owner.UserID)
	return world{env: env, owner: owner, ownerID: ownerID, password: d.Password, org: org, project: env.SeedProject(org, ownerID)}
}

func (w world) service() *secrets.Service { return w.env.Wired.Options.Secrets }

func (w world) ownerContext() context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{Subject: policy.Subject{UserID: w.ownerID}})
}

func (w world) member(t *testing.T, role string) *apptest.Client {
	t.Helper()
	return w.env.SeedSession(w.env.SeedUser(w.org, uuid.NewString()[:8]+"@example.com", memberPassword, role))
}

func (w world) orgPath(rest string) string { return apptest.OrgPath(w.org, rest) }

func (w world) projectPath(rest string) string {
	return apptest.OrgPath(w.org, "/projects/"+w.project.String()+rest)
}

func (w world) envPath(name, rest string) string {
	return w.projectPath("/environments/" + name + rest)
}

func put(c *apptest.Client, path string, body any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPut, Path: path, Body: body})
}

func post(c *apptest.Client, path string, body any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: body})
}

func secretOf(t *testing.T, res apptest.Response, status int) secretView {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	var v secretView
	res.Decode(t, &v)
	return v
}

func requireError(t *testing.T, res apptest.Response, status int, code string) {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	require.NotNil(t, res.Error, string(res.Raw))
	assert.Equal(t, code, res.Error.Code)
}

func assertNoValue(t *testing.T, raw []byte, values ...string) {
	t.Helper()
	for _, v := range values {
		assert.False(t, strings.Contains(string(raw), v), "the value leaked: %s", raw)
	}
}
