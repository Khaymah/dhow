//go:build !dhowtest

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	podman "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Podman"
)

var ErrDriverMissing = errors.New("is not built into this dhow-runner yet; the Kubernetes driver arrives with M3.11")

func driverCapabilities(cfg config.Config) sandbox.Capabilities {
	if cfg.Driver == config.DriverPodman {
		return sandbox.CapabilitiesOf(&podman.Driver{})
	}
	return sandbox.Capabilities{}
}

func newDriver(ctx context.Context, cfg config.Config, log *slog.Logger) (sandbox.Driver, error) {
	switch cfg.Driver {
	case config.DriverPodman:
		return podman.New(ctx, podman.Options{Config: cfg, Log: log, Levels: driverIsolation(cfg)})
	default:
		return nil, fmt.Errorf("%s=%s %w", config.EnvDriver, cfg.Driver, ErrDriverMissing)
	}
}
