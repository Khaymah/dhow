package policy_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func TestProjectRoleRaisesOrLowersButNeverTouchesAdmins(t *testing.T) {
	tests := []struct {
		name     string
		org      policy.Role
		bindings []policy.Role
		want     policy.Role
	}{
		{"no binding keeps the organization role", policy.RoleDeveloper, nil, policy.RoleDeveloper},
		{"a binding raises a developer", policy.RoleDeveloper, []policy.Role{policy.RoleMaintainer}, policy.RoleMaintainer},
		{"a binding lowers a maintainer", policy.RoleMaintainer, []policy.Role{policy.RoleViewer}, policy.RoleViewer},
		{"the highest of several bindings wins", policy.RoleViewer, []policy.Role{policy.RoleDeveloper, policy.RoleMaintainer}, policy.RoleMaintainer},
		{"a binding never grants owner", policy.RoleDeveloper, []policy.Role{policy.RoleOwner}, policy.RoleDeveloper},
		{"an unknown binding is ignored", policy.RoleDeveloper, []policy.Role{"root"}, policy.RoleDeveloper},
		{"an admin is never lowered", policy.RoleAdmin, []policy.Role{policy.RoleViewer}, policy.RoleAdmin},
		{"an owner is never lowered", policy.RoleOwner, []policy.Role{policy.RoleViewer}, policy.RoleOwner},
		{"no organization role means no project role", "", []policy.Role{policy.RoleAdmin}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.ProjectRole(tt.org, tt.bindings))
		})
	}
}

func TestCanOnAProjectUsesTheProjectRole(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	member := func(role policy.Role) policy.Subject {
		return policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: role}}}
	}
	on := func(bindings ...policy.Role) policy.Resource {
		return policy.Resource{OrganizationID: org, ProjectID: project, ProjectBindings: bindings}
	}
	tests := []struct {
		name     string
		subject  policy.Subject
		action   policy.Action
		resource policy.Resource
		want     bool
	}{
		{"a developer cannot manage a project", member(policy.RoleDeveloper), policy.ProjectManage, on(), false},
		{"a developer with a maintainer binding manages it", member(policy.RoleDeveloper), policy.ProjectManage, on(policy.RoleMaintainer), true},
		{
			"a developer with a maintainer binding still cannot create projects", member(policy.RoleDeveloper), policy.ProjectManage,
			policy.Resource{OrganizationID: org},
			false,
		},
		{"a maintainer lowered to viewer cannot manage it", member(policy.RoleMaintainer), policy.ProjectManage, on(policy.RoleViewer), false},
		{"a maintainer lowered to viewer still reads it", member(policy.RoleMaintainer), policy.ProjectRead, on(policy.RoleViewer), true},
		{"a viewer reads a project", member(policy.RoleViewer), policy.ProjectRead, on(), true},
		{"a project resource never grants an organization action", member(policy.RoleOwner), policy.MemberManage, on(), false},
		{"another organization's member gets nothing", policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{
			{OrganizationID: uuid.New(), Role: policy.RoleOwner},
		}}, policy.ProjectRead, on(policy.RoleAdmin), false},
		{"a token needs the scope as well", policy.Subject{
			UserID: uuid.New(), TokenID: uuid.New(), TokenScopes: []policy.Action{policy.ProjectRead},
			Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleDeveloper}},
		}, policy.ProjectManage, on(policy.RoleMaintainer), false},
		{"a token with the scope follows the binding", policy.Subject{
			UserID: uuid.New(), TokenID: uuid.New(), TokenScopes: []policy.Action{policy.ProjectManage},
			Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleDeveloper}},
		}, policy.ProjectManage, on(policy.RoleMaintainer), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.Can(tt.subject, tt.action, tt.resource))
		})
	}
}

