package e2e_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

type meData struct {
	User struct {
		Id, Email, Name, Locale string
	}
	Memberships []struct {
		OrganizationId, OrganizationName, OrganizationSlug, Role string
	}
	CsrfToken string
	SessionId string
	ExpiresAt string
}

func TestSetupLoginMeLogout(t *testing.T) {
	env := apptest.New(t)

	before := env.Do(apptest.Request{Method: http.MethodGet, Path: "/meta"})
	assert.JSONEq(t, `{"Version":"test","Commit":"test","ServerBuild":{"Version":"test","Commit":"test"},"SetupRequired":true}`, string(before.Data))

	d := apptest.DefaultSetup()
	setup := env.Setup(d)
	require.Equal(t, http.StatusOK, setup.Status, string(setup.Raw))
	assert.True(t, setup.Success)
	assert.Equal(t, "identity.setup.completed", setup.Message)
	var created struct {
		User         struct{ Id, Email, Name, Locale string }
		Organization struct{ Id, Name, Slug string }
	}
	setup.Decode(t, &created)
	assert.Equal(t, "owner@example.com", created.User.Email, "the email is stored lowercased")
	assert.Equal(t, "Mona Owner", created.User.Name)
	assert.Equal(t, "ar", created.User.Locale)
	assert.Equal(t, "dhow-co", created.Organization.Slug)

	after := env.Do(apptest.Request{Method: http.MethodGet, Path: "/meta"})
	assert.JSONEq(t, `{"Version":"test","Commit":"test","ServerBuild":{"Version":"test","Commit":"test"},"SetupRequired":false}`, string(after.Data))

	anonymous := env.Do(apptest.Request{Method: http.MethodGet, Path: "/me"})
	assert.Equal(t, http.StatusUnauthorized, anonymous.Status)
	assert.Equal(t, "UNAUTHENTICATED", anonymous.Error.Code)

	client, login := env.Login("OWNER@example.com ", d.Password)
	require.Equal(t, http.StatusOK, login.Status, string(login.Raw))
	assert.Equal(t, "session.login.succeeded", login.Message)
	assert.NotEmpty(t, client.CSRFToken)
	assert.NotEmpty(t, client.Cookie)
	assert.Equal(t, "no-store", login.Header.Get("Cache-Control"))

	me := client.Get("/me")
	require.Equal(t, http.StatusOK, me.Status, string(me.Raw))
	var got meData
	me.Decode(t, &got)
	assert.Equal(t, created.User.Id, got.User.Id)
	assert.Equal(t, "owner@example.com", got.User.Email)
	require.Len(t, got.Memberships, 1)
	assert.Equal(t, "owner", got.Memberships[0].Role)
	assert.Equal(t, created.Organization.Id, got.Memberships[0].OrganizationId)
	assert.Equal(t, client.CSRFToken, got.CsrfToken, "/me returns the same CSRF token as login")
	assert.NotEmpty(t, got.SessionId)

	sessions := client.Get("/sessions")
	require.Equal(t, http.StatusOK, sessions.Status)
	var list []struct {
		Id, UserAgent, IpAddress string
		Current                  bool
	}
	sessions.Decode(t, &list)
	require.Len(t, list, 1)
	assert.True(t, list[0].Current)
	assert.Equal(t, got.SessionId, list[0].Id)
	assert.Equal(t, apptest.UserAgent, list[0].UserAgent)
	assert.Equal(t, "203.0.113.7", list[0].IpAddress)

	logout := env.Logout(client)
	require.Equal(t, http.StatusOK, logout.Status, string(logout.Raw))
	cleared := logout.Header.Get("Set-Cookie")
	assert.Contains(t, cleared, config.SessionCookieName+"=;")
	assert.Contains(t, cleared, "Max-Age=0")

	assert.Equal(t, http.StatusUnauthorized, client.Get("/me").Status, "revocation takes effect immediately")
	assert.Equal(t, http.StatusUnauthorized, client.Get("/sessions").Status)
}

