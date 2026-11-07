package policy_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func TestMatrix(t *testing.T) {
	org := uuid.New()
	want := map[policy.Role]map[policy.Action]bool{
		policy.RoleOwner: {
			policy.OrgRead: true, policy.OrgUpdate: true, policy.OrgLeave: true, policy.OrgTransfer: true, policy.OrgSecurity: true,
			policy.MemberRead: true, policy.MemberManage: true, policy.AuditRead: true, policy.RunnerRead: true, policy.RunnerManage: true,
			policy.TokenCreate: true, policy.TokenManage: true, policy.JobRead: true, policy.JobCreate: true, policy.JobCancel: true,
			policy.LogRead: true, policy.ProjectRead: true, policy.ProjectManage: true,
			policy.RunRead: true, policy.RunTrigger: true, policy.RunCancel: true, policy.RunApprove: true, policy.ArtifactRead: true,
			policy.EnvironmentManage: true, policy.EnvironmentPause: true, policy.EnvironmentBypass: true, policy.DeploymentApprove: true,
			policy.DeploymentRollback: true, policy.DeploymentCheck: true, policy.SecretRead: true, policy.SecretManage: true, policy.OrgSecretManage: true,
			policy.IngestManage: true, policy.NotificationManage: true, policy.OrgNotificationManage: true,
		},
		policy.RoleAdmin: {
			policy.OrgRead: true, policy.OrgUpdate: true, policy.OrgLeave: true, policy.MemberRead: true, policy.MemberManage: true,
			policy.AuditRead: true, policy.RunnerRead: true, policy.RunnerManage: true, policy.TokenCreate: true, policy.TokenManage: true,
			policy.JobRead: true, policy.JobCreate: true, policy.JobCancel: true, policy.LogRead: true, policy.ProjectRead: true,
			policy.ProjectManage: true, policy.RunRead: true, policy.RunTrigger: true, policy.RunCancel: true, policy.RunApprove: true,
			policy.ArtifactRead: true, policy.EnvironmentManage: true, policy.EnvironmentPause: true, policy.EnvironmentBypass: true,
			policy.DeploymentApprove: true, policy.DeploymentRollback: true, policy.DeploymentCheck: true, policy.SecretRead: true, policy.SecretManage: true,
			policy.OrgSecretManage: true, policy.IngestManage: true, policy.NotificationManage: true, policy.OrgNotificationManage: true,
		},
		policy.RoleMaintainer: {
			policy.OrgRead: true, policy.OrgLeave: true, policy.MemberRead: true, policy.RunnerRead: true, policy.JobRead: true,
			policy.JobCancel: true, policy.LogRead: true, policy.TokenCreate: true, policy.ProjectRead: true, policy.ProjectManage: true,
			policy.RunRead: true, policy.RunTrigger: true, policy.RunCancel: true, policy.RunApprove: true, policy.ArtifactRead: true,
			policy.EnvironmentPause: true, policy.DeploymentApprove: true, policy.DeploymentRollback: true, policy.DeploymentCheck: true, policy.SecretRead: true,
			policy.SecretManage: true, policy.IngestManage: true, policy.NotificationManage: true,
		},
		policy.RoleDeveloper: {
			policy.OrgRead: true, policy.OrgLeave: true, policy.MemberRead: true, policy.RunnerRead: true, policy.JobRead: true,
			policy.LogRead: true, policy.TokenCreate: true, policy.ProjectRead: true, policy.RunRead: true, policy.RunTrigger: true,
			policy.ArtifactRead: true, policy.DeploymentApprove: true, policy.SecretRead: true,
		},
		policy.RoleViewer: {
			policy.OrgRead: true, policy.OrgLeave: true, policy.JobRead: true, policy.TokenCreate: true, policy.ProjectRead: true,
			policy.RunRead: true,
		},
	}
	for _, role := range policy.Roles() {
		subject := policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: role}}}
		for _, action := range policy.Actions() {
			if action == policy.SessionManageSelf {
				continue
			}
			t.Run(string(role)+"/"+string(action), func(t *testing.T) {
				assert.Equal(t, want[role][action], policy.Can(subject, action, policy.Resource{OrganizationID: org}))
			})
		}
	}
}

