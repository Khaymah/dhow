package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	output "github.com/Khaymah/dhow/apps/cli/internal/Output"
	"github.com/Khaymah/dhow/internal/ingestpush"
)

const (
	ExitRefused    = 1
	ExitToken      = 3
	ExitTemporary  = 4
	ExitUnexpected = 5

	maxTokenInput = 1 << 10
)

func runArtifact(args []string, env Env) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help" {
		c := catalog("", env)
		if len(args) == 0 {
			return usageFor(env.Err, c, output.KeyPushUsage, ExitUsage)
		}
		return usageFor(env.Out, c, output.KeyPushUsage, ExitValid)
	}
	if args[0] != SubcommandPush {
		c := catalog("", env)
		say(env.Err, c.T(output.KeyPushUnknownSubcommand, output.Printable(args[0])), c.T(output.KeyHint))
		return ExitUsage
	}
	return runPush(args[1:], env)
}

func runPush(args []string, env Env) int {
	opts, bad := parsePush(args)
	c := catalog(opts.lang, env)
	if opts.help {
		return usageFor(env.Out, c, output.KeyPushUsage, ExitValid)
	}
	if bad != nil {
		msg := c.T(bad.key)
		if bad.arg != "" {
			msg = c.T(bad.key, output.Printable(bad.arg))
		}
		say(env.Err, msg, c.T(output.KeyHint))
		return ExitUsage
	}
	meta := ingestpush.Meta{
		Name: opts.name, Ref: opts.ref, RefKind: opts.refKind, Sha: opts.sha, NoTrigger: opts.noTrigger, Provenance: opts.provenance,
	}
	server := firstOf(opts.server, env.Getenv(EnvServer))
	if code, ok := precheck(c, env, opts, meta, server); !ok {
		return code
	}
	pkg, err := ingestpush.Prepare(opts.paths[0])
	if err != nil {
		return fail(c, env, err)
	}
	defer pkg.Close()
	if opts.dryRun {
		return renderDryRun(c, env, opts, meta, server, pkg)
	}
	token, ok := readToken(c, env, opts)
	if !ok {
		return ExitUsage
	}
	return upload(c, env, opts, ingestpush.Options{
		Server: server, Token: token, Meta: meta, CAFile: firstOf(opts.caFile, env.Getenv(EnvCAFile)), Retries: opts.retries, Lang: c.Lang,
	}, pkg)
}

func precheck(c output.Catalog, env Env, opts pushOptions, meta ingestpush.Meta, server string) (int, bool) {
	required := []struct{ flag, value string }{
		{"--name", opts.name}, {"--ref", opts.ref}, {"--sha", opts.sha},
	}
	if !opts.dryRun {
		required = append(required, struct{ flag, value string }{"--server", server})
	}
	for _, r := range required {
		if r.value == "" {
			say(env.Err, c.T(output.KeyPushRequired, r.flag), c.T(output.KeyHint))
			return ExitUsage, false
		}
	}
	if err := meta.Validate(); err != nil {
		return fail(c, env, err), false
	}
	return 0, true
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func readToken(c output.Catalog, env Env, opts pushOptions) (string, bool) {
	token := env.Getenv(EnvIngestToken)
	if opts.tokenStdin {
		if env.In == nil {
			say(env.Err, c.T(output.KeyPushNeedToken))
			return "", false
		}
		raw, err := io.ReadAll(io.LimitReader(env.In, maxTokenInput))
		if err != nil {
			say(env.Err, c.T(output.KeyPushTokenRead))
			return "", false
		}
		token = strings.TrimSpace(string(raw))
	}
	switch {
	case token == "":
		say(env.Err, c.T(output.KeyPushNeedToken))
		return "", false
	case !ingestpush.ValidToken(token):
		say(env.Err, c.T(output.KeyPushTokenShape))
		return "", false
	}
	return token, true
}

func upload(c output.Catalog, env Env, opts pushOptions, o ingestpush.Options, pkg *ingestpush.Package) int {
	ctx := env.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	o.OnRetry = func(r ingestpush.Retry) {
		say(env.Err, c.T(output.KeyPushRetrying, r.Attempt, r.Of, c.T(output.CodeKey(r.Code)), r.Wait.Round(time.Second).String()))
	}
	res, err := ingestpush.Upload(ctx, o, pkg)
	if err != nil {
		return fail(c, env, err)
	}
	return renderResult(c, env, opts, res)
}

func fail(c output.Catalog, env Env, err error) int {
	var pe *ingestpush.Error
	if errors.As(err, &pe) {
		say(env.Err, failureLines(c, pe)...)
		return exitOf(pe.Class)
	}
	say(env.Err, c.T(output.KeyPushPrefix)+" "+inputMessage(c, err))
	return ExitUsage
}

func exitOf(class ingestpush.Class) int {
	switch class {
	case ingestpush.ClassRefused:
		return ExitRefused
	case ingestpush.ClassToken:
		return ExitToken
	case ingestpush.ClassTemporary:
		return ExitTemporary
	case ingestpush.ClassUnexpected:
		return ExitUnexpected
	default:
		return ExitUsage
	}
}