func TestSetupIsOneShot(t *testing.T) {
	env := apptest.New(t)
	d := apptest.DefaultSetup()
	token := env.SetupToken()
	d.Token = token
	require.Equal(t, http.StatusOK, env.Setup(d).Status)

	again := env.Setup(d)
	assert.Equal(t, http.StatusConflict, again.Status)
	assert.Equal(t, "SETUP_COMPLETED", again.Error.Code)

	_, created, err := env.Wired.Identity.EnsureSetupToken(t.Context())
	require.NoError(t, err)
	assert.False(t, created, "no new token is minted once an owner exists")

	d.Email, d.OrgSlug = "second@example.com", "second"
	second := env.Setup(d)
	assert.Equal(t, http.StatusConflict, second.Status, "a second owner cannot be created with the old token either")
	assert.Equal(t, "SETUP_COMPLETED", second.Error.Code)
}

func TestLoginErrorsAreGeneric(t *testing.T) {
	env := apptest.New(t)
	_, d := env.SetupAndLogin()

	unknown := env.Do(apptest.Request{
		Method: http.MethodPost, Path: "/sessions", Headers: map[string]string{"Origin": apptest.PublicOrigin},
		Body: map[string]string{"Email": "nobody@example.com", "Password": "whatever whatever"},
	})
	wrong := env.Do(apptest.Request{
		Method: http.MethodPost, Path: "/sessions", Headers: map[string]string{"Origin": apptest.PublicOrigin},
		Body: map[string]string{"Email": d.Email, "Password": "not the password"},
	})
	malformed := env.Do(apptest.Request{
		Method: http.MethodPost, Path: "/sessions", Headers: map[string]string{"Origin": apptest.PublicOrigin},
		Body: map[string]string{"Email": "not-an-email", "Password": "whatever whatever"},
	})

	for name, res := range map[string]apptest.Response{"unknown": unknown, "wrong": wrong, "malformed": malformed} {
		assert.Equal(t, http.StatusUnauthorized, res.Status, name)
		assert.Equal(t, "INVALID_CREDENTIALS", res.Error.Code, name)
		assert.Empty(t, res.Header.Get("Set-Cookie"), name)
		assert.Empty(t, res.Header.Get("Retry-After"), name)
	}
	assert.JSONEq(t, string(unknown.Raw), string(wrong.Raw), "an unknown account and a wrong password are indistinguishable")
	assert.JSONEq(t, string(unknown.Raw), string(malformed.Raw))
}

func TestPasswordChange(t *testing.T) {
	env := apptest.New(t)
	first, d := env.SetupAndLogin()
	second, res := env.Login(d.Email, d.Password)
	require.Equal(t, http.StatusOK, res.Status)
	env.Breach.Add("this password is breached")

	change := func(current, next string) apptest.Response {
		return first.Do(apptest.Request{Method: http.MethodPut, Path: "/me/password", Body: map[string]string{"CurrentPassword": current, "NewPassword": next}})
	}
	tests := []struct {
		name     string
		current  string
		next     string
		status   int
		wantCode string
		field    string
	}{
		{"too short", d.Password, "short", 422, "VALIDATION_FAILED", "NewPassword"},
		{"eleven characters", d.Password, "elevenchars", 422, "VALIDATION_FAILED", "NewPassword"},
		{"too long", d.Password, strings.Repeat("a", 129), 422, "VALIDATION_FAILED", "NewPassword"},
		{"wrong current password", "not the current one", "a brand new passphrase", 403, "INVALID_CURRENT_PASSWORD", ""},
		{"unchanged", d.Password, d.Password, 422, "VALIDATION_FAILED", "NewPassword"},
		{"breached", d.Password, "this password is breached", 422, "PASSWORD_BREACHED", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := change(tt.current, tt.next)
			assert.Equal(t, tt.status, res.Status, string(res.Raw))
			require.NotNil(t, res.Error)
			assert.Equal(t, tt.wantCode, res.Error.Code)
			if tt.field != "" {
				require.NotEmpty(t, res.Error.Details)
				assert.Equal(t, tt.field, res.Error.Details[0].Field)
			}
		})
	}
	assert.Equal(t, http.StatusOK, second.Get("/me").Status, "failed changes revoke nothing")

	ok := change(d.Password, "a brand new passphrase")
	require.Equal(t, http.StatusOK, ok.Status, string(ok.Raw))
	var revoked struct{ Revoked int }
	ok.Decode(t, &revoked)
	assert.Equal(t, 1, revoked.Revoked)

	assert.Equal(t, http.StatusOK, first.Get("/me").Status, "the session that changed the password stays")
	assert.Equal(t, http.StatusUnauthorized, second.Get("/me").Status, "every other session is revoked at once")

	_, old := env.Login(d.Email, d.Password)
	assert.Equal(t, http.StatusUnauthorized, old.Status)
	_, fresh := env.Login(d.Email, "a brand new passphrase")
	assert.Equal(t, http.StatusOK, fresh.Status)
}

