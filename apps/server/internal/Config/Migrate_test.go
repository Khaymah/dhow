package config_test

import (
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
