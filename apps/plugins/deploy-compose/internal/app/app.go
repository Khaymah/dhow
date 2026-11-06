package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/config"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/stack"
	"github.com/Khaymah/dhow/internal/deployplugin"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

func Run(ctx context.Context, s config.Settings, log *slog.Logger) *deployrelease.Failure {
	common := s.Common
	common.Deploy.Activator = &stack.Activator{
		Project: s.Project, File: s.File, WaitTimeout: s.WaitTimeout, PullTimeout: s.PullTimeout, Log: log,
		Policy: policy.Options{AllowUnsafe: s.AllowUnsafe, Images: s.Images},
	}
	return deployplugin.Run(ctx, common, log)
}

func Code(f *deployrelease.Failure) string {
	var v *policy.Violation
	if f.Exit == stack.ExitPolicy {
		if errors.As(f, &v) {
			return v.Code
		}
		return "COMPOSE_INVALID"
	}
	return deployplugin.Code(f.Exit)
}
