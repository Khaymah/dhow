package apperrors

const (
	CodeInternal           = "INTERNAL"
	CodeNotFound           = "NOT_FOUND"
	CodeMethodNotAllowed   = "METHOD_NOT_ALLOWED"
	CodeBadRequest         = "BAD_REQUEST"
	CodeValidationFailed   = "VALIDATION_FAILED"
	CodePayloadTooLarge    = "PAYLOAD_TOO_LARGE"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

const (
	CodeUnauthenticated        = "UNAUTHENTICATED"
	CodeForbidden              = "FORBIDDEN"
	CodeCSRFFailed             = "CSRF_FAILED"
	CodeInvalidCredentials     = "INVALID_CREDENTIALS"      //nolint:gosec // A public error code, not a credential.
	CodeInvalidCurrentPassword = "INVALID_CURRENT_PASSWORD" //nolint:gosec // A public error code, not a credential.
	CodePasswordBreached       = "PASSWORD_BREACHED"
	CodeSetupCompleted         = "SETUP_COMPLETED"
	CodeSetupTokenInvalid      = "SETUP_TOKEN_INVALID" //nolint:gosec // A public error code, not a credential.
	CodeConflict               = "CONFLICT"
	CodeMfaChallengeExpired    = "MFA_CHALLENGE_EXPIRED"
	CodeInvalidMfaCode         = "INVALID_MFA_CODE"
	CodeInvalidCurrentFactor   = "INVALID_CURRENT_FACTOR"
	CodeEnrollmentExpired      = "ENROLLMENT_EXPIRED"
	CodePasskeyRejected        = "PASSKEY_REJECTED"
	CodeRunnerRevoked          = "RUNNER_REVOKED"
	CodeSecretsUnavailable     = "SECRETS_UNAVAILABLE"
	CodeJobFinished            = "JOB_FINISHED"
	CodeJobQueueFull           = "JOB_QUEUE_FULL"
	CodeAttemptNotFound        = "ATTEMPT_NOT_FOUND"
	CodeTooManyStreams         = "TOO_MANY_STREAMS"
	CodeRateLimited            = "RATE_LIMITED"
)

const (
	CodeLastOwner               = "LAST_OWNER"
	CodeRoleNotAllowed          = "ROLE_NOT_ALLOWED"
	CodeAlreadyMember           = "ALREADY_MEMBER"
	CodeInvitationPending       = "INVITATION_PENDING"
	CodeInvitationInvalid       = "INVITATION_INVALID"
	CodeInvitationEmailMismatch = "INVITATION_EMAIL_MISMATCH"
	CodeInvitationAccountExists = "INVITATION_ACCOUNT_EXISTS"
	CodeInvitationSendLimit     = "INVITATION_SEND_LIMIT"
)

const (
	CodeOidcFlowInvalid         = "OIDC_FLOW_INVALID"
	CodeOidcSignInFailed        = "OIDC_SIGN_IN_FAILED"
	CodeOidcNotLinked           = "OIDC_NOT_LINKED"
	CodeOidcDomainNotAllowed    = "OIDC_DOMAIN_NOT_ALLOWED"
	CodeOidcProviderUnavailable = "OIDC_PROVIDER_UNAVAILABLE"
	CodeOidcIdentityInUse       = "OIDC_IDENTITY_IN_USE"
	CodeOidcAlreadyLinked       = "OIDC_ALREADY_LINKED"
	CodeLastSignInMethod        = "LAST_SIGN_IN_METHOD"
	CodeSessionRequired         = "SESSION_REQUIRED"
	CodeApiTokenLimit           = "API_TOKEN_LIMIT" //nolint:gosec // A public error code, not a credential.
	CodeScopeNotAllowed         = "SCOPE_NOT_ALLOWED"
	CodeSecondFactorRequired    = "SECOND_FACTOR_REQUIRED"
)

const (
	CodeProjectArchived      = "PROJECT_ARCHIVED"
	CodeForkPolicyNotAllowed = "FORK_POLICY_NOT_ALLOWED"
	CodeForkCacheNotAllowed  = "FORK_CACHE_NOT_ALLOWED"
	KeyProjectArchived       = "errors.projectArchived"
	KeyForkPolicyNotAllowed  = "errors.forkPolicyNotAllowed"
	KeyForkCacheNotAllowed   = "errors.forkCacheNotAllowed"
)

const (
	KeyInternal           = "errors.internal"
	KeyNotFound           = "errors.notFound"
	KeyMethodNotAllowed   = "errors.methodNotAllowed"
	KeyBadRequest         = "errors.badRequest"
	KeyValidationFailed   = "errors.validationFailed"
	KeyPayloadTooLarge    = "errors.payloadTooLarge"
	KeyServiceUnavailable = "errors.serviceUnavailable"
)

const (
	KeyUnauthenticated        = "errors.unauthenticated"
	KeyForbidden              = "errors.forbidden"
	KeyCSRFFailed             = "errors.csrfFailed"
	KeyInvalidCredentials     = "errors.invalidCredentials"
	KeyInvalidCurrentPassword = "errors.invalidCurrentPassword"
	KeyPasswordBreached       = "errors.passwordBreached"
	KeySetupCompleted         = "errors.setupCompleted"
	KeySetupTokenInvalid      = "errors.setupTokenInvalid"
	KeyConflict               = "errors.conflict"
	KeyMfaChallengeExpired    = "errors.mfaChallengeExpired"
	KeyInvalidMfaCode         = "errors.invalidMfaCode"
	KeyInvalidCurrentFactor   = "errors.invalidCurrentFactor"
	KeyEnrollmentExpired      = "errors.enrollmentExpired"
	KeyPasskeyRejected        = "errors.passkeyRejected"
	KeyRunnerRevoked          = "errors.runnerRevoked"
	KeySecretsUnavailable     = "errors.secretsUnavailable"
	KeyJobFinished            = "errors.jobFinished"
	KeyJobQueueFull           = "errors.jobQueueFull"
	KeyAttemptNotFound        = "errors.attemptNotFound"
	KeyTooManyStreams         = "errors.tooManyStreams"
	KeyRateLimited            = "errors.rateLimited"
)

const (
	KeyLastOwner               = "errors.lastOwner"
	KeyRoleNotAllowed          = "errors.roleNotAllowed"
	KeyAlreadyMember           = "errors.alreadyMember"
	KeyInvitationPending       = "errors.invitationPending"
	KeyInvitationInvalid       = "errors.invitationInvalid"
	KeyInvitationEmailMismatch = "errors.invitationEmailMismatch"
	KeyInvitationAccountExists = "errors.invitationAccountExists"
	KeyInvitationSendLimit     = "errors.invitationSendLimit"
)

const (
	KeyOidcFlowInvalid         = "errors.oidcFlowInvalid"
	KeyOidcSignInFailed        = "errors.oidcSignInFailed"
	KeyOidcNotLinked           = "errors.oidcNotLinked"
	KeyOidcDomainNotAllowed    = "errors.oidcDomainNotAllowed"
	KeyOidcProviderUnavailable = "errors.oidcProviderUnavailable"
	KeyOidcIdentityInUse       = "errors.oidcIdentityInUse"
	KeyOidcAlreadyLinked       = "errors.oidcAlreadyLinked"
	KeyLastSignInMethod        = "errors.lastSignInMethod"
	KeySessionRequired         = "errors.sessionRequired"
	KeyApiTokenLimit           = "errors.apiTokenLimit"
	KeyScopeNotAllowed         = "errors.scopeNotAllowed"
	KeySecondFactorRequired    = "errors.secondFactorRequired"
)
