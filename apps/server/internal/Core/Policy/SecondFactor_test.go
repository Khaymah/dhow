package policy_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

const grace = 7 * 24 * time.Hour

func TestSecondFactorRuleCoversRoles(t *testing.T) {
	tests := []struct {
		rule policy.SecondFactorRule
		want map[policy.Role]bool
	}{
		{policy.SecondFactorOff, map[policy.Role]bool{}},
		{policy.SecondFactorAdmins, map[policy.Role]bool{policy.RoleOwner: true, policy.RoleAdmin: true}},
		{policy.SecondFactorEveryone, map[policy.Role]bool{
			policy.RoleOwner: true, policy.RoleAdmin: true, policy.RoleMaintainer: true, policy.RoleDeveloper: true, policy.RoleViewer: true,
		}},
	}
	for _, tt := range tests {
		for _, role := range policy.Roles() {
			t.Run(string(tt.rule)+"/"+string(role), func(t *testing.T) {
				assert.Equal(t, tt.want[role], tt.rule.Covers(role))
			})
		}
		assert.False(t, tt.rule.Covers(policy.Role("intruder")), "an unknown role is never covered by accident")
	}
}

func TestParseSecondFactorRule(t *testing.T) {
	for _, r := range policy.SecondFactorRules() {
		got, ok := policy.ParseSecondFactorRule(string(r))
		assert.True(t, ok)
		assert.Equal(t, r, got)
	}
	for _, bad := range []string{"", "Off", "admin", "all", "everyone "} {
		_, ok := policy.ParseSecondFactorRule(bad)
		assert.False(t, ok, bad)
	}
	assert.Less(t, policy.SecondFactorOff.Reach(), policy.SecondFactorAdmins.Reach())
	assert.Less(t, policy.SecondFactorAdmins.Reach(), policy.SecondFactorEveryone.Reach())
}

func TestSecondFactorDue(t *testing.T) {
	enabled := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	joinedBefore := enabled.Add(-30 * 24 * time.Hour)
	joinedAfter := enabled.Add(3 * 24 * time.Hour)
	tests := []struct {
		name  string
		state policy.SecondFactorState
		want  time.Time
	}{
		{"off asks nothing", policy.SecondFactorState{Rule: policy.SecondFactorOff, JoinedAt: joinedBefore, Role: policy.RoleOwner}, time.Time{}},
		{"a member with a factor owes nothing", policy.SecondFactorState{
			Rule: policy.SecondFactorEveryone, Since: &enabled, JoinedAt: joinedBefore, Role: policy.RoleViewer, HasFactor: true,
		}, time.Time{}},
		{"admins does not cover a developer", policy.SecondFactorState{
			Rule: policy.SecondFactorAdmins, Since: &enabled, JoinedAt: joinedBefore, Role: policy.RoleDeveloper,
		}, time.Time{}},
		{"an earlier member counts from the rule", policy.SecondFactorState{
			Rule: policy.SecondFactorAdmins, Since: &enabled, JoinedAt: joinedBefore, Role: policy.RoleAdmin,
		}, enabled.Add(grace)},
		{"a later member counts from joining", policy.SecondFactorState{
			Rule: policy.SecondFactorEveryone, Since: &enabled, JoinedAt: joinedAfter, Role: policy.RoleViewer,
		}, joinedAfter.Add(grace)},
		{"an admin counts from when admins were first covered", policy.SecondFactorState{
			Rule: policy.SecondFactorEveryone, Since: &joinedAfter, AdminsSince: &enabled, JoinedAt: joinedBefore, Role: policy.RoleAdmin,
		}, enabled.Add(grace)},
		{"a viewer counts from the widening", policy.SecondFactorState{
			Rule: policy.SecondFactorEveryone, Since: &joinedAfter, AdminsSince: &enabled, JoinedAt: joinedBefore, Role: policy.RoleViewer,
		}, joinedAfter.Add(grace)},
		{"a rule without a start counts from joining", policy.SecondFactorState{
			Rule: policy.SecondFactorEveryone, JoinedAt: joinedBefore, Role: policy.RoleViewer,
		}, joinedBefore.Add(grace)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.SecondFactorDue(tt.state, grace))
		})
	}
}

func TestSecondFactorBlocksOnlySensitiveActionsAfterTheDueDate(t *testing.T) {
	org, other := uuid.New(), uuid.New()
	due := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	subject := policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{
		{OrganizationID: org, Role: policy.RoleOwner, SecondFactorDue: due},
		{OrganizationID: other, Role: policy.RoleOwner},
	}}
	here := policy.Resource{OrganizationID: org}
	for _, a := range policy.Actions() {
		t.Run(string(a), func(t *testing.T) {
			assert.False(t, policy.SecondFactorBlocks(subject, a, here, due.Add(-time.Second)), "inside the grace period")
			assert.Equal(t, policy.IsSensitive(a), policy.SecondFactorBlocks(subject, a, here, due), "from the due moment on")
			assert.False(t, policy.SecondFactorBlocks(subject, a, policy.Resource{OrganizationID: other}, due.Add(time.Hour)),
				"another organization with its own rule")
			assert.False(t, policy.SecondFactorBlocks(subject, a, policy.Resource{}, due.Add(time.Hour)), "no organization")
		})
	}
	want := []policy.Action{
		policy.OrgUpdate, policy.OrgTransfer, policy.OrgSecurity, policy.MemberManage, policy.RunnerManage, policy.JobCreate, policy.TokenCreate,
		policy.ProjectManage, policy.RunApprove, policy.EnvironmentManage, policy.EnvironmentBypass, policy.DeploymentApprove,
		policy.SecretManage, policy.OrgSecretManage,
	}
	assert.ElementsMatch(t, want, policy.SensitiveActions())
	for _, read := range []policy.Action{policy.OrgRead, policy.MemberRead, policy.AuditRead, policy.RunnerRead, policy.JobRead, policy.LogRead} {
		assert.False(t, policy.IsSensitive(read), "%s stays allowed", read)
	}
}

func TestOrgSecurityIsOwnerOnlyAndNeverATokenScope(t *testing.T) {
	for _, role := range policy.Roles() {
		assert.Equal(t, role == policy.RoleOwner, policy.Allows(role, policy.OrgSecurity), role)
	}
	assert.False(t, policy.IsTokenAction(policy.OrgSecurity))
}