func TestSessionManageSelf(t *testing.T) {
	org := uuid.New()
	tests := []struct {
		name     string
		subject  policy.Subject
		resource policy.Resource
		want     bool
	}{
		{"member", policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleViewer}}}, policy.Resource{}, true},
		{"no membership at all", policy.Subject{UserID: uuid.New()}, policy.Resource{}, true},
		{"anonymous", policy.Subject{}, policy.Resource{}, false},
		{"self action never carries an organization", policy.Subject{UserID: uuid.New()}, policy.Resource{OrganizationID: org}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.Can(tt.subject, policy.SessionManageSelf, tt.resource))
		})
	}
}

func TestCanDenies(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	owner := policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: orgA, Role: policy.RoleOwner}}}
	tests := []struct {
		name     string
		subject  policy.Subject
		action   policy.Action
		resource policy.Resource
	}{
		{"anonymous", policy.Subject{}, policy.OrgRead, policy.Resource{OrganizationID: orgA}},
		{"member of another organization", owner, policy.OrgRead, policy.Resource{OrganizationID: orgB}},
		{"organization action without an organization", owner, policy.OrgRead, policy.Resource{}},
		{"unknown action", owner, policy.Action("org.delete"), policy.Resource{OrganizationID: orgA}},
		{"unknown role", policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: orgA, Role: "root"}}}, policy.OrgRead, policy.Resource{OrganizationID: orgA}},
		{"instance action through the organization check", owner, policy.OrgCreate, policy.Resource{OrganizationID: orgA}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, policy.Can(tt.subject, tt.action, tt.resource))
		})
	}
}

func TestParseRole(t *testing.T) {
	for _, r := range policy.Roles() {
		got, ok := policy.ParseRole(string(r))
		assert.True(t, ok)
		assert.Equal(t, r, got)
	}
	_, ok := policy.ParseRole("superuser")
	assert.False(t, ok)
}

func TestTable(t *testing.T) {
	table := policy.NewTable(
		policy.Public(http.MethodPost, "/sessions"),
		policy.Authenticated(http.MethodGet, "/me", policy.SessionManageSelf),
		policy.Organization(http.MethodGet, "/organizations/{id}", policy.OrgRead, "id"),
		policy.Organization(http.MethodGet, "/broken", policy.OrgRead, ""),
		policy.Instance(http.MethodPost, "/organizations", policy.OrgCreate),
		policy.Instance(http.MethodPost, "/instance-read", policy.OrgRead),
		policy.Organization(http.MethodPost, "/organizations/{id}/create", policy.OrgCreate, "id"),
		policy.Authenticated(http.MethodGet, "/wrong-action", policy.OrgRead),
		policy.Rule{Method: http.MethodGet, Path: "/no-scope"},
		policy.Runner("/dhow.runner.v1.AuthService/Challenge", policy.RunnerChallenge),
		policy.Rule{Method: http.MethodGet, Path: "/dhow.runner.v1.AuthService/Authenticate", Scope: policy.ScopeRunner, Runner: policy.RunnerSigned},
		policy.Rule{Method: http.MethodPost, Path: "/dhow.runner.v1.AuthService/RotateKeys", Scope: policy.ScopeRunner},
		policy.Rule{Method: http.MethodPost, Path: "/dhow.runner.v1.LeaseService/Lease", Scope: policy.ScopeRunner, Runner: policy.RunnerSession, Action: policy.OrgRead},
		policy.Rule{Method: http.MethodPost, Path: "/runner-on-public", Scope: policy.ScopePublic, Runner: policy.RunnerSession},
	)
	tests := []struct {
		name   string
		method string
		path   string
		found  bool
		scope  policy.Scope
	}{
		{"public", http.MethodPost, "/sessions", true, policy.ScopePublic},
		{"authenticated", http.MethodGet, "/me", true, policy.ScopeAuthenticated},
		{"organization", http.MethodGet, "/organizations/{id}", true, policy.ScopeOrganization},
		{"wrong method", http.MethodDelete, "/sessions", false, 0},
		{"unknown path", http.MethodGet, "/unknown", false, 0},
		{"organization rule without a parameter is invalid", http.MethodGet, "/broken", false, 0},
		{"instance", http.MethodPost, "/organizations", true, policy.ScopeInstance},
		{"instance rule with an organization action is invalid", http.MethodPost, "/instance-read", false, 0},
		{"organization rule with an instance action is invalid", http.MethodPost, "/organizations/{id}/create", false, 0},
		{"authenticated rule with an organization action is invalid", http.MethodGet, "/wrong-action", false, 0},
		{"rule without a scope is invalid", http.MethodGet, "/no-scope", false, 0},
		{"runner procedure", http.MethodPost, "/dhow.runner.v1.AuthService/Challenge", true, policy.ScopeRunner},
		{"runner procedure is POST only", http.MethodGet, "/dhow.runner.v1.AuthService/Challenge", false, 0},
		{"runner rule with another method is invalid", http.MethodGet, "/dhow.runner.v1.AuthService/Authenticate", false, 0},
		{"runner rule without an access kind is invalid", http.MethodPost, "/dhow.runner.v1.AuthService/RotateKeys", false, 0},
		{"runner rule with an action is invalid", http.MethodPost, "/dhow.runner.v1.LeaseService/Lease", false, 0},
		{"runner access on another scope is invalid", http.MethodPost, "/runner-on-public", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, ok := table.Lookup(tt.method, tt.path)
			assert.Equal(t, tt.found, ok)
			assert.Equal(t, tt.scope, rule.Scope)
		})
	}
}

