package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	app "github.com/Khaymah/dhow/apps/runner/internal/App"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	args := os.Args[1:]
	switch filepath.Base(os.Args[0]) {
	case app.ConnectName:
		args = append([]string{app.CommandConnect}, args...)
	case app.GitSSHName:
		args = append([]string{app.CommandGitSSH}, args...)
	case app.GitAskpassName:
		args = append([]string{app.CommandGitAskpass}, args...)
	}
	err := app.Main(ctx, args, app.Env{
		Getenv: config.FromEnv(),
		Stdio:  app.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
	})
	stop()
	var exit *app.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.Code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dhow-runner:", err)
		os.Exit(1)
	}
}