func TestPasswordChangeSurvivesAnUnavailableBreachService(t *testing.T) {
	env := apptest.New(t)
	client, d := env.SetupAndLogin()
	env.Breach.FailWith(assert.AnError)
	res := client.Do(apptest.Request{Method: http.MethodPut, Path: "/me/password", Body: map[string]string{
		"CurrentPassword": d.Password, "NewPassword": "another long passphrase",
	}})
	assert.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Contains(t, env.Logs.String(), "breached-password check unavailable")
}

func TestRevokeSessions(t *testing.T) {
	env := apptest.New(t)
	first, d := env.SetupAndLogin()
	second, _ := env.Login(d.Email, d.Password)
	third, _ := env.Login(d.Email, d.Password)

	var list []struct {
		Id      string
		Current bool
	}
	res := first.Get("/sessions")
	res.Decode(t, &list)
	require.Len(t, list, 3)

	var secondID string
	var secondMe meData
	second.Get("/me").Decode(t, &secondMe)
	secondID = secondMe.SessionId

	del := first.Do(apptest.Request{Method: http.MethodDelete, Path: "/sessions/" + secondID})
	require.Equal(t, http.StatusOK, del.Status, string(del.Raw))
	assert.Equal(t, http.StatusUnauthorized, second.Get("/me").Status)
	assert.Equal(t, http.StatusOK, third.Get("/me").Status)
	again := first.Do(apptest.Request{Method: http.MethodDelete, Path: "/sessions/" + secondID})
	assert.Equal(t, http.StatusNotFound, again.Status, "an already revoked session is gone")

	invalid := first.Do(apptest.Request{Method: http.MethodDelete, Path: "/sessions/not-a-uuid"})
	assert.Equal(t, http.StatusBadRequest, invalid.Status)

	others := first.Do(apptest.Request{Method: http.MethodPost, Path: "/sessions/revoke-others"})
	require.Equal(t, http.StatusOK, others.Status, string(others.Raw))
	var count struct{ Revoked int }
	others.Decode(t, &count)
	assert.Equal(t, 1, count.Revoked)
	assert.Equal(t, http.StatusUnauthorized, third.Get("/me").Status)
	assert.Equal(t, http.StatusOK, first.Get("/me").Status)

	none := first.Do(apptest.Request{Method: http.MethodPost, Path: "/sessions/revoke-others"})
	none.Decode(t, &count)
	assert.Equal(t, 0, count.Revoked)
}

func TestCannotTouchAnotherUsersSession(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	orgID := env.OrganizationID(d.OrgSlug)
	env.SeedUser(orgID, "dev@example.com", "developer password 1", "developer")
	dev, res := env.Login("dev@example.com", "developer password 1")
	require.Equal(t, http.StatusOK, res.Status)

	var ownerMe meData
	owner.Get("/me").Decode(t, &ownerMe)
	steal := dev.Do(apptest.Request{Method: http.MethodDelete, Path: "/sessions/" + ownerMe.SessionId})
	assert.Equal(t, http.StatusNotFound, steal.Status, "another user's session looks like it does not exist")
	assert.Equal(t, http.StatusOK, owner.Get("/me").Status)

	var devSessions []struct{ Id string }
	dev.Get("/sessions").Decode(t, &devSessions)
	require.Len(t, devSessions, 1)
	assert.NotEqual(t, ownerMe.SessionId, devSessions[0].Id)
}
