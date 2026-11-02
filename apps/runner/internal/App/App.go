package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	installer "github.com/Khaymah/dhow/apps/runner/internal/Installer"
	podman "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Podman"
	"github.com/Khaymah/dhow/internal/buildinfo"
	"github.com/Khaymah/dhow/internal/logging"
)

const (
	CommandRegister = "register"
	CommandRun      = "run"
	CommandVersion  = "version"
)

var (
	ErrUsage        = errors.New("usage: dhow-runner install [--answers <file>] [--dry-run] [--yes] | dhow-runner doctor [--fix] | dhow-runner uninstall [--remove-user] [--yes] | dhow-runner register --url <server> --token-file <path|-> [--name <name>] [--dev] | dhow-runner run [--dev] | dhow-runner rotate-keys [--dev] | dhow-runner version")
	ErrTokenOnFlags = errors.New("the registration token is never passed on the command line, where shell history and ps would keep it; use --token-file <path> or --token-file - to read it from stdin")
)

type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type Env struct {
	Getenv  func(string) string
	Stdio   Stdio
	Machine func() (config.Machine, error)
	Harden  func() error
	Root    func() bool
	Sealer  identity.Sealer
}

func Main(ctx context.Context, args []string, env Env) error {
	if len(args) == 0 {
		return ErrUsage
	}
	if env.Machine == nil {
		env.Machine = config.DetectMachine
	}
	if env.Harden == nil {
		env.Harden = hardenProcess
	}
	if env.Root == nil {
		env.Root = func() bool { return os.Geteuid() == 0 }
	}
	if env.Sealer == nil && env.Root() {
		env.Sealer = identity.SystemdCreds{}
	}
	switch args[0] {
	case CommandAgent:
		return runAgent(ctx, env)
	case CommandConnect:
		return runConnect(ctx, args[1:], env)
	case CommandCheckout:
		return runCheckout(ctx, env)
	case CommandGitSSH:
		return runGitSSH(ctx, args[1:], env)
	case CommandGitAskpass:
		return runGitAskpass(args[1:], env)
	case podman.CommandMountWorkspace, podman.CommandUnmountWorkspace:
		return podman.RunWorkspaceMount(args[0], args[1:])
	case CommandRegister:
		return runRegister(ctx, args[1:], env)
	case CommandRun:
		return runRun(ctx, args[1:], env)
	case CommandRotateKeys:
		return runRotateKeys(ctx, args[1:], env)
	case installer.CommandInstall, installer.CommandDoctor, installer.CommandUninstall:
		return runInstaller(ctx, args, env)
	case CommandVersion:
		info := buildinfo.Get()
		if _, err := fmt.Fprintf(env.Stdio.Out, "dhow-runner %s (%s, %s)\n", info.Version, info.Commit, info.GoVersion); err != nil {
			return err
		}
		if image := buildinfo.CheckoutImage(); image != "" {
			_, err := fmt.Fprintf(env.Stdio.Out, "checkout image %s\n", image)
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q: %w", args[0], ErrUsage)
	}
}

func refuseTokenFlags(args []string) error {
	for _, a := range args {
		name := strings.TrimLeft(a, "-")
		name, _, _ = strings.Cut(name, "=")
		if strings.HasPrefix(a, "-") && (name == "token" || name == "registration-token") {
			return ErrTokenOnFlags
		}
		if strings.HasPrefix(a, config.RegistrationTokenPrefix) {
			return ErrTokenOnFlags
		}
	}
	return nil
}

func newFlags(name string, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	return fs
}

func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return logging.New(w, level).With("component", "dhow-runner")
}

func openStore(cfg config.Config, env Env) (identity.Store, error) {
	owner := config.NoOwner
	if env.Root() {
		o, err := config.OwnerOf(cfg.StateDir)
		switch {
		case err == nil:
			owner = o
		case !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
	}
	return identity.Open(cfg, env.Sealer, owner)
}
