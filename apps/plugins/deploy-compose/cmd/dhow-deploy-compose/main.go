package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/app"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/config"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

func main() { os.Exit(run()) }

func run() int {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	settings, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "dhow: PLUGIN_INPUT_INVALID: %v\n", err)
		return deployrelease.ExitInput
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	f := app.Run(ctx, settings, log)
	if f == nil {
		return deployrelease.ExitOK
	}
	_, _ = fmt.Fprintf(os.Stderr, "dhow: %s\n", app.Code(f))
	return f.Exit
}
