package e2e_test

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	"github.com/Khaymah/dhow/internal/buildinfo"
	"github.com/Khaymah/dhow/internal/logging"
)

func canCreate(t *testing.T, c *apptest.Client) bool {
	t.Helper()
	res := c.Get("/me")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var me struct{ CanCreateOrganizations bool }
	res.Decode(t, &me)
	return me.CanCreateOrganizations
}

func TestASaveReachesEveryInstanceOnTheDatabaseWithoutARestart(t *testing.T) {
	first := apptest.New(t)
	owner, d, fresh := ownerWithTotp(t, first)
	second := apptest.NewWithOptions(t, apptest.Options{Pool: first.Pool, Clock: first.Clock})
	member := second.SeedSession(first.SeedUser(first.OrganizationID(d.OrgSlug), "dev@example.com", "a long password for dev", "developer"))
	require.False(t, canCreate(t, member))

	save(t, owner, "organizations", fresh(), map[string]any{"Creation": "any-user"})
	require.Eventually(t, func() bool { return canCreate(t, member) }, 5*time.Second, 20*time.Millisecond,
		"the second instance reloads on the notification")

	save(t, owner, "rate-limits", fresh(), map[string]any{"Public": 1234})
	save(t, owner, "passwords", fresh(), map[string]any{"BreachedCheck": false})
	save(t, owner, "logs", fresh(), map[string]any{"RetentionDays": 9})
	save(t, owner, "mail", fresh(), map[string]any{"Host": "127.0.0.1", "From": "dhow@example.com", "Tls": "none"})
	require.Eventually(t, func() bool {
		v := second.Wired.Settings.Current().Values
		return v.RateLimits.Public == 1234 && !v.BreachedPasswordCheck && v.Logs.RetentionDays == 9 && v.Mail.Enabled()
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, int64(1), second.Wired.Settings.Current().Version(settingspec.GroupMail))

	second.Breach.Add("a breached but long password")
	res := member.Do(apptest.Request{Method: http.MethodPut, Path: "/me/password", Body: map[string]string{
		"CurrentPassword": "a long password for dev", "NewPassword": "a breached but long password",
	}})
	assert.Equal(t, http.StatusOK, res.Status, "the check is off on the second instance too: %s", res.Raw)
}

type currentMigrations struct{}

func (currentMigrations) State(context.Context) (migrations.SchemaState, error) {
	return migrations.SchemaCurrent, nil
}

func build(t *testing.T, secret []byte, env settingspec.Raw, pool *pgxpool.Pool) error {
	t.Helper()
	publicURL, err := url.Parse(apptest.PublicOrigin)
	require.NoError(t, err)
	log := logging.New(&apptest.SyncBuffer{}, slog.LevelDebug)
	wired, err := app.Build(t.Context(), app.Infra{
		Config: config.Config{
			SecretKey: secret, PublicURL: publicURL, ListenAddr: config.DefaultListenAddr, Settings: env,
			Artifacts: config.Artifacts{BlobDir: apptest.PrivateDir(t)},
		},
		Log: log, Pool: pool, Migrations: currentMigrations{}, Keys: apptest.NewKeys(t, pool, log),
		Build: buildinfo.Info{Version: "test"}, Breach: &apptest.FakeBreach{}, HashParams: ptr(apptest.TestParams()),
	})
	if err == nil {
		wired.Close()
	}
	return err
}

func ptr[T any](v T) *T { return &v }

func TestAnInvalidSavedValueStopsTheStart(t *testing.T) {
	secret := randomKey(t)
	unopenable := append([]byte{keys.FormatVersion}, randomKey(t)...)
	unopenable = append(unopenable, randomKey(t)...)
	unopenable = append(unopenable, randomKey(t)...)
	tests := []struct {
		name    string
		group   string
		value   string
		secrets []byte
		env     settingspec.Raw
		want    string
	}{
		{"a port that is not a number", "mail", `{"Port": "abc"}`, nil, nil, "DHOW_SMTP_PORT"},
		{"retention out of range", "logs", `{"RetentionDays": 0}`, nil, nil, "DHOW_LOG_RETENTION_DAYS (from the database)"},
		{"a host without a sender", "mail", `{"Host": "smtp.example.com"}`, nil, nil, "DHOW_SMTP_FROM"},
		{"an unknown field", "logs", `{"Nope": 1}`, nil, nil, "logs.Nope"},
		{"an unknown group", "colors", `{}`, nil, nil, "colors"},
		{"a secret sealed under a data key this keyring does not hold", "mail", `{}`, unopenable, nil, "cannot be decrypted"},
		{"a valid row", "mail", `{"Host": "smtp.example.com", "From": "dhow@example.com"}`, nil, nil, ""},
		{"the environment completes a saved row", "mail", `{"Host": "smtp.example.com"}`, nil, settingspec.Raw{settingspec.EnvSMTPFrom: "dhow@example.com"}, ""},
		{"the environment does not hide a broken saved value", "logs", `{"RetentionDays": 0}`, nil, settingspec.Raw{settingspec.EnvLogRetentionDays: "7"}, "DHOW_LOG_RETENTION_DAYS (from the database)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := dbtest.NewPool(t)
			_, err := pool.Exec(t.Context(), `INSERT INTO "InstanceSettings" ("Group", "Value", "Secrets") VALUES ($1, $2::jsonb, $3)`,
				tt.group, tt.value, tt.secrets)
			require.NoError(t, err)
			err = build(t, secret, tt.env, pool)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.NotContains(t, err.Error(), "relay-credential")
		})
	}
}

func randomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestAFailedReloadFailsReadinessUntilTheNextSuccessfulReload(t *testing.T) {
	env := apptest.New(t)
	ready := func() apptest.Response { return env.Do(apptest.Request{Method: http.MethodGet, Path: "/health/ready"}) }
	res := ready()
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Contains(t, string(res.Raw), `{"Name":"settings","Ok":true}`)

	_, err := env.Pool.Exec(t.Context(), `INSERT INTO "InstanceSettings" ("Group", "Value") VALUES ('logs', '{"RetentionDays": 0}'::jsonb)`)
	require.NoError(t, err)
	_, err = env.Pool.Exec(t.Context(), `SELECT pg_notify('dhow_settings_changed', '')`)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return ready().Status == http.StatusServiceUnavailable }, 5*time.Second, 20*time.Millisecond,
		"stale settings never keep serving as ready")
	res = ready()
	assert.Contains(t, string(res.Raw), `{"Name":"settings","Ok":false}`)
	assert.NotContains(t, string(res.Raw), "RETENTION", "the response names the check without internal details")
	assert.Equal(t, 30, env.Wired.Settings.Current().Values.Logs.RetentionDays, "the previous values stay in effect")

	_, err = env.Pool.Exec(t.Context(), `UPDATE "InstanceSettings" SET "Value" = '{"RetentionDays": 9}'::jsonb WHERE "Group" = 'logs'`)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return ready().Status == http.StatusOK }, 10*time.Second, 50*time.Millisecond,
		"the retry picks up the fixed value without another notification")
	assert.Contains(t, string(ready().Raw), `{"Name":"settings","Ok":true}`)
	assert.Equal(t, 9, env.Wired.Settings.Current().Values.Logs.RetentionDays)
}