func TestRuleUnsafe(t *testing.T) {
	tests := map[string]bool{
		http.MethodGet: false, http.MethodHead: false, http.MethodOptions: false,
		http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	}
	for method, want := range tests {
		assert.Equal(t, want, policy.Public(method, "/x").Unsafe(), method)
	}
}

func TestCanInstance(t *testing.T) {
	org := uuid.New()
	owner := policy.Subject{UserID: uuid.New(), InstanceOwner: true}
	member := policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleOwner}}}
	tests := []struct {
		name    string
		subject policy.Subject
		action  policy.Action
		inst    policy.InstanceSettings
		want    bool
	}{
		{"instance owner", owner, policy.OrgCreate, policy.InstanceSettings{}, true},
		{"owner of an organization only", member, policy.OrgCreate, policy.InstanceSettings{}, false},
		{"any user when the instance allows it", member, policy.OrgCreate, policy.InstanceSettings{AnyUserCreatesOrganizations: true}, true},
		{"anonymous even when the instance allows it", policy.Subject{}, policy.OrgCreate, policy.InstanceSettings{AnyUserCreatesOrganizations: true}, false},
		{"organization action through the instance check", owner, policy.OrgRead, policy.InstanceSettings{}, false},
		{"instance owner manages the settings", owner, policy.InstanceSettingsManage, policy.InstanceSettings{}, true},
		{"an organization owner never manages the settings", member, policy.InstanceSettingsManage, policy.InstanceSettings{AnyUserCreatesOrganizations: true}, false},
		{"anonymous never manages the settings", policy.Subject{}, policy.InstanceSettingsManage, policy.InstanceSettings{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.CanInstance(tt.subject, tt.action, tt.inst))
		})
	}
}

func TestRunnerRule(t *testing.T) {
	for _, access := range []policy.RunnerAccess{policy.RunnerRegistration, policy.RunnerChallenge, policy.RunnerSigned, policy.RunnerSession} {
		r := policy.Runner("/dhow.runner.v1.RegistrationService/Register", access)
		assert.True(t, r.Valid())
		assert.Equal(t, http.MethodPost, r.Method)
		assert.True(t, r.Unsafe())
	}
	assert.False(t, policy.Runner("/x", policy.RunnerSession+1).Valid())
	assert.False(t, policy.Runner("no-slash", policy.RunnerSession).Valid())
}

