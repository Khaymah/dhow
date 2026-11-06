package stack

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

const (
	ExitPolicy = 30

	OverrideFile = "compose.dhow.yaml"

	DefaultWaitTimeout = 120 * time.Second
	MaxWaitTimeout     = 10 * time.Minute
	DefaultPullTimeout = 10 * time.Minute
	execGrace          = 30 * time.Second
	maxLogged          = 4 << 10
	sharedEnvFile      = "shared/.env"
)

var (
	ErrCommand = errors.New("docker compose did not finish")
	ErrFile    = errors.New("the compose file on the server is not a regular file")
)

type Activator struct {
	Project     string
	File        string
	WaitTimeout time.Duration
	PullTimeout time.Duration
	Policy      policy.Options
	Log         deployrelease.Logger

	links []string
}

func (a *Activator) Prepare(ctx context.Context, r deployrelease.Remote, rel deployrelease.Release) error {
	file := rel.Path + "/" + a.File
	info, err := r.Lstat(file)
	if err != nil || !info.Mode.IsRegular() {
		return &deployrelease.Failure{Exit: ExitPolicy, Reason: deployrelease.CauseOther, Text: "the compose file is not on the server as a regular file", Err: errors.Join(ErrFile, err)}
	}
	raw, err := r.ReadFile(file, policy.MaxFileBytes+1)
	if err != nil {
		return fmt.Errorf("read the compose file: %w", err)
	}
	opt := a.Policy
	opt.DeployPath = rel.DeployPath
	opt.Links = a.links
	res, err := policy.Check(raw, opt)
	if err != nil {
		return refusal(err)
	}
	override, err := policy.Override(res.Images)
	if err != nil {
		return refusal(err)
	}
	if err := deployrelease.WriteFile(r, rel.Path+"/"+OverrideFile, override, 0o644); err != nil {
		return fmt.Errorf("write the image override: %w", err)
	}
	return a.run(ctx, r, rel, a.pullTimeout(), "pull", "--quiet")
}

func (a *Activator) Activate(ctx context.Context, r deployrelease.Remote, rel deployrelease.Release) error {
	wait := a.waitTimeout()
	return a.run(ctx, r, rel, wait+execGrace, "up", "--detach", "--wait", "--no-build", "--wait-timeout="+strconv.Itoa(int(wait.Seconds())))
}

func refusal(err error) *deployrelease.Failure {
	return &deployrelease.Failure{Exit: ExitPolicy, Reason: deployrelease.CauseOther, Text: "the compose file is not safe to deploy", Err: err}
}

func (a *Activator) waitTimeout() time.Duration {
	if a.WaitTimeout <= 0 {
		return DefaultWaitTimeout
	}
	return min(a.WaitTimeout, MaxWaitTimeout)
}

func (a *Activator) pullTimeout() time.Duration {
	if a.PullTimeout <= 0 {
		return DefaultPullTimeout
	}
	return a.PullTimeout
}

func (a *Activator) Argv(rel deployrelease.Release, envFile bool, tail ...string) []string {
	argv := []string{
		"docker", "compose",
		"--project-name=" + a.Project,
		"--project-directory=" + rel.Path,
		"--file=" + rel.Path + "/" + a.File,
		"--file=" + rel.Path + "/" + OverrideFile,
	}
	if envFile {
		argv = append(argv, "--env-file="+rel.DeployPath+"/"+sharedEnvFile)
	}
	return append(argv, tail...)
}

func (a *Activator) run(ctx context.Context, r deployrelease.Remote, rel deployrelease.Release, limit time.Duration, tail ...string) error {
	info, err := r.Lstat(rel.DeployPath + "/" + sharedEnvFile)
	argv := a.Argv(rel, err == nil && info.Mode.IsRegular(), tail...)
	cctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	started := time.Now()
	res, err := r.Exec(cctx, argv)
	if err != nil {
		return fmt.Errorf("%w: docker compose %s: %w", ErrCommand, tail[0], err)
	}
	if a.Log != nil {
		a.Log.Info("docker compose finished", "command", tail[0], "exit", res.ExitCode, "seconds", int(time.Since(started).Seconds()), "output", logged(res.Output))
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%w: docker compose %s exited with %d", ErrCommand, tail[0], res.ExitCode)
	}
	return nil
}

func logged(b []byte) string {
	if len(b) > maxLogged {
		b = b[len(b)-maxLogged:]
	}
	return string(b)
}
