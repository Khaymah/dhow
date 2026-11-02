package app

import (
	"context"
	"fmt"
	"os"

	checkout "github.com/Khaymah/dhow/apps/runner/internal/Checkout"
)

const (
	CommandCheckout   = checkout.Command
	CommandGitSSH     = "__git-ssh"
	CommandGitAskpass = "__git-askpass"
	GitSSHName        = "git-ssh"
	GitAskpassName    = "git-askpass"
)

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

func exitWith(code int) error {
	if code == 0 {
		return nil
	}
	return &ExitError{Code: code}
}

func runCheckout(ctx context.Context, env Env) error {
	dir, err := os.Getwd()
	if err != nil {
		return exitWith(checkout.ExitFailed)
	}
	return exitWith(checkout.Run(ctx, checkout.Options{Getenv: env.Getenv, Stdout: env.Stdio.Out, Stderr: env.Stdio.Err, Dir: dir}))
}

func helperOptions(env Env) checkout.HelperOptions {
	return checkout.HelperOptions{Getenv: env.Getenv, Stdin: env.Stdio.In, Stdout: env.Stdio.Out, Stderr: env.Stdio.Err}
}

func runGitSSH(ctx context.Context, args []string, env Env) error {
	return exitWith(checkout.RunSSH(ctx, args, helperOptions(env)))
}

func runGitAskpass(args []string, env Env) error {
	return exitWith(checkout.RunAskpass(args, helperOptions(env)))
}
