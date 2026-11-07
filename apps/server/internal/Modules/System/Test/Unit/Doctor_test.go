package unit_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	installer "github.com/Khaymah/dhow/apps/server/internal/Core/Installer"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
	support "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Test/Support"
	"github.com/Khaymah/dhow/internal/buildinfo"
	"github.com/Khaymah/dhow/internal/installer/fakesys"
)

const (
	privateStateDir = "/srv/dhow-private-state"
	leakyDoctorErr  = `dial tcp 10.0.0.5:5432: password authentication failed for user "dhow" password=hunter2`
)

var (
	doctorNow   = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	builtAt     = time.Date(2026, 11, 6, 9, 30, 0, 0, time.UTC)
	doctorBuild = buildinfo.Info{Version: "v1.2.3", Commit: "abc123", BuiltAt: builtAt}
)

type fixedClock struct {
	at  time.Time
	err error
}

func (c fixedClock) GetDatabaseTime(context.Context) (time.Time, error) { return c.at, c.err }

func doctorController(db support.FakeDB, clock system.ClockSource) *system.Controller {
	log := support.DiscardLogger()
	health := system.NewHealthService(db, support.FakeMigrations{}, log)
	return system.NewController(health, system.NewMetaService(support.FakeQueries{}, doctorBuild)).
		WithDoctor(system.NewDoctorService(health, doctorBuild, system.DoctorOptions{
			Host: fakesys.New(), StateDir: privateStateDir, Clock: clock, Now: func() time.Time { return doctorNow },
		}))
}

func asUser(lang i18n.Locale, subject policy.Subject) context.Context {
	ctx := i18n.WithLocale(context.Background(), lang)
	return auth.WithPrincipal(ctx, auth.Principal{Subject: subject})
}

func ownerSubject() policy.Subject {
	return policy.Subject{UserID: uuid.New(), InstanceOwner: true}
}

func memberSubject() policy.Subject {
	return policy.Subject{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: uuid.New(), Role: policy.RoleAdmin}}}
}

func doctorData(t *testing.T, ctx context.Context, c *system.Controller) gen.DoctorReport {
	t.Helper()
	res, err := c.GetDoctor(ctx, gen.GetDoctorRequestObject{})
	require.NoError(t, err)
	body, ok := res.(gen.GetDoctor200JSONResponse)
	require.True(t, ok)
	require.True(t, body.Success)
	require.NotNil(t, body.Message)
	assert.Equal(t, "system.doctor.loaded", *body.Message)
	require.NotNil(t, body.Data)
	return *body.Data
}

func TestDoctorGivesTheOwnerEveryCheck(t *testing.T) {
	c := doctorController(support.FakeDB{}, fixedClock{at: doctorNow})
	data := doctorData(t, asUser(i18n.En, ownerSubject()), c)
	assert.Equal(t, gen.ServerBuild{Version: "v1.2.3", Commit: "abc123", BuiltAt: &builtAt}, data.ServerBuild)
	assert.Equal(t, gen.HealthStatusStatus("ok"), data.Health.Status)
	require.NotNil(t, data.Checks)
	codes := make([]string, 0, len(*data.Checks))
	for _, check := range *data.Checks {
		codes = append(codes, check.Code)
		assert.Equal(t, gen.DoctorStatusOk, check.Status, check.Code)
		assert.Nil(t, check.Fix, check.Code)
		assert.NotEmpty(t, check.Title, check.Code)
	}
	assert.Equal(t, []string{"database", "migrations", "disk", "clock", "mptcp"}, codes)
}

func TestDoctorGivesAMemberTheVersionAndTheHealthOnly(t *testing.T) {
	c := doctorController(support.FakeDB{Err: errors.New(leakyDoctorErr)}, fixedClock{at: doctorNow})
	for name, subject := range map[string]policy.Subject{
		"member of an organization": memberSubject(),
		"no membership":             {UserID: uuid.New()},
		"owner through an api token": {
			UserID: uuid.New(), InstanceOwner: true, TokenID: uuid.New(), TokenScopes: []policy.Action{policy.OrgRead},
		},
	} {
		t.Run(name, func(t *testing.T) {
			data := doctorData(t, asUser(i18n.En, subject), c)
			assert.Nil(t, data.Checks)
			assert.Equal(t, "v1.2.3", data.ServerBuild.Version)
			assert.Equal(t, gen.HealthStatusStatus("unavailable"), data.Health.Status)
			raw, err := json.Marshal(data)
			require.NoError(t, err)
			var top map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &top))
			assert.NotContains(t, top, "Checks")
		})
	}
}

