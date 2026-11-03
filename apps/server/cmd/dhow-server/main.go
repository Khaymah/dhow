package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	installer "github.com/Khaymah/dhow/apps/server/internal/Core/Installer"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

const commandHealthcheck = "healthcheck"

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dhow-server:", err)
		os.Exit(exitCode(err))
	}
}

func exitCode(err error) int {
	var incompatible *migrations.IncompatibleError
	if errors.As(err, &incompatible) {
		return incompatible.ExitCode()
	}
	return 1
}

func run(ctx context.Context, args []string) error {
	switch {
	case len(args) == 0:
		return app.Run(ctx)
	case len(args) == 1 && args[0] == commandHealthcheck:
		return app.RunHealthcheck(ctx)
	case args[0] == app.CommandMigrate:
		return runMigrate(ctx, args[1:])
	case installer.IsCommand(args[0]):
		return runInstaller(ctx, args)
	default:
		return fmt.Errorf("unknown command %q; run without arguments to start the server, or use %q, %q, %q, %q or %q",
			args[0], commandHealthcheck, app.CommandMigrate, installer.CommandInstall, installer.CommandDoctor, installer.CommandUninstall)
	}
}

func runMigrate(ctx context.Context, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return app.RunMigrate(ctx, args, os.Stdout)
}

func runInstaller(ctx context.Context, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return installer.Main(ctx, args, installer.App{
		Sys:     installer.NewHost(),
		DB:      installer.PostgresChecker{},
		In:      os.Stdin,
		InFD:    installer.StdinFD(os.Stdin),
		Out:     os.Stdout,
		Locale:  config.LocaleFromEnv(),
		Version: buildinfo.Get().Version,
	}, os.Stderr)
}
