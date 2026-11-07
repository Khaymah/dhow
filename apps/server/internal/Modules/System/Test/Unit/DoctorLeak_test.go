package unit_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Models"
	support "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Test/Support"
	engine "github.com/Khaymah/dhow/internal/installer"
	"github.com/Khaymah/dhow/internal/installer/fakesys"
)

const (
	leakyPath   = "/etc/dhow/keyring-secret-path"
	leakyHost   = "db-internal.corp.example"
	leakyMarker = "planted-marker-91ab"
	leakyKernel = "6.8.0-leaky-kernel"
	warnFree    = 3 << 30
)

func failingCheck(name models.HealthCheckName) system.ReadinessCheck {
	return system.ReadinessCheck{Name: name, Run: func(context.Context) error {
		return errors.New("open " + leakyPath + " on " + leakyHost + " token=" + leakyMarker)
	}}
}

func leakyController() *system.Controller {
	health := system.NewHealthService(support.FakeDB{}, support.FakeMigrations{}, support.DiscardLogger(),
		failingCheck(models.CheckKeyring), failingCheck(models.CheckSettings), failingCheck(models.CheckLogPartitions))
	host := fakesys.New()
	host.Free = warnFree
	host.Put(engine.MPTCPEnabledPath, "1\n", 0o644)
	host.Put(engine.KernelReleasePath, leakyKernel+"\n", 0o444)
	return system.NewController(health, system.NewMetaService(support.FakeQueries{}, doctorBuild)).
		WithDoctor(system.NewDoctorService(health, doctorBuild, system.DoctorOptions{
			Host: host, StateDir: privateStateDir, Clock: fixedClock{err: errors.New(leakyHost)}, Now: func() time.Time { return doctorNow },
		}))
}

func TestDoctorLeaksNothingFromFailingChecksForTheOwnerOrAMember(t *testing.T) {
	c := leakyController()
	forbidden := []string{leakyPath, leakyHost, leakyMarker, leakyKernel, privateStateDir, "/proc/", "token="}
	for _, lang := range []i18n.Locale{i18n.En, i18n.Ar} {
		for name, subject := range map[string]policy.Subject{"owner": ownerSubject(), "member": memberSubject()} {
			data := doctorData(t, asUser(lang, subject), c)
			raw, err := json.Marshal(data)
			require.NoError(t, err)
			for _, text := range forbidden {
				assert.NotContains(t, string(raw), text, name)
			}
			assert.Equal(t, gen.HealthStatusStatus("unavailable"), data.Health.Status)
			if name != "owner" {
				assert.Nil(t, data.Checks)
				continue
			}
			require.NotNil(t, data.Checks)
			states := map[string]gen.DoctorStatus{}
			for _, check := range *data.Checks {
				states[check.Code] = check.Status
				if check.Status != gen.DoctorStatusOk {
					assert.NotNil(t, check.Fix, check.Code)
				}
			}
			assert.Equal(t, gen.DoctorStatusFail, states["keyring"])
			assert.Equal(t, gen.DoctorStatusFail, states["settings"])
			assert.Equal(t, gen.DoctorStatusFail, states["log_partitions"])
			assert.Equal(t, gen.DoctorStatusWarn, states["disk"])
			assert.Equal(t, gen.DoctorStatusWarn, states["mptcp"])
		}
	}
}

type countingDB struct{ pings *atomic.Int32 }

func (c countingDB) Ping(context.Context) error {
	c.pings.Add(1)
	return nil
}

func TestDoctorRunsTheChecksOnceForQuickCalls(t *testing.T) {
	pings := &atomic.Int32{}
	now := doctorNow
	health := system.NewHealthService(countingDB{pings}, support.FakeMigrations{}, support.DiscardLogger())
	c := system.NewController(health, system.NewMetaService(support.FakeQueries{}, doctorBuild)).
		WithDoctor(system.NewDoctorService(health, doctorBuild, system.DoctorOptions{
			Host: fakesys.New(), StateDir: privateStateDir, Now: func() time.Time { return now },
		}))
	member := asUser(i18n.En, memberSubject())
	for range 5 {
		doctorData(t, member, c)
	}
	assert.Equal(t, int32(1), pings.Load(), "a member looping on the endpoint costs one ping")
	doctorData(t, asUser(i18n.En, ownerSubject()), c)
	assert.Equal(t, int32(2), pings.Load(), "the owner view is a separate entry")
	now = now.Add(system.DoctorCacheTTL)
	doctorData(t, member, c)
	assert.Equal(t, int32(3), pings.Load(), "the entry expires")
}