func TestDoctorShowsAFailedCheckWithTheCommandLineFixWhenTheDatabaseIsCut(t *testing.T) {
	c := doctorController(support.FakeDB{Err: errors.New(leakyDoctorErr)}, fixedClock{err: errors.New(leakyDoctorErr)})
	for lang, m := range map[i18n.Locale]installer.Messages{i18n.En: installer.NewMessages(installer.LangEn), i18n.Ar: installer.NewMessages(installer.LangAr)} {
		t.Run(string(lang), func(t *testing.T) {
			data := doctorData(t, asUser(lang, ownerSubject()), c)
			require.NotNil(t, data.Checks)
			byCode := map[string]gen.DoctorCheck{}
			for _, check := range *data.Checks {
				byCode[check.Code] = check
			}
			require.Contains(t, byCode, "database")
			assert.Equal(t, gen.DoctorStatusFail, byCode["database"].Status)
			require.NotNil(t, byCode["database"].Fix)
			assert.Equal(t, m.T(installer.KeyAdviceLogs), *byCode["database"].Fix)
			assert.Equal(t, m.T(installer.KeyCheckNamed, "database"), byCode["database"].Title)
			assert.Equal(t, gen.DoctorStatusFail, byCode["migrations"].Status)
			assert.NotContains(t, byCode, "clock", "the clock check needs the database")
			assert.Equal(t, gen.DoctorStatusOk, byCode["disk"].Status, "checks that do not need the database still run")
		})
	}
}

func TestDoctorTranslatesByLocaleAndKeepsTheArabicDefault(t *testing.T) {
	c := doctorController(support.FakeDB{Err: errors.New("down")}, nil)
	subject := ownerSubject()
	en := doctorData(t, asUser(i18n.En, subject), c)
	ar := doctorData(t, asUser(i18n.Ar, subject), c)
	fallback := doctorData(t, auth.WithPrincipal(context.Background(), auth.Principal{Subject: subject}), c)
	require.NotNil(t, en.Checks)
	require.NotNil(t, ar.Checks)
	require.NotNil(t, fallback.Checks)
	require.NotNil(t, (*en.Checks)[0].Fix)
	require.NotNil(t, (*ar.Checks)[0].Fix)
	assert.NotEqual(t, *(*en.Checks)[0].Fix, *(*ar.Checks)[0].Fix)
	assert.Equal(t, *(*ar.Checks)[0].Fix, *(*fallback.Checks)[0].Fix)
}

func TestDoctorNeverReturnsADetailOrAPath(t *testing.T) {
	c := doctorController(support.FakeDB{Err: errors.New(leakyDoctorErr)}, fixedClock{err: errors.New(leakyDoctorErr)})
	for _, lang := range []i18n.Locale{i18n.En, i18n.Ar} {
		res, err := c.GetDoctor(asUser(lang, ownerSubject()), gen.GetDoctorRequestObject{})
		require.NoError(t, err)
		raw, err := json.Marshal(res)
		require.NoError(t, err)
		for _, marker := range []string{"hunter2", "10.0.0.5", "5432", "password", "dial tcp", privateStateDir, "/proc/", "statfs"} {
			assert.NotContains(t, string(raw), marker, marker)
		}
	}
}

func TestMetaCarriesTheServerBuild(t *testing.T) {
	c := doctorController(support.FakeDB{}, nil)
	res, err := c.GetMeta(context.Background(), gen.GetMetaRequestObject{})
	require.NoError(t, err)
	body, ok := res.(gen.GetMeta200JSONResponse)
	require.True(t, ok)
	require.NotNil(t, body.Data.ServerBuild)
	assert.Equal(t, gen.ServerBuild{Version: "v1.2.3", Commit: "abc123", BuiltAt: &builtAt}, *body.Data.ServerBuild)
}
