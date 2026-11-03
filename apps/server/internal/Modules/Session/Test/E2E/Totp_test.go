package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

func withTotp(t *testing.T) (*apptest.Env, *apptest.Client, apptest.SetupDetails, apptest.TotpEnrollment) {
	t.Helper()
	env := apptest.New(t)
	c, d := env.SetupAndLogin()
	enrolled := c.EnableTotp(apptest.PasswordFactor(d.Password))
	env.Clock.Advance(31 * time.Second)
	return env, c, d, enrolled
}

func TestEnablingTotpNeedsTheCodeAndThePasswordAndRevokesOtherSessions(t *testing.T) {
	env := apptest.New(t)
	c, d := env.SetupAndLogin()
	other, _ := env.Login(d.Email, d.Password)

	secret := c.StartTotp()
	assert.Len(t, secret, 20)

	wrongCode := c.ConfirmTotp(env.TotpCode(secret, 3), apptest.PasswordFactor(d.Password))
	assert.Equal(t, http.StatusUnprocessableEntity, wrongCode.Status, string(wrongCode.Raw))
	assert.Equal(t, "VALIDATION_FAILED", wrongCode.Error.Code)

	wrongPassword := c.ConfirmTotp(env.TotpCode(secret, 0), apptest.PasswordFactor("not the password at all"))
	assert.Equal(t, http.StatusForbidden, wrongPassword.Status)
	assert.Equal(t, "INVALID_CURRENT_FACTOR", wrongPassword.Error.Code)

	notAFactorYet := c.ConfirmTotp(env.TotpCode(secret, 0), apptest.CodeFactor("totp", env.TotpCode(secret, 0)))
	assert.Equal(t, "INVALID_CURRENT_FACTOR", notAFactorYet.Error.Code, "without a second factor only the password counts")

	ok := c.ConfirmTotp(env.TotpCode(secret, 0), apptest.PasswordFactor(d.Password))
	require.Equal(t, http.StatusOK, ok.Status, string(ok.Raw))
	assert.Equal(t, "mfa.totp.enabled", ok.Message)
	var change struct {
		Revoked       int
		RecoveryCodes []string
	}
	ok.Decode(t, &change)
	assert.Equal(t, 1, change.Revoked)
	assert.Len(t, change.RecoveryCodes, 10, "the first second factor comes with recovery codes")

	assert.Equal(t, http.StatusUnauthorized, other.Get("/me").Status, "every other session is revoked")
	assert.Equal(t, http.StatusOK, c.Get("/me").Status, "the current session stays")

	var status struct {
		Enabled, TotpEnabled   bool
		RecoveryCodesRemaining int
		PasskeyCount           int
		TotpEnabledAt          string
	}
	c.Get("/me/mfa").Decode(t, &status)
	assert.True(t, status.Enabled)
	assert.True(t, status.TotpEnabled)
	assert.Equal(t, 10, status.RecoveryCodesRemaining)
	assert.NotEmpty(t, status.TotpEnabledAt)
	assert.Equal(t, 1, env.AuditCount("auth.totp_enabled"))
}

