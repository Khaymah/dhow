package config

import "time"

const (
	DefaultListenAddr = "127.0.0.1:8080"
	MinSecretKeyBytes = 32
)

const (
	MaxBodyBytes = 1 << 20
	APITimeout   = 15 * time.Second
)

const (
	ReadHeaderTimeout = 5 * time.Second
	ReadTimeout       = 30 * time.Second
	WriteTimeout      = 60 * time.Second
	IdleTimeout       = 120 * time.Second
	MaxHeaderBytes    = 64 << 10
	ShutdownTimeout   = 20 * time.Second
)

const HealthcheckTimeout = 3 * time.Second

const (
	DatabaseMaxConns    = 20
	DatabaseMaxIdleTime = 5 * time.Minute
	DatabaseConnTimeout = 10 * time.Second
	DatabaseAppName     = "dhow-server"
)

const (
	APIPrefix = "/api/v1"

	SessionCookieName            = "__Host-dhow_session"
	CSRFHeaderName               = "X-CSRF-Token"
	SessionIdleTimeout           = 12 * time.Hour
	SessionPrivilegedIdleTimeout = 4 * time.Hour
	SessionAbsoluteTimeout       = 7 * 24 * time.Hour
	SessionTouchInterval         = time.Minute
	MembershipTouchInterval      = time.Minute
	SessionTokenBytes            = 32
	AccountSecurityPath          = "/?stack=account"
	SecondFactorGrace            = 7 * 24 * time.Hour
)

const (
	MfaCookieName           = "__Host-dhow_mfa"
	MfaPendingTTL           = 5 * time.Minute
	MfaMaxChallengeAttempts = 5
	MfaFreeAttempts         = 5
	TotpEnrollmentTTL       = 10 * time.Minute
	TotpIssuer              = "Dhow"
	TotpSealLabel           = "dhow/totp-secret/v1"
	WebAuthnChallengeTTL    = 5 * time.Minute
	WebAuthnDisplayName     = "Dhow"
	WebAuthnHandleBytes     = 32
	PasskeyNameMaxLength    = 64
	MfaCodeMaxBytes         = 64
)

const (
	OidcCookieName   = "__Host-dhow_oidc"
	OidcFlowTTL      = 10 * time.Minute
	OidcFlowKeyLabel = "dhow/oidc-flow/v1"
	OidcCallbackPath = "/oidc/callback/"

	OidcProvidersLockKey int64 = 0x64686f775f6f6964
)

const (
	APITokenPrefix        = "dhow_pat_"
	APITokenBytes         = 32
	APITokenKeyLabel      = "dhow/api-token/v1" //nolint:gosec // A key derivation label, not a credential.
	APITokenMaxPerUser    = 50
	APITokenMaxTTLDays    = 366
	APITokenNameMax       = 100
	APITokenListLimit     = 200
	APITokenTouchInterval = time.Minute
	APITokenMaxScopes     = 16
)

const (
	PreferencesBodyMaxBytes = 128 << 10
	MaxCustomThemes         = 50
	ThemeNameMax            = 40
	DefaultThemeID          = "sea"
)

const (
	SetupTokenTTL   = time.Hour
	SetupTokenBytes = 32
)

const (
	InvitationTTL        = 7 * 24 * time.Hour
	InvitationTokenBytes = 32
	InvitationListLimit  = 200
	MailTimeout          = 10 * time.Second
	AlertMaxPending      = 32
	TeamDescriptionMax   = 500
)

const (
	PasswordMinLength = 12
	PasswordMaxLength = 128
	AuthBodyMaxBytes  = 16 << 10
)

const (
	AccountFreeAttempts = 5
	IPFreeAttempts      = 20
	ThrottleBaseLock    = 30 * time.Second
	ThrottleMaxLock     = 15 * time.Minute
	ThrottleResetAfter  = time.Hour
)

const (
	BreachedCheckTimeout = 3 * time.Second
	BreachedRangeURL     = "https://api.pwnedpasswords.com/range/"
)

const (
	DefaultPageLimit = 25
	MaxPageLimit     = 100
	MaxPage          = 100_000
)

const (
	ServerHeartbeatEvery     = 30 * time.Second
	ServerInstanceStaleAfter = 3 * ServerHeartbeatEvery
	ServerRetireTimeout      = 5 * time.Second
)