func TestProjectRules(t *testing.T) {
	rule := policy.Project(http.MethodPatch, "/organizations/{id}/projects/{projectId}", policy.ProjectManage, "id", "projectId")
	assert.True(t, rule.Valid())
	assert.True(t, rule.AllowTokens().Valid())
	assert.Equal(t, policy.ScopeProject, rule.Scope)
	tests := []struct {
		name string
		rule policy.Rule
	}{
		{"an organization action", policy.Project(http.MethodGet, "/x/{id}/{p}", policy.MemberRead, "id", "p")},
		{"no project parameter", policy.Project(http.MethodGet, "/x/{id}", policy.ProjectRead, "id", "")},
		{"no organization parameter", policy.Project(http.MethodGet, "/x/{p}", policy.ProjectRead, "", "p")},
		{"one parameter for both", policy.Project(http.MethodGet, "/x/{id}", policy.ProjectRead, "id", "id")},
		{"a project parameter on an organization rule", policy.Rule{
			Method: http.MethodGet, Path: "/x/{id}", Scope: policy.ScopeOrganization, Action: policy.ProjectRead, OrgParam: "id", ProjectParam: "p",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, tt.rule.Valid())
		})
	}
	assert.True(t, policy.IsTokenAction(policy.ProjectRead))
	assert.True(t, policy.IsTokenAction(policy.ProjectManage))
	for _, a := range policy.ProjectActions() {
		assert.Contains(t, policy.Actions(), a)
	}
}

func TestSecondFactorFollowsTheProjectRole(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	now := time.Now()
	past := now.Add(-time.Hour)
	member := func(role policy.Role, due, adminDue time.Time) policy.Subject {
		return policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{
			{OrganizationID: org, Role: role, SecondFactorDue: due, AdminSecondFactorDue: adminDue},
		}}
	}
	on := func(bindings ...policy.Role) policy.Resource {
		return policy.Resource{OrganizationID: org, ProjectID: project, ProjectBindings: bindings}
	}
	tests := []struct {
		name     string
		subject  policy.Subject
		action   policy.Action
		resource policy.Resource
		want     bool
	}{
		{"an overdue admin cannot manage a project", member(policy.RoleAdmin, past, past), policy.ProjectManage, on(), true},
		{"an overdue admin still reads it", member(policy.RoleAdmin, past, past), policy.ProjectRead, on(), false},
		{
			"a developer raised to project admin is covered by the admins rule", member(policy.RoleDeveloper, time.Time{}, past),
			policy.ProjectManage, on(policy.RoleAdmin), true,
		},
		{
			"a developer raised to maintainer is not", member(policy.RoleDeveloper, time.Time{}, past), policy.ProjectManage,
			on(policy.RoleMaintainer), false,
		},
		{
			"the admin due date applies only on that project", member(policy.RoleDeveloper, time.Time{}, past), policy.ProjectManage,
			policy.Resource{OrganizationID: org},
			false,
		},
		{
			"a member with a factor has no due date", member(policy.RoleDeveloper, time.Time{}, time.Time{}), policy.ProjectManage,
			on(policy.RoleAdmin), false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.SecondFactorBlocks(tt.subject, tt.action, tt.resource, now))
		})
	}
	assert.True(t, policy.IsSensitive(policy.ProjectManage))
	state := policy.SecondFactorState{Rule: policy.SecondFactorAdmins, JoinedAt: past, Role: policy.RoleDeveloper}
	assert.True(t, policy.SecondFactorDue(state, time.Minute).IsZero())
	assert.Equal(t, past.Add(time.Minute), policy.SecondFactorDueAsAdmin(state, time.Minute))
}