func TestTheSecretIsStoredSealedAndNeverLogged(t *testing.T) {
	env, _, _, enrolled := withTotp(t)
	var sealed []byte
	require.NoError(t, env.Pool.QueryRow(context.Background(),
		`SELECT "SecretSealed" FROM "TotpFactors" WHERE "ConfirmedAt" IS NOT NULL AND NOT "IsDeleted"`).Scan(&sealed))
	assert.NotContains(t, string(sealed), string(enrolled.Secret))
	assert.Len(t, sealed, keys.Overhead+20, "the envelope format: 97 bytes, inside the CHECK of 128")
	assert.Equal(t, keys.FormatVersion, sealed[0])
	var hashes int
	require.NoError(t, env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "RecoveryCodes" WHERE octet_length("CodeHash") = 32`).Scan(&hashes))
	assert.Equal(t, 10, hashes)
	logs := env.Logs.String()
	for _, code := range enrolled.RecoveryCodes {
		assert.NotContains(t, logs, code)
		assert.NotContains(t, logs, strings.ReplaceAll(code, "-", ""))
	}
	assert.NotContains(t, logs, auth.TotpSecretText(enrolled.Secret))
}

func TestSignInWithTotpTakesTwoSteps(t *testing.T) {
	env, _, d, enrolled := withTotp(t)

	step := env.PasswordStep(d.Email, d.Password, nil)
	require.Equal(t, http.StatusAccepted, step.Status, string(step.Raw))
	assert.Equal(t, "session.mfa.required", step.Message)
	var pending struct {
		Methods   []string
		ExpiresAt time.Time
	}
	step.Decode(t, &pending)
	assert.ElementsMatch(t, []string{"totp", "recovery_code"}, pending.Methods)
	assert.WithinDuration(t, env.Clock.Now().Add(5*time.Minute), pending.ExpiresAt, time.Second)

	_, _, hasSession := apptest.SetCookie(step.Header, config.SessionCookieName)
	assert.False(t, hasSession, "the password alone never yields a session")
	cookie, raw, ok := apptest.SetCookie(step.Header, config.MfaCookieName)
	require.True(t, ok)
	for _, attr := range []string{"HttpOnly", "Secure", "SameSite=Strict", "Path=/", "Max-Age=300"} {
		assert.Contains(t, raw, attr)
	}
	assert.NotContains(t, strings.ToLower(raw), "domain=")

	asSession := env.Do(apptest.Request{Method: http.MethodGet, Path: "/me", Headers: map[string]string{"Cookie": config.SessionCookieName + "=" + cookie}})
	assert.Equal(t, http.StatusUnauthorized, asSession.Status, "the challenge is not a session")

	c, done := env.CompleteMfa(cookie, map[string]string{"Method": "totp", "Code": env.TotpCode(enrolled.Secret, 0)}, nil)
	require.Equal(t, http.StatusOK, done.Status, string(done.Raw))
	assert.Equal(t, http.StatusOK, c.Get("/me").Status)
	_, cleared, ok := apptest.SetCookie(done.Header, config.MfaCookieName)
	require.True(t, ok)
	assert.Contains(t, cleared, "Max-Age=0")
	assert.Equal(t, 1, env.AuditCount("auth.mfa_pending"))
	var method string
	require.NoError(t, env.Pool.QueryRow(context.Background(),
		`SELECT "Data"->>'Method' FROM "AuditEvents" WHERE "Action" = 'auth.login' ORDER BY "Seq" DESC LIMIT 1`).Scan(&method))
	assert.Equal(t, "totp", method)
}

func TestTotpCodesCannotBeReplayedOrComeFromTheWrongStep(t *testing.T) {
	env, _, d, enrolled := withTotp(t)
	code := env.TotpCode(enrolled.Secret, 0)
	_, first := env.CompleteMfa(env.StartMfa(d.Email, d.Password), map[string]string{"Method": "totp", "Code": code}, nil)
	require.Equal(t, http.StatusOK, first.Status, string(first.Raw))

	used := auth.TotpStep(env.Clock.Now())
	tests := []struct {
		name string
		code func() string
	}{
		{"the same code again", func() string { return code }},
		{"the previous step after a newer one was used", func() string { return auth.TotpCodeAt(enrolled.Secret, used-1) }},
		{"two steps back", func() string { return env.TotpCode(enrolled.Secret, -2) }},
		{"two steps ahead", func() string { return env.TotpCode(enrolled.Secret, 2) }},
		{"not a code", func() string { return "12ab56" }},
		{"too long", func() string { return strings.Repeat("1", 65) }},
	}
	for _, tt := range tests {
		cookie := env.StartMfa(d.Email, d.Password)
		_, res := env.CompleteMfa(cookie, map[string]string{"Method": "totp", "Code": tt.code()}, nil)
		assert.Equal(t, http.StatusUnauthorized, res.Status, tt.name)
		require.NotNil(t, res.Error, tt.name)
		assert.Equal(t, "INVALID_MFA_CODE", res.Error.Code, tt.name)
		assert.Empty(t, res.Header.Get("Retry-After"), tt.name)
		env.Clock.Advance(time.Minute)
	}
}

func TestTheChallengeExpiresWorksOnceAndStaysWithItsBrowser(t *testing.T) {
	env, _, d, enrolled := withTotp(t)
	good := func() map[string]string {
		return map[string]string{"Method": "totp", "Code": env.TotpCode(enrolled.Secret, 0)}
	}

	expired := env.StartMfa(d.Email, d.Password)
	env.Clock.Advance(5*time.Minute + time.Second)
	_, res := env.CompleteMfa(expired, good(), nil)
	assert.Equal(t, http.StatusUnauthorized, res.Status)
	assert.Equal(t, "MFA_CHALLENGE_EXPIRED", res.Error.Code)

	_, res = env.CompleteMfa("", good(), nil)
	assert.Equal(t, "MFA_CHALLENGE_EXPIRED", res.Error.Code, "no cookie")
	_, res = env.CompleteMfa("forged-cookie-value", good(), nil)
	assert.Equal(t, "MFA_CHALLENGE_EXPIRED", res.Error.Code, "a forged cookie")

	other := env.StartMfa(d.Email, d.Password)
	_, res = env.CompleteMfa(other, good(), map[string]string{"User-Agent": "a different browser"})
	assert.Equal(t, "MFA_CHALLENGE_EXPIRED", res.Error.Code, "the challenge is bound to the browser that passed the password")

	_, res = env.CompleteMfa(other, good(), nil)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	env.Clock.Advance(31 * time.Second)
	_, res = env.CompleteMfa(other, good(), nil)
	assert.Equal(t, "MFA_CHALLENGE_EXPIRED", res.Error.Code, "a used challenge never works again")

	missingHeader := env.Do(apptest.Request{
		Method: http.MethodPost, Path: "/sessions/mfa", Body: good(),
		Headers: map[string]string{"Cookie": config.MfaCookieName + "=" + other},
	})
	assert.Equal(t, "CSRF_FAILED", missingHeader.Error.Code, "the second step checks the Origin like the first")
}

func TestAChallengeCannotFinishAnotherUsersSignIn(t *testing.T) {
	env, owner, d, enrolled := withTotp(t)
	orgID := env.OrganizationID(d.OrgSlug)
	env.SeedUser(orgID, "second@example.com", "second user passphrase", "admin")
	second, _ := env.Login("second@example.com", "second user passphrase")
	secondTotp := second.EnableTotp(apptest.PasswordFactor("second user passphrase"))
	_ = owner
	env.Clock.Advance(31 * time.Second)

	ownersChallenge := env.StartMfa(d.Email, d.Password)
	_, res := env.CompleteMfa(ownersChallenge, map[string]string{"Method": "totp", "Code": env.TotpCode(secondTotp.Secret, 0)}, nil)
	assert.Equal(t, "INVALID_MFA_CODE", res.Error.Code, "another user's code never completes this user's sign-in")
	_, res = env.CompleteMfa(ownersChallenge, map[string]string{"Method": "recovery_code", "Code": secondTotp.RecoveryCodes[0]}, nil)
	assert.Equal(t, "INVALID_MFA_CODE", res.Error.Code, "nor does another user's recovery code")

	c, res := env.CompleteMfa(ownersChallenge, map[string]string{"Method": "totp", "Code": env.TotpCode(enrolled.Secret, 0)}, nil)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var me meData
	c.Get("/me").Decode(t, &me)
	assert.Equal(t, "owner@example.com", me.User.Email)
}

func TestRecoveryCodesWorkOnceAndAreAudited(t *testing.T) {
	env, _, d, enrolled := withTotp(t)
	code := strings.ToLower(enrolled.RecoveryCodes[3])
	c, res := env.CompleteMfa(env.StartMfa(d.Email, d.Password), map[string]string{"Method": "recovery_code", "Code": code}, nil)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, http.StatusOK, c.Get("/me").Status)
	assert.Equal(t, 1, env.AuditCount("auth.recovery_code_used"))
	var remaining string
	require.NoError(t, env.Pool.QueryRow(context.Background(),
		`SELECT "Data"->>'Remaining' FROM "AuditEvents" WHERE "Action" = 'auth.recovery_code_used'`).Scan(&remaining))
	assert.Equal(t, "9", remaining)

	_, again := env.CompleteMfa(env.StartMfa(d.Email, d.Password), map[string]string{"Method": "recovery_code", "Code": code}, nil)
	assert.Equal(t, "INVALID_MFA_CODE", again.Error.Code, "a used recovery code is refused")
}
