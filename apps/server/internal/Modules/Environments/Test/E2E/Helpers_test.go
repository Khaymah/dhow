package e2e_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const memberPassword = "a long password for an environment member"

type world struct {
	env      *apptest.Env
	owner    *apptest.Client
	ownerID  uuid.UUID
	password string
	org      uuid.UUID
	project  uuid.UUID
}

type refView struct{ Kind, Pattern string }

type environmentView struct {
	Id, OrganizationId, ProjectId, Name, Description string
	Version, RuleVersion                             int64
	Paused                                           bool
	PausedAt, PausedByUserId, PausedReason           *string
	Rules                                            struct {
		AllowedRefs       []refView
		AllowPullRequests bool
		Reviewers         struct {
			UserIds, TeamIds     []string
			UserCount, TeamCount int
			IdsShown             bool
		}
		RequiredApprovals    int
		AllowSelfApproval    bool
		WaitMinutes          int
		ApprovalTimeoutHours int
		RequirePinnedImages  bool
		AllowedPlugins       []string
		RequiredIsolation    string
		AllowAdminBypass     bool
		SupersedePending     bool
	}
}

func newWorld(t *testing.T) world {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	ownerID := uuid.MustParse(owner.UserID)
	return world{env: env, owner: owner, ownerID: ownerID, password: d.Password, org: org, project: env.SeedProject(org, ownerID)}
}

func (w world) member(t *testing.T, role string) (*apptest.Client, uuid.UUID) {
	t.Helper()
	id := w.env.SeedUser(w.org, uuid.NewString()[:8]+"@example.com", memberPassword, role)
	return w.env.SeedSession(id), id
}

func (w world) path(project uuid.UUID, rest string) string {
	return apptest.OrgPath(w.org, "/projects/"+project.String()+"/environments"+rest)
}

func (w world) create(c *apptest.Client, body map[string]any) apptest.Response {
	if _, ok := body["CurrentFactor"]; !ok {
		body["CurrentFactor"] = memberFactor()
		if c == w.owner {
			body["CurrentFactor"] = w.ownerFactor()
		}
	}
	return w.createRaw(c, body)
}

func (w world) createRaw(c *apptest.Client, body map[string]any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPost, Path: w.path(w.project, ""), Body: body})
}

func (w world) get(c *apptest.Client, name string) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodGet, Path: w.path(w.project, "/"+name)})
}

func (w world) update(c *apptest.Client, name string, body map[string]any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPut, Path: w.path(w.project, "/"+name), Body: body})
}

func (w world) post(c *apptest.Client, name, action string, body any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPost, Path: w.path(w.project, "/"+name+"/"+action), Body: body})
}

func (w world) ownerFactor() map[string]any { return apptest.PasswordFactor(w.password) }

func memberFactor() map[string]any { return apptest.PasswordFactor(memberPassword) }

func wrongFactor() map[string]any { return apptest.PasswordFactor("not the password at all") }

func decode(t *testing.T, res apptest.Response, status int) environmentView {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	var e environmentView
	res.Decode(t, &e)
	return e
}

func requireError(t *testing.T, res apptest.Response, status int, code string) {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	require.NotNil(t, res.Error, string(res.Raw))
	assert.Equal(t, code, res.Error.Code)
}

func detailFields(res apptest.Response) []string {
	if res.Error == nil {
		return nil
	}
	out := make([]string, 0, len(res.Error.Details))
	for _, d := range res.Error.Details {
		out = append(out, d.Field)
	}
	return out
}

func (w world) production(t *testing.T) environmentView {
	t.Helper()
	return decode(t, w.create(w.owner, map[string]any{"Name": "production"}), http.StatusCreated)
}
