package config

import (
	"errors"
	"fmt"
	"os"
)

type MigrateConfig struct {
	DatabaseURL string
	Dev         bool
}

func LoadMigrateFromEnv() (MigrateConfig, error) { return LoadMigrate(os.Getenv) }

func LoadMigrate(getenv func(string) string) (MigrateConfig, error) {
	var cfg MigrateConfig
	var errs []error
	dev, err := parseBool(getenv(EnvDev))
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvDev, err))
	}
	cfg.Dev = dev
	if cfg.DatabaseURL, err = loadDatabaseURL(getenv, dev); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvDatabaseURL, err))
	}
	if len(errs) > 0 {
		return MigrateConfig{}, errors.Join(errs...)
	}
	return cfg, nil
}
