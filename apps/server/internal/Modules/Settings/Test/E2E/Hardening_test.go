package e2e_test

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func seededPool(t *testing.T, group, value string, secrets []byte) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.NewPool(t)
	_, err := pool.Exec(t.Context(), `INSERT INTO "InstanceSettings" ("Group", "Value", "Secrets") VALUES ($1, $2::jsonb, $3)`, group, value, secrets)
	require.NoError(t, err)
	return pool
}

func withEnv(pool *pgxpool.Pool, vars map[string]string) apptest.Options {
	return apptest.Options{Pool: pool, Profile: func(c *config.Config) {
		for k, v := range vars {
			c.SetEnvSetting(k, v)
		}
	}}
}

func detailFields(res apptest.Response) []string {
	out := []string{}
	if res.Error == nil {
		return out
	}
	for _, d := range res.Error.Details {
		out = append(out, d.Field)
	}
	return out
}

func TestAPasswordFromTheEnvironmentLocksTheServerItGoesTo(t *testing.T) {
	pool := seededPool(t, "mail", `{"Host": "smtp.example.com", "From": "dhow@example.com"}`, nil)
	env := apptest.NewWithOptions(t, withEnv(pool, map[string]string{
		settingspec.EnvSMTPUsername: "dhow", settingspec.EnvSMTPPassword: relaySecret,
	}))
	owner, _, fresh := ownerWithTotp(t, env)
	v := readSettings(t, owner)
	for _, field := range []string{"Host", "Port", "Tls"} {
		assert.True(t, v.field(t, "Mail", field).Locked, field)
	}
	assert.Equal(t, "database", v.field(t, "Mail", "Host").Source, "the value still comes from the database")
	assert.Contains(t, string(owner.Get("/instance/settings").Raw), `"LockedBy":"DHOW_SMTP_PASSWORD"`)
	for field, value := range map[string]any{"Host": "attacker.example.net", "Port": 2525, "Tls": "tls"} {
		res := put(owner, "mail", v.version("Mail"), fresh(), map[string]any{field: value})
		require.Equal(t, http.StatusConflict, res.Status, string(res.Raw))
		assert.Equal(t, "SETTING_LOCKED_BY_ENV", errorCode(res))
		assert.Equal(t, []string{field}, detailFields(res))
	}
	assert.Equal(t, "smtp.example.com", env.Wired.Settings.Current().Values.Mail.Host)
	saved := save(t, owner, "mail", fresh(), map[string]any{"From": "Dhow <dhow@example.com>"})
	assert.Equal(t, "Dhow <dhow@example.com>", saved.field(t, "Mail", "From").text(t))
}

func TestChangingTheServerAsksForTheSavedPasswordAgain(t *testing.T) {
	env := apptest.New(t)
	owner, _, fresh := ownerWithTotp(t, env)
	save(t, owner, "mail", fresh(), map[string]any{
		"Host": "smtp.example.com", "From": "dhow@example.com", "Username": "dhow", "Password": relaySecret,
	})
	version := readSettings(t, owner).version("Mail")
	res := put(owner, "mail", version, fresh(), map[string]any{"Host": "other.example.net"})
	require.Equal(t, http.StatusBadRequest, res.Status, string(res.Raw))
	assert.Equal(t, "SETTING_INVALID", errorCode(res))
	assert.Equal(t, []string{"Password"}, detailFields(res))
	assert.Equal(t, "smtp.example.com", env.Wired.Settings.Current().Values.Mail.Host)

	save(t, owner, "mail", fresh(), map[string]any{"Port": 2525})
	v := save(t, owner, "mail", fresh(), map[string]any{"Host": "other.example.net", "Password": "a-new-relay-credential"})
	assert.Equal(t, "other.example.net", v.field(t, "Mail", "Host").text(t))
	assert.Equal(t, "a-new-relay-credential", env.Wired.Settings.Current().Values.Mail.Password)
}

func TestTheDashboardSendsMailWithoutTLSOnlyToThisMachine(t *testing.T) {
	env := apptest.New(t)
	owner, _, fresh := ownerWithTotp(t, env)
	for _, host := range []string{"relay.internal", "10.0.0.25", "smtp.example.com"} {
		res := put(owner, "mail", readSettings(t, owner).version("Mail"), fresh(), map[string]any{"Host": host, "From": "dhow@example.com", "Tls": "none"})
		require.Equal(t, http.StatusBadRequest, res.Status, host)
		assert.Equal(t, []string{"Tls"}, detailFields(res), host)
	}
	for _, host := range []string{"127.0.0.1", "localhost"} {
		v := save(t, owner, "mail", fresh(), map[string]any{"Host": host, "From": "dhow@example.com", "Tls": "none"})
		assert.Equal(t, "none", v.field(t, "Mail", "Tls").text(t), host)
	}
	inEnv := apptest.NewWithOptions(t, withEnv(nil, map[string]string{
		settingspec.EnvSMTPHost: "relay.internal", settingspec.EnvSMTPTLS: "none", settingspec.EnvSMTPFrom: "dhow@example.com",
	}))
	assert.Equal(t, "none", inEnv.Wired.Settings.Current().Values.Mail.TLS, "the environment keeps working for existing installs")
}

func TestASecretSetInTheEnvironmentSurvivesANewSecretKey(t *testing.T) {
	sealed := append([]byte{keys.FormatVersion}, randomKey(t)...)
	sealed = append(sealed, randomKey(t)...)
	sealed = append(sealed, randomKey(t)...)
	pool := seededPool(t, "mail", `{"Host": "smtp.example.com", "From": "dhow@example.com", "Username": "dhow"}`, sealed)
	require.Error(t, build(t, randomKey(t), nil, pool), "without the variable the start fails closed")

	env := apptest.NewWithOptions(t, withEnv(pool, map[string]string{settingspec.EnvSMTPPassword: relaySecret}))
	assert.Equal(t, relaySecret, env.Wired.Settings.Current().Values.Mail.Password)
	owner, _, fresh := ownerWithTotp(t, env)
	password := readSettings(t, owner).field(t, "Mail", "Password")
	assert.True(t, password.Locked)
	assert.Contains(t, string(owner.Get("/instance/settings").Raw), `"Saved":true`)

	save(t, owner, "mail", fresh(), map[string]any{"From": "Dhow <dhow@example.com>"})
	var kept []byte
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "Secrets" FROM "InstanceSettings" WHERE "Group" = 'mail'`).Scan(&kept))
	assert.Equal(t, sealed, kept, "a save of other fields keeps the sealed copy untouched")

	save(t, owner, "mail", fresh(), map[string]any{"Password": ""})
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "Secrets" FROM "InstanceSettings" WHERE "Group" = 'mail'`).Scan(&kept))
	assert.Empty(t, kept, "the unused copy is cleared from the dashboard")
	assert.Contains(t, string(owner.Get("/instance/settings").Raw), `"Saved":false`)
}
