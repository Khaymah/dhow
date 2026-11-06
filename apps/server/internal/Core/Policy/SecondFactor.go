package policy

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

type SecondFactorRule string

const (
	SecondFactorOff      SecondFactorRule = "off"
	SecondFactorAdmins   SecondFactorRule = "admins"
	SecondFactorEveryone SecondFactorRule = "everyone"
)

func SecondFactorRules() []SecondFactorRule {
	return []SecondFactorRule{SecondFactorOff, SecondFactorAdmins, SecondFactorEveryone}
}

func ParseSecondFactorRule(v string) (SecondFactorRule, bool) {
	for _, r := range SecondFactorRules() {
		if string(r) == v {
			return r, true
		}
	}
	return "", false
}

func (r SecondFactorRule) Reach() int {
	switch r {
	case SecondFactorAdmins:
		return 1
	case SecondFactorEveryone:
		return 2
	default:
		return 0
	}
}

func (r SecondFactorRule) Covers(role Role) bool {
	switch r {
	case SecondFactorEveryone:
		return Rank(role) > 0
	case SecondFactorAdmins:
		return AtLeast(role, RoleAdmin)
	default:
		return false
	}
}

func SensitiveActions() []Action {
	return []Action{
		OrgUpdate, OrgTransfer, OrgSecurity, MemberManage, RunnerManage, JobCreate, TokenCreate, ProjectManage, RunApprove,
		EnvironmentManage, EnvironmentBypass, DeploymentApprove, DeploymentRollback, SecretManage, OrgSecretManage, IngestManage,
	}
}

func IsSensitive(a Action) bool { return slices.Contains(SensitiveActions(), a) }

type SecondFactorState struct {
	Rule        SecondFactorRule
	Since       *time.Time
	AdminsSince *time.Time
	JoinedAt    time.Time
	Role        Role
	HasFactor   bool
}

func SecondFactorDue(s SecondFactorState, grace time.Duration) time.Time {
	if s.HasFactor || !s.Rule.Covers(s.Role) {
		return time.Time{}
	}
	since := s.Since
	if s.AdminsSince != nil && SecondFactorAdmins.Covers(s.Role) {
		since = s.AdminsSince
	}
	start := s.JoinedAt
	if since != nil && since.After(start) {
		start = *since
	}
	return start.Add(grace)
}

func SecondFactorDueAsAdmin(s SecondFactorState, grace time.Duration) time.Time {
	if !AtLeast(s.Role, RoleAdmin) {
		s.Role = RoleAdmin
	}
	return SecondFactorDue(s, grace)
}

func SecondFactorBlocks(s Subject, action Action, r Resource, now time.Time) bool {
	if !IsSensitive(action) || r.OrganizationID == uuid.Nil {
		return false
	}
	for _, m := range s.Memberships {
		if m.OrganizationID != r.OrganizationID {
			continue
		}
		due := m.SecondFactorDue
		if r.ProjectID != uuid.Nil && !AtLeast(m.Role, RoleAdmin) && AtLeast(ProjectRole(m.Role, r.ProjectBindings), RoleAdmin) {
			due = m.AdminSecondFactorDue
		}
		return !due.IsZero() && !now.Before(due)
	}
	return false
}
