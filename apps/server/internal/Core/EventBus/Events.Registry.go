package eventbus

import (
	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	SystemInstanceSetupCompleted = "system.instance.setupCompleted"
	SystemSettingsChanged        = "system.settings.changed"

	IdentityOrganizationUpdated = "identity.organization.updated"

	IdentityOrganizationCreated  = "identity.organization.created"
	IdentityOwnershipTransferred = "identity.organization.ownership_transferred"
	IdentitySecondFactorChanged  = "identity.organization.second_factor_changed"
	IdentityMemberLeft           = "identity.member.left"
	IdentityMemberRoleChanged    = "identity.member.role_changed"
	IdentityMemberRemoved        = "identity.member.removed"
	IdentityMemberMfaReset       = "identity.member.mfa_reset"
	IdentityTeamCreated          = "identity.team.created"
	IdentityTeamUpdated          = "identity.team.updated"
	IdentityTeamDeleted          = "identity.team.deleted"
	IdentityTeamMemberAdded      = "identity.team.member_added"
	IdentityTeamMemberRemoved    = "identity.team.member_removed"
	IdentityRoleBindingAdded     = "identity.team.role_binding_added"
	IdentityRoleBindingChanged   = "identity.team.role_binding_changed"
	IdentityRoleBindingRemoved   = "identity.team.role_binding_removed"
	IdentityInvitationCreated    = "identity.invitation.created"
	IdentityInvitationResent     = "identity.invitation.resent"
	IdentityInvitationRevoked    = "identity.invitation.revoked"
	IdentityInvitationAccepted   = "identity.invitation.accepted"

	AuthLogin                 = "auth.login"
	AuthLoginFailed           = "auth.login_failed"
	AuthLogout                = "auth.logout"
	AuthPasswordChanged       = "auth.password_changed"
	AuthSessionRevoked        = "auth.session_revoked"
	AuthSessionsRevokedOthers = "auth.sessions_revoked_others"

	AuthMfaPending                = "auth.mfa_pending"
	AuthMfaFailed                 = "auth.mfa_failed"
	AuthRecoveryCodeUsed          = "auth.recovery_code_used"
	AuthTotpEnabled               = "auth.totp_enabled"
	AuthTotpDisabled              = "auth.totp_disabled"
	AuthRecoveryCodesRegenerated  = "auth.recovery_codes_regenerated"
	AuthPasskeyAdded              = "auth.passkey_added"                //nolint:gosec // An audit action name, not a credential.
	AuthPasskeyRenamed            = "auth.passkey_renamed"              //nolint:gosec // An audit action name, not a credential.
	AuthPasskeyRemoved            = "auth.passkey_removed"              //nolint:gosec // An audit action name, not a credential.
	AuthPasskeySignCountRegressed = "auth.passkey_sign_count_regressed" //nolint:gosec // An audit action name, not a credential.

	AuthOidcLoginFailed  = "auth.oidc_login_failed"
	AuthIdentityLinked   = "auth.identity_linked"
	AuthIdentityUnlinked = "auth.identity_unlinked"
	AuthApiTokenCreated  = "auth.api_token_created" //nolint:gosec // An audit action name, not a credential.
	AuthApiTokenRevoked  = "auth.api_token_revoked" //nolint:gosec // An audit action name, not a credential.

	RunnerRegistered               = "runner.registered"
	RunnerUpdated                  = "runner.updated"
	RunnerRevoked                  = "runner.revoked"
	RunnerDisabledCloneSuspected   = "runner.disabled_clone_suspected"
	RunnerKeysRotated              = "runner.keys_rotated"
	RunnerRegistrationTokenCreated = "runner.registration_token_created"
	RunnerRegistrationTokenRevoked = "runner.registration_token_revoked"

	JobCreated         = "job.created"
	JobCancelRequested = "job.cancel_requested"

	ProjectCreated  = "project.created"
	ProjectUpdated  = "project.updated"
	ProjectArchived = "project.archived"

	ProjectRepositoryLinked     = "project.repository_linked"
	ProjectRepositoryUpdated    = "project.repository_updated"
	ProjectHostKeyConfirmed     = "project.host_key_confirmed"
	ProjectDeployKeyRotated     = "project.deploy_key_rotated"
	ProjectDeployKeyConfirmed   = "project.deploy_key_confirmed"
	ProjectWebhookSecretRotated = "project.webhook_secret_rotated"
	ProjectRepositoryUnlinked   = "project.repository_unlinked"

	RunCreated         = "project.run_created"
	RunStarted         = "project.run_started"
	RunCancelRequested = "project.run_cancel_requested"
	RunRerunRequested  = "project.run_rerun_requested"
	RunApproved        = "project.run_approved"

	EnvironmentCreated = "environment.created"
	EnvironmentUpdated = "environment.updated"
	EnvironmentRemoved = "environment.removed"
	EnvironmentPaused  = "environment.paused"
	EnvironmentResumed = "environment.resumed"
)

