package policy

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleOwner      Role = "owner"
	RoleAdmin      Role = "admin"
	RoleMaintainer Role = "maintainer"
	RoleDeveloper  Role = "developer"
	RoleViewer     Role = "viewer"
)

func Roles() []Role {
	return []Role{RoleOwner, RoleAdmin, RoleMaintainer, RoleDeveloper, RoleViewer}
}

func ParseRole(v string) (Role, bool) {
	for _, r := range Roles() {
		if string(r) == v {
			return r, true
		}
	}
	return "", false
}

type Action string

const (
	OrgRead            Action = "org.read"
	OrgUpdate          Action = "org.update"
	OrgLeave           Action = "org.leave"
	OrgTransfer        Action = "org.transfer"
	OrgSecurity        Action = "org.security"
	MemberRead         Action = "member.read"
	MemberManage       Action = "member.manage"
	AuditRead          Action = "audit.read"
	SessionManageSelf  Action = "session.manage_self"
	OrgCreate          Action = "org.create"
	RunnerRead         Action = "runner.read"
	RunnerManage       Action = "runner.manage"
	TokenCreate        Action = "token.create"
	TokenManage        Action = "token.manage"
	JobRead            Action = "job.read"
	JobCreate          Action = "job.create"
	JobCancel          Action = "job.cancel"
	LogRead            Action = "log.read"
	ProjectRead        Action = "project.read"
	ProjectManage      Action = "project.manage"
	RunRead            Action = "run.read"
	RunTrigger         Action = "run.trigger"
	RunCancel          Action = "run.cancel"
	RunApprove         Action = "run.approve"
	ArtifactRead       Action = "artifact.read"
	EnvironmentManage  Action = "environment.manage"
	EnvironmentPause   Action = "environment.pause"
	EnvironmentBypass  Action = "environment.bypass"
	DeploymentApprove  Action = "deployment.approve"
	DeploymentRollback Action = "deployment.rollback"
	DeploymentCheck    Action = "deployment.check"
	SecretRead         Action = "secret.read"
	SecretManage       Action = "secret.manage"
	OrgSecretManage    Action = "org.secret.manage"

	InstanceSettingsManage Action = "instance.settings.manage"
)

func Actions() []Action {
	return []Action{
		OrgRead, OrgUpdate, OrgLeave, OrgTransfer, OrgSecurity, MemberRead, MemberManage, AuditRead, SessionManageSelf, OrgCreate,
		RunnerRead, RunnerManage, TokenCreate, TokenManage, JobRead, JobCreate, JobCancel, LogRead, InstanceSettingsManage,
		ProjectRead, ProjectManage, RunRead, RunTrigger, RunCancel, RunApprove, ArtifactRead,
		EnvironmentManage, EnvironmentPause, EnvironmentBypass, DeploymentApprove, DeploymentRollback, DeploymentCheck, SecretRead, SecretManage, OrgSecretManage,
	}
}

func TokenActions() []Action {
	return []Action{
		OrgRead, OrgUpdate, MemberRead, MemberManage, AuditRead, RunnerRead, RunnerManage, JobRead, JobCancel, ProjectRead, ProjectManage,
		RunRead, RunTrigger, RunCancel, SecretRead,
	}
}

func IsTokenAction(a Action) bool { return slices.Contains(TokenActions(), a) }

func InstanceActions() []Action { return []Action{OrgCreate, InstanceSettingsManage} }

func IsInstanceAction(a Action) bool {
	for _, ia := range InstanceActions() {
		if ia == a {
			return true
		}
	}
	return false
}

