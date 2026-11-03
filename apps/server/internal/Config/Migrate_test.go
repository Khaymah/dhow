package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func TestLoadMigrateNeedsOnlyTheDatabase(t *testing.T) {
	cfg, err := config.LoadMigrate(env(map[string]string{config.EnvDatabaseURL: goodDB}))
	require.NoError(t, err)
	assert.Equal(t, goodDB, cfg.DatabaseURL)
	assert.False(t, cfg.Dev)

	dev, err := config.LoadMigrate(env(map[string]string{
		config.EnvDatabaseURL: "postgres://dhow@localhost/dhow?sslmode=disable", config.EnvDev: "true",
	}))
	require.NoError(t, err)
	assert.True(t, dev.Dev)
}

func TestLoadMigrateRejects(t *testing.T) {
	for name, vars := range map[string]map[string]string{
		"no database":          {},
		"plain text in prod":   {config.EnvDatabaseURL: "postgres://dhow@db/dhow?sslmode=disable"},
		"a bad DHOW_DEV":       {config.EnvDatabaseURL: goodDB, config.EnvDev: "maybe"},
		"two database sources": {config.EnvDatabaseURL: goodDB, config.EnvDatabaseURLFile: "/nonexistent"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.LoadMigrate(env(vars))
			assert.Error(t, err)
		})
	}
}

func TestOnIncompatibleAndInstanceID(t *testing.T) {
	cfg, err := config.Load(env(valid()))
	require.NoError(t, err)
	assert.Equal(t, config.OnIncompatibleExit, cfg.OnIncompatible)
	assert.Empty(t, cfg.InstanceID)

	vars := valid()
	vars[config.EnvOnIncompatible] = "wait"
	vars[config.EnvInstanceID] = "web-1"
	cfg, err = config.Load(env(vars))
	require.NoError(t, err)
	assert.Equal(t, config.OnIncompatibleWait, cfg.OnIncompatible)
	assert.Equal(t, "web-1", cfg.InstanceID)

	for name, bad := range map[string][2]string{
		"an unknown mode":           {config.EnvOnIncompatible, "loop"},
		"an instance id with space": {config.EnvInstanceID, "web 1"},
		"a long instance id":        {config.EnvInstanceID, strings.Repeat("a", 129)},
	} {
		t.Run(name, func(t *testing.T) {
			vars := valid()
			vars[bad[0]] = bad[1]
			_, err := config.Load(env(vars))
			assert.ErrorContains(t, err, bad[0])
		})
	}
}