func AuditedEvents() []string {
	return []string{
		SystemInstanceSetupCompleted,
		SystemSettingsChanged,
		IdentityOrganizationUpdated,
		IdentityOrganizationCreated,
		IdentityOwnershipTransferred,
		IdentitySecondFactorChanged,
		IdentityMemberLeft,
		IdentityMemberRoleChanged,
		IdentityMemberRemoved,
		IdentityMemberMfaReset,
		IdentityTeamCreated,
		IdentityTeamUpdated,
		IdentityTeamDeleted,
		IdentityTeamMemberAdded,
		IdentityTeamMemberRemoved,
		IdentityRoleBindingAdded,
		IdentityRoleBindingChanged,
		IdentityRoleBindingRemoved,
		IdentityInvitationCreated,
		IdentityInvitationResent,
		IdentityInvitationRevoked,
		IdentityInvitationAccepted,
		AuthLogin,
		AuthLoginFailed,
		AuthLogout,
		AuthPasswordChanged,
		AuthSessionRevoked,
		AuthSessionsRevokedOthers,
		AuthMfaPending,
		AuthMfaFailed,
		AuthRecoveryCodeUsed,
		AuthTotpEnabled,
		AuthTotpDisabled,
		AuthRecoveryCodesRegenerated,
		AuthPasskeyAdded,
		AuthPasskeyRenamed,
		AuthPasskeyRemoved,
		AuthPasskeySignCountRegressed,
		AuthOidcLoginFailed,
		AuthIdentityLinked,
		AuthIdentityUnlinked,
		AuthApiTokenCreated,
		AuthApiTokenRevoked,
		RunnerRegistered,
		RunnerUpdated,
		RunnerRevoked,
		RunnerDisabledCloneSuspected,
		RunnerKeysRotated,
		RunnerRegistrationTokenCreated,
		RunnerRegistrationTokenRevoked,
		JobCreated,
		JobCancelRequested,
		ProjectCreated,
		ProjectUpdated,
		ProjectArchived,
		ProjectRepositoryLinked,
		ProjectRepositoryUpdated,
		ProjectHostKeyConfirmed,
		ProjectDeployKeyRotated,
		ProjectDeployKeyConfirmed,
		ProjectWebhookSecretRotated,
		ProjectRepositoryUnlinked,
		RunCreated,
		RunStarted,
		RunCancelRequested,
		RunRerunRequested,
		RunApproved,
		EnvironmentCreated,
		EnvironmentUpdated,
		EnvironmentRemoved,
		EnvironmentPaused,
		EnvironmentResumed,
	}
}

const (
	SeverityKey  = "Severity"
	SeverityHigh = "high"
)

type Audited struct {
	OrganizationIDs []uuid.UUID
	ActorUserID     *uuid.UUID
	TargetType      string
	TargetID        string
	IP              string
	Data            map[string]string
	Tx              *sqlc.Queries
}
