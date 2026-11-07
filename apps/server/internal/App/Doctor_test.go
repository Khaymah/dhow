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

func instanceOwnerOf(t *testing.T, c *apptest.Client) bool {
	t.Helper()
	res := c.Get("/me")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var me struct{ InstanceOwner bool }
	res.Decode(t, &me)
	return me.InstanceOwner
}

func TestDoctorChecksNeedTheInstanceOwnerNotJustAnOrganizationOwner(t *testing.T) {
	env := apptest.New(t)
	fx := buildFixture(t, env)
	instanceOwner := env.SeedSession(mustUUID(fx.byRole[policy.RoleOwner]))
	require.True(t, instanceOwnerOf(t, instanceOwner), "the fixture owner must be the instance owner, or this test proves nothing")

	secondOwner := env.SeedSession(env.SeedUser(mustUUID(fx.orgA), "second-owner@example.com", "a long password for second owner", "owner"))
	otherOrgOwner := env.SeedSession(mustUUID(fx.outsiderID))
	for name, c := range map[string]*apptest.Client{"second owner of the same organization": secondOwner, "owner of another organization": otherOrgOwner} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, instanceOwnerOf(t, c))
			report := doctorOf(t, c.Get(doctorPath))
			assert.Nil(t, report.Checks)
		})
	}
	assert.NotNil(t, doctorOf(t, instanceOwner.Get(doctorPath)).Checks)
}

func TestDoctorRendersFixSentencesWithoutLeaking(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(0xB0 + i%16)
	}
	env := apptest.NewConfigured(t, nil, apptest.TestParams(), func(cfg *config.Config) { cfg.SecretKey = secret })
	fx := buildFixture(t, env)
	_, err := env.Pool.Exec(t.Context(), `UPDATE "KeyEncryptionKeys" SET "CheckValue" = decode(repeat('00', 32), 'hex')`)
	require.NoError(t, err)
	owner := env.SeedSession(mustUUID(fx.byRole[policy.RoleOwner]))
	member := env.SeedSession(mustUUID(fx.byRole[policy.RoleViewer]))

	ownerRes := owner.Get(doctorPath)
	report := doctorOf(t, ownerRes)
	require.NotNil(t, report.Checks)
	var fixes int
	for _, check := range *report.Checks {
		if check.Fix != nil {
			fixes++
			assert.NotEmpty(t, *check.Fix)
		}
	}
	require.Positive(t, fixes, "a failing check must carry a Fix")
	memberRes := member.Get(doctorPath)
	assert.Nil(t, doctorOf(t, memberRes).Checks)

	forbidden := []string{
		string(secret), hex.EncodeToString(secret), base64.StdEncoding.EncodeToString(secret), base64.RawURLEncoding.EncodeToString(secret),
		env.Config.Artifacts.BlobDir, config.TestDatabaseURL(), "/proc/", "KeyEncryptionKeys", "CheckValue", "SQLSTATE",
	}
	for _, res := range []apptest.Response{ownerRes, memberRes} {
		for _, text := range forbidden {
			assert.NotContains(t, string(res.Raw), text)
		}
	}
}