func Grants(role Role) []Action {
	switch role {
	case RoleOwner:
		return []Action{
			OrgRead, OrgUpdate, OrgLeave, OrgTransfer, OrgSecurity, MemberRead, MemberManage, AuditRead, SessionManageSelf, RunnerRead,
			RunnerManage, TokenCreate, TokenManage, JobRead, JobCreate, JobCancel, LogRead, ProjectRead, ProjectManage,
			RunRead, RunTrigger, RunCancel, RunApprove, ArtifactRead, EnvironmentManage, EnvironmentPause, EnvironmentBypass, DeploymentApprove, DeploymentRollback, DeploymentCheck,
			SecretRead, SecretManage, OrgSecretManage,
		}
	case RoleAdmin:
		return []Action{
			OrgRead, OrgUpdate, OrgLeave, MemberRead, MemberManage, AuditRead, SessionManageSelf, RunnerRead, RunnerManage,
			TokenCreate, TokenManage, JobRead, JobCreate, JobCancel, LogRead, ProjectRead, ProjectManage,
			RunRead, RunTrigger, RunCancel, RunApprove, ArtifactRead, EnvironmentManage, EnvironmentPause, EnvironmentBypass, DeploymentApprove, DeploymentRollback, DeploymentCheck,
			SecretRead, SecretManage, OrgSecretManage,
		}
	case RoleMaintainer:
		return []Action{
			OrgRead, OrgLeave, MemberRead, SessionManageSelf, RunnerRead, JobRead, JobCancel, LogRead, TokenCreate, ProjectRead, ProjectManage,
			RunRead, RunTrigger, RunCancel, RunApprove, ArtifactRead, EnvironmentPause, DeploymentApprove, DeploymentRollback, DeploymentCheck, SecretRead, SecretManage,
		}
	case RoleDeveloper:
		return []Action{
			OrgRead, OrgLeave, MemberRead, SessionManageSelf, RunnerRead, JobRead, LogRead, TokenCreate, ProjectRead, RunRead, RunTrigger,
			ArtifactRead, DeploymentApprove, SecretRead,
		}
	case RoleViewer:
		return []Action{OrgRead, OrgLeave, SessionManageSelf, JobRead, TokenCreate, ProjectRead, RunRead}
	default:
		return nil
	}
}

func Allows(role Role, action Action) bool {
	for _, a := range Grants(role) {
		if a == action {
			return true
		}
	}
	return false
}

type Membership struct {
	OrganizationID       uuid.UUID
	Role                 Role
	SecondFactorDue      time.Time
	AdminSecondFactorDue time.Time
	LastSeenAt           time.Time
}

type Subject struct {
	UserID        uuid.UUID
	SessionID     uuid.UUID
	InstanceOwner bool
	Memberships   []Membership
	TokenID       uuid.UUID
	TokenScopes   []Action
}

func (s Subject) ViaToken() bool { return s.TokenID != uuid.Nil }

func (s Subject) RoleIn(organizationID uuid.UUID) (Role, bool) {
	for _, m := range s.Memberships {
		if m.OrganizationID == organizationID {
			return m.Role, true
		}
	}
	return "", false
}

type Resource struct {
	OrganizationID  uuid.UUID
	ProjectID       uuid.UUID
	ProjectBindings []Role
}

func Can(s Subject, action Action, r Resource) bool {
	if s.UserID == uuid.Nil || IsInstanceAction(action) {
		return false
	}
	if s.ViaToken() && (!IsTokenAction(action) || !slices.Contains(s.TokenScopes, action)) {
		return false
	}
	if action == SessionManageSelf {
		return r.OrganizationID == uuid.Nil
	}
	if r.OrganizationID == uuid.Nil {
		return false
	}
	role, ok := s.RoleIn(r.OrganizationID)
	if !ok {
		return false
	}
	if r.ProjectID != uuid.Nil {
		if !IsProjectAction(action) {
			return false
		}
		role = ProjectRole(role, r.ProjectBindings)
	}
	return Allows(role, action)
}

type InstanceSettings struct {
	AnyUserCreatesOrganizations bool
}

func CanInstance(s Subject, action Action, inst InstanceSettings) bool {
	if s.UserID == uuid.Nil || s.ViaToken() || !IsInstanceAction(action) {
		return false
	}
	switch action {
	case OrgCreate:
		return s.InstanceOwner || inst.AnyUserCreatesOrganizations
	case InstanceSettingsManage:
		return s.InstanceOwner
	default:
		return false
	}
}
