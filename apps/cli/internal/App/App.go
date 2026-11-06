package app

import (
	"context"
	"fmt"
	"io"

	output "github.com/Khaymah/dhow/apps/cli/internal/Output"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

const (
	ExitValid   = 0
	ExitInvalid = 1
	ExitUsage   = 2

	CommandLint    = "lint"
	CommandVersion = "version"
	CommandHelp    = "help"
)

type Env struct {
	Getenv func(string) string
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	Ctx    context.Context
}

func Main(args []string, env Env) int {
	if env.Out == nil {
		env.Out = io.Discard
	}
	if env.Err == nil {
		env.Err = io.Discard
	}
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	if len(args) == 0 {
		return usage(env.Err, catalog("", env), ExitUsage)
	}
	switch args[0] {
	case CommandLint:
		return runLint(args[1:], env)
	case CommandArtifact:
		return runArtifact(args[1:], env)
	case CommandVersion:
		info := buildinfo.Get()
		if _, err := fmt.Fprintf(env.Out, "dhow %s (%s, %s)\n", info.Version, info.Commit, info.GoVersion); err != nil {
			return ExitUsage
		}
		return ExitValid
	case CommandHelp, "-h", "-help", "--help":
		return usage(env.Out, catalog("", env), ExitValid)
	default:
		c := catalog("", env)
		say(env.Err, c.T(output.KeyUnknownCommand, output.Printable(args[0])), c.T(output.KeyHint))
		return ExitUsage
	}
}

func catalog(flagLang string, env Env) output.Catalog {
	return output.NewCatalog(output.ChooseLang(flagLang, env.Getenv))
}

func usage(w io.Writer, c output.Catalog, code int) int {
	return usageFor(w, c, output.KeyUsage, code)
}

func usageFor(w io.Writer, c output.Catalog, key output.Key, code int) int {
	if _, err := io.WriteString(w, c.T(key)); err != nil {
		return ExitUsage
	}
	return code
}

func say(w io.Writer, lines ...string) {
	for _, l := range lines {
		if _, err := io.WriteString(w, l+"\n"); err != nil {
			return
		}
	}
}
