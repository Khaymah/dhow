package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

type seenPrefs struct {
	Saved            bool
	ReleaseNotesSeen *string
}

func markSeen(c *apptest.Client, version any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPut, Path: "/me/release-notes-seen", Body: map[string]any{"Version": version}})
}

func TestReleaseNotesSeenIsStoredPerUser(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()

	var got seenPrefs
	owner.Get("/me/preferences").Decode(t, &got)
	assert.Nil(t, got.ReleaseNotesSeen)

	res := markSeen(owner, "0.8.0")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, "preferences.releaseNotesSeen", res.Message)
	res.Decode(t, &got)
	require.NotNil(t, got.ReleaseNotesSeen)
	assert.Equal(t, "0.8.0", *got.ReleaseNotesSeen)
	assert.False(t, got.Saved, "marking the notes as seen does not count as saving the display preferences")

	again := seenPrefs{}
	other, _ := env.Login(d.Email, d.Password)
	other.Get("/me/preferences").Decode(t, &again)
	require.NotNil(t, again.ReleaseNotesSeen)
	assert.Equal(t, "0.8.0", *again.ReleaseNotesSeen, "another device sees the same state")

	require.Equal(t, http.StatusOK, markSeen(owner, "0.9.0-rc.1").Status)
	owner.Get("/me/preferences").Decode(t, &got)
	assert.Equal(t, "0.9.0-rc.1", *got.ReleaseNotesSeen)

	member := env.SeedSession(env.SeedUserWithoutMembership("seen-other@example.com", "a long password for seen"))
	var theirs seenPrefs
	member.Get("/me/preferences").Decode(t, &theirs)
	assert.Nil(t, theirs.ReleaseNotesSeen, "another user has not seen anything")
}

func TestReleaseNotesSeenRejectsAnythingButAVersion(t *testing.T) {
	env := apptest.New(t)
	owner, _ := env.SetupAndLogin()
	for _, v := range []any{"", "latest", "1.2", "1.2.3; DROP TABLE", "1.2.3\n", "v1.2.3"} {
		res := markSeen(owner, v)
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "%v: %s", v, string(res.Raw))
		require.NotNil(t, res.Error)
		require.NotEmpty(t, res.Error.Details)
		assert.Equal(t, "Version", res.Error.Details[0].Field)
		assert.Contains(t, res.Error.Details[0].Message, "رقم النسخة غير صالح")
	}
	assert.Equal(t, http.StatusBadRequest, markSeen(owner, 7).Status)
}

func TestReleaseNotesSeenNeedsASessionAndCSRF(t *testing.T) {
	env := apptest.New(t)
	owner, _ := env.SetupAndLogin()
	anonymous := env.Do(apptest.Request{Method: http.MethodPut, Path: "/me/release-notes-seen", Body: map[string]any{"Version": "1.0.0"}})
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, anonymous.Status)
	noToken := owner.Do(apptest.Request{
		Method: http.MethodPut, Path: "/me/release-notes-seen", Body: map[string]any{"Version": "1.0.0"},
		Headers: map[string]string{"X-CSRF-Token": ""},
	})
	assert.Equal(t, http.StatusForbidden, noToken.Status)
}