func TestRunnerActionsNeedAMembership(t *testing.T) {
	org := uuid.New()
	developer := policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleDeveloper}}}
	assert.True(t, policy.Can(developer, policy.RunnerRead, policy.Resource{OrganizationID: org}))
	assert.False(t, policy.Can(developer, policy.RunnerManage, policy.Resource{OrganizationID: org}))
	assert.False(t, policy.Can(developer, policy.RunnerRead, policy.Resource{OrganizationID: uuid.New()}))
	assert.False(t, policy.Can(policy.Subject{}, policy.RunnerRead, policy.Resource{OrganizationID: org}))
}

func TestTokenSubjectIsTheIntersectionOfRoleAndScopes(t *testing.T) {
	org := uuid.New()
	token := func(role policy.Role, scopes ...policy.Action) policy.Subject {
		return policy.Subject{
			UserID: uuid.New(), TokenID: uuid.New(), TokenScopes: scopes,
			Memberships: []policy.Membership{{OrganizationID: org, Role: role}},
		}
	}
	tests := []struct {
		name    string
		subject policy.Subject
		action  policy.Action
		want    bool
	}{
		{"scope and role allow", token(policy.RoleAdmin, policy.MemberManage), policy.MemberManage, true},
		{"role allows but the scope is missing", token(policy.RoleAdmin, policy.MemberRead), policy.MemberManage, false},
		{"scope present but the role no longer allows it", token(policy.RoleDeveloper, policy.MemberManage), policy.MemberManage, false},
		{"a token never manages its own session", token(policy.RoleOwner, policy.TokenActions()...), policy.SessionManageSelf, false},
		{"a token never leaves the organization", token(policy.RoleOwner, policy.TokenActions()...), policy.OrgLeave, false},
		{"a token never transfers ownership", token(policy.RoleOwner, policy.TokenActions()...), policy.OrgTransfer, false},
		{"a token never makes tokens", token(policy.RoleOwner, policy.TokenActions()...), policy.TokenCreate, false},
		{"a token never manages tokens", token(policy.RoleOwner, policy.TokenActions()...), policy.TokenManage, false},
		{"a forged scope outside the token actions", token(policy.RoleOwner, policy.TokenCreate), policy.TokenCreate, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := policy.Resource{OrganizationID: org}
			if tt.action == policy.SessionManageSelf {
				resource = policy.Resource{}
			}
			assert.Equal(t, tt.want, policy.Can(tt.subject, tt.action, resource))
		})
	}
	other := token(policy.RoleOwner, policy.OrgRead)
	assert.False(t, policy.Can(other, policy.OrgRead, policy.Resource{OrganizationID: uuid.New()}), "another organization")
	owner := token(policy.RoleOwner, policy.OrgRead)
	owner.InstanceOwner = true
	assert.False(t, policy.CanInstance(owner, policy.OrgCreate, policy.InstanceSettings{AnyUserCreatesOrganizations: true}))
	assert.False(t, policy.CanInstance(owner, policy.InstanceSettingsManage, policy.InstanceSettings{}), "a token never manages the settings")
}

func TestTokenRules(t *testing.T) {
	ok := policy.Organization(http.MethodGet, "/organizations/{id}/members", policy.MemberRead, "id").AllowTokens()
	assert.True(t, ok.Valid())
	assert.True(t, ok.Tokens)
	assert.False(t, policy.Organization(http.MethodGet, "/organizations/{id}", policy.OrgRead, "id").Tokens, "tokens are opt-in")
	assert.False(t, policy.Organization(http.MethodPost, "/organizations/{id}/leave", policy.OrgLeave, "id").AllowTokens().Valid())
	assert.False(t, policy.Organization(http.MethodPost, "/organizations/{id}/api-tokens", policy.TokenCreate, "id").AllowTokens().Valid())
	assert.False(t, policy.Authenticated(http.MethodGet, "/me", policy.SessionManageSelf).AllowTokens().Valid())
	assert.False(t, policy.Public(http.MethodGet, "/meta").AllowTokens().Valid())
	for _, a := range policy.TokenActions() {
		assert.False(t, policy.IsInstanceAction(a))
		assert.NotEqual(t, policy.SessionManageSelf, a)
	}
}
