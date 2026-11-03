package app

import (
	"os"
	"strconv"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	archive "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Archive"
)

const (
	CommandPack      = sandbox.PackCommand
	CommandUnpack    = sandbox.UnpackCommand
	CommandHashFiles = sandbox.HashFilesCommand
	EnvUnpackMax     = sandbox.UnpackMaxEnv
)

func transferArgs(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

func runPack(args []string, env Env) error {
	dir, err := os.Getwd()
	if err != nil {
		return exitWith(sandbox.TransferExitFailed)
	}
	return exitWith(archive.RunPack(dir, transferArgs(args), env.Stdio.Out, env.Stdio.Err))
}

func runUnpack(env Env) error {
	dir, err := os.Getwd()
	if err != nil {
		return exitWith(sandbox.TransferExitFailed)
	}
	var limit int64
	if v := env.Getenv(EnvUnpackMax); v != "" {
		if limit, err = strconv.ParseInt(v, 10, 64); err != nil || limit < 0 {
			return exitWith(sandbox.TransferExitFailed)
		}
	}
	return exitWith(archive.RunUnpack(dir, env.Stdio.In, limit, env.Stdio.Err))
}

func runHashFiles(args []string, env Env) error {
	dir, err := os.Getwd()
	if err != nil {
		return exitWith(sandbox.TransferExitFailed)
	}
	return exitWith(archive.RunHashFiles(dir, transferArgs(args), env.Stdio.Out, env.Stdio.Err))
}