func TestEnvironmentActionsFollowTheProjectRole(t *testing.T) {
	org, project, other := uuid.New(), uuid.New(), uuid.New()
	member := func(role policy.Role) policy.Subject {
		return policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: role}}}
	}
	on := func(id uuid.UUID, bindings ...policy.Role) policy.Resource {
		return policy.Resource{OrganizationID: org, ProjectID: id, ProjectBindings: bindings}
	}
	tests := []struct {
		name     string
		subject  policy.Subject
		action   policy.Action
		resource policy.Resource
		want     bool
	}{
		{"an admin manages environments", member(policy.RoleAdmin), policy.EnvironmentManage, on(project), true},
		{"a maintainer does not manage environments", member(policy.RoleMaintainer), policy.EnvironmentManage, on(project), false},
		{"a maintainer pauses an environment", member(policy.RoleMaintainer), policy.EnvironmentPause, on(project), true},
		{"a developer does not pause an environment", member(policy.RoleDeveloper), policy.EnvironmentPause, on(project), false},
		{"a developer may approve a deployment", member(policy.RoleDeveloper), policy.DeploymentApprove, on(project), true},
		{"a viewer may not approve a deployment", member(policy.RoleViewer), policy.DeploymentApprove, on(project), false},
		{"a maintainer may not bypass the rules", member(policy.RoleMaintainer), policy.EnvironmentBypass, on(project), false},
		{"an admin binding grants manage on its project", member(policy.RoleDeveloper), policy.EnvironmentManage, on(project, policy.RoleAdmin), true},
		{"an admin binding grants bypass on its project", member(policy.RoleMaintainer), policy.EnvironmentBypass, on(project, policy.RoleAdmin), true},
		{"an admin binding grants nothing on another project", member(policy.RoleDeveloper), policy.EnvironmentManage, on(other), false},
		{"a viewer binding stops a maintainer pausing", member(policy.RoleMaintainer), policy.EnvironmentPause, on(project, policy.RoleViewer), false},
		{"an owner binding never grants manage", member(policy.RoleDeveloper), policy.EnvironmentManage, on(project, policy.RoleOwner), false},
		{"a token never manages environments", policy.Subject{
			UserID: uuid.New(), TokenID: uuid.New(), TokenScopes: policy.TokenActions(),
			Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleOwner}},
		}, policy.EnvironmentManage, on(project), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.Can(tt.subject, tt.action, tt.resource))
		})
	}
	for _, a := range []policy.Action{policy.EnvironmentManage, policy.EnvironmentPause, policy.EnvironmentBypass, policy.DeploymentApprove} {
		assert.True(t, policy.IsProjectAction(a), "%s", a)
		assert.False(t, policy.IsTokenAction(a), "%s", a)
		assert.Contains(t, policy.Actions(), a)
	}
	for _, a := range []policy.Action{policy.EnvironmentManage, policy.EnvironmentBypass, policy.DeploymentApprove} {
		assert.True(t, policy.IsSensitive(a), "%s", a)
	}
	assert.False(t, policy.IsSensitive(policy.EnvironmentPause), "pausing during an incident never waits for a second factor")
}

func TestSecretActions(t *testing.T) {
	org, project, other := uuid.New(), uuid.New(), uuid.New()
	member := func(role policy.Role) policy.Subject {
		return policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: role}}}
	}
	on := func(id uuid.UUID, bindings ...policy.Role) policy.Resource {
		return policy.Resource{OrganizationID: org, ProjectID: id, ProjectBindings: bindings}
	}
	token := policy.Subject{
		UserID: uuid.New(), TokenID: uuid.New(), TokenScopes: policy.TokenActions(),
		Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleOwner}},
	}
	tests := []struct {
		name     string
		subject  policy.Subject
		action   policy.Action
		resource policy.Resource
		want     bool
	}{
		{"a developer reads secret names", member(policy.RoleDeveloper), policy.SecretRead, on(project), true},
		{"a viewer does not read secret names", member(policy.RoleViewer), policy.SecretRead, on(project), false},
		{"a maintainer manages project secrets", member(policy.RoleMaintainer), policy.SecretManage, on(project), true},
		{"a developer does not manage project secrets", member(policy.RoleDeveloper), policy.SecretManage, on(project), false},
		{"a maintainer binding lets a developer manage its project", member(policy.RoleDeveloper), policy.SecretManage, on(project, policy.RoleMaintainer), true},
		{"a maintainer binding grants nothing on another project", member(policy.RoleDeveloper), policy.SecretManage, on(other), false},
		{"a viewer binding stops a maintainer", member(policy.RoleMaintainer), policy.SecretManage, on(project, policy.RoleViewer), false},
		{"a maintainer does not manage organization secrets", member(policy.RoleMaintainer), policy.OrgSecretManage, on(uuid.Nil), false},
		{"an admin manages organization secrets", member(policy.RoleAdmin), policy.OrgSecretManage, on(uuid.Nil), true},
		{"a token reads secret names", token, policy.SecretRead, on(project), true},
		{"a token never writes project secrets", token, policy.SecretManage, on(project), false},
		{"a token never writes organization secrets", token, policy.OrgSecretManage, on(uuid.Nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.Can(tt.subject, tt.action, tt.resource))
		})
	}
	assert.True(t, policy.IsProjectAction(policy.SecretRead))
	assert.True(t, policy.IsProjectAction(policy.SecretManage))
	assert.False(t, policy.IsProjectAction(policy.OrgSecretManage))
	assert.Equal(t, []bool{true, false, false},
		[]bool{policy.IsTokenAction(policy.SecretRead), policy.IsTokenAction(policy.SecretManage), policy.IsTokenAction(policy.OrgSecretManage)})
	assert.True(t, policy.IsSensitive(policy.SecretManage))
	assert.True(t, policy.IsSensitive(policy.OrgSecretManage))
	assert.False(t, policy.IsSensitive(policy.SecretRead))
}
