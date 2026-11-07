package app_test

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
)

const doctorPath = "/system/doctor"

func doctorOf(t *testing.T, res apptest.Response) gen.DoctorReport {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var report gen.DoctorReport
	res.Decode(t, &report)
	return report
}

func TestDoctorIsDeclaredForEverySignedInUser(t *testing.T) {
	var found bool
	for _, rule := range system.Policies() {
		if rule.Key() == http.MethodGet+" "+doctorPath {
			found = true
			assert.Equal(t, policy.ScopeAuthenticated, rule.Scope)
		}
	}
	assert.True(t, found, "GET %s has no row in the route table", doctorPath)
}

func TestDoctorShowsTheChecksToTheServerOwnerOnly(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	for _, p := range principals(fx) {
		t.Run(p.name, func(t *testing.T) {
			res := fx.client(env, p).Get(doctorPath)
			if p.kind == "anonymous" {
				assert.Equal(t, http.StatusUnauthorized, res.Status)
				return
			}
			report := doctorOf(t, res)
			assert.Equal(t, "test", report.ServerBuild.Version)
			assert.Equal(t, gen.HealthStatusStatus("ok"), report.Health.Status)
			if p.kind == "member" && p.role == policy.RoleOwner {
				require.NotNil(t, report.Checks)
				assert.NotEmpty(t, *report.Checks)
				return
			}
			assert.Nil(t, report.Checks, "only the instance owner sees the checks")
		})
	}
}

func TestDoctorAnswersInTheLanguageOfTheRequest(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	owner := env.SeedSession(mustUUID(fx.byRole[policy.RoleOwner]))
	titles := map[string]string{}
	for _, lang := range []string{"en", "ar"} {
		res := owner.Do(apptest.Request{Method: http.MethodGet, Path: doctorPath, Headers: map[string]string{"Accept-Language": lang}})
		report := doctorOf(t, res)
		require.NotNil(t, report.Checks)
		titles[lang] = (*report.Checks)[0].Title
	}
	assert.NotEmpty(t, titles["en"])
	assert.NotEqual(t, titles["en"], titles["ar"])
}

func TestDoctorLeaksNoSecretAndNoPath(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(0xA0 + i%16)
	}
	env := apptest.NewConfigured(t, nil, apptest.TestParams(), func(cfg *config.Config) { cfg.SecretKey = secret })
	fx := buildFixture(t, env)
	owner := env.SeedSession(mustUUID(fx.byRole[policy.RoleOwner]))
	res := owner.Get(doctorPath)
	report := doctorOf(t, res)
	require.NotNil(t, report.Checks)

	forbidden := []string{
		string(secret), hex.EncodeToString(secret), base64.StdEncoding.EncodeToString(secret), base64.RawURLEncoding.EncodeToString(secret),
		env.Config.Artifacts.BlobDir, config.TestDatabaseURL(), "/proc/", "/var/lib", "/etc/",
	}
	if u, err := url.Parse(config.TestDatabaseURL()); err == nil {
		forbidden = append(forbidden, u.Hostname())
		if pw, ok := u.User.Password(); ok && pw != "" {
			forbidden = append(forbidden, pw)
		}
	}
	for _, text := range forbidden {
		assert.NotContains(t, string(res.Raw), text)
	}
	assert.NotContains(t, string(res.Raw), `"Detail"`)
}
