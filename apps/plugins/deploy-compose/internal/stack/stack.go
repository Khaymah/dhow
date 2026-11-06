package stack

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

const (
	ExitPolicy = 30

	OverrideFile = policy.OverrideFile

	DefaultWaitTimeout = 120 * time.Second
	MaxWaitTimeout     = 10 * time.Minute
	DefaultPullTimeout = 10 * time.Minute
	execGrace          = 30 * time.Second
	maxLogged          = 4 << 10
	sharedEnvFile      = "shared/.env"
	noEnvFile          = "/dev/null"
	maxEnvBytes        = 64 << 10
	safePath           = "/usr/local/bin:/usr/bin:/bin"

	CodeChanged      = "COMPOSE_FILE_CHANGED"
	CodeEnvForbidden = "COMPOSE_ENV_FORBIDDEN"
	CodeSource       = "COMPOSE_BIND_SOURCE"
)

var (
	ErrCommand = errors.New("docker compose did not finish")
	ErrFile    = errors.New("the compose file on the server is not a regular file")

	homePattern = regexp.MustCompile(`^/[A-Za-z0-9._/-]{0,200}$`)
)

type Activator struct {
	Project     string
	File        string
	WaitTimeout time.Duration
	PullTimeout time.Duration
	Policy      policy.Options
	Log         deployrelease.Logger

	links     []string
	releaseID string
}

func (a *Activator) options(deployPath string, links []string) policy.Options {
	opt := a.Policy
	opt.File = a.File
	opt.DeployPath = deployPath
	opt.Links = links
	return opt
}

func (a *Activator) Prepare(ctx context.Context, r deployrelease.Remote, rel deployrelease.Release) error {
	raw, err := a.readRegular(r, rel.Path+"/"+a.File, policy.MaxFileBytes+1)
	if err != nil {
		return err
	}
	opt := a.options(rel.DeployPath, a.links)
	if a.releaseID != "" && rel.ID != a.releaseID {
		opt.Images = nil
	}
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
	if err := writeRecord(r, rel, newRecord(raw, override, a.links, a.Policy.AllowUnsafe)); err != nil {
		return fmt.Errorf("write the record of the compose files: %w", err)
	}
	a.pruneRecords(r, rel)
	return a.run(ctx, r, rel, a.pullTimeout(), "pull", "--quiet")
}

func (a *Activator) neverPrepared(r deployrelease.Remote, rel deployrelease.Release) bool {
	for _, p := range []string{recordPath(rel), rel.Path + "/" + OverrideFile} {
		if _, err := r.Lstat(p); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

func (a *Activator) Activate(ctx context.Context, r deployrelease.Remote, rel deployrelease.Release) error {
	if a.neverPrepared(r, rel) {
		if err := a.Prepare(ctx, r, rel); err != nil {
			return err
		}
	}
	if err := a.verify(r, rel); err != nil {
		return err
	}
	wait := a.waitTimeout()
	return a.run(ctx, r, rel, wait+execGrace, "up", "--detach", "--wait", "--no-build", "--wait-timeout="+strconv.Itoa(int(wait.Seconds())))
}

func (a *Activator) readRegular(r deployrelease.Remote, file string, limit int64) ([]byte, error) {
	info, err := r.Lstat(file)
	if err != nil || !info.Mode.IsRegular() {
		return nil, &deployrelease.Failure{Exit: ExitPolicy, Reason: deployrelease.CauseOther, Text: "the compose file is not on the server as a regular file", Err: errors.Join(ErrFile, err)}
	}
	raw, err := r.ReadFile(file, limit)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	return raw, nil
}

func (a *Activator) verify(r deployrelease.Remote, rel deployrelease.Release) error {
	rec, err := readRecord(r, rel)
	if err != nil {
		return err
	}
	compose, err := a.readRegular(r, rel.Path+"/"+a.File, policy.MaxFileBytes+1)
	if err != nil {
		return err
	}
	override, err := a.readRegular(r, rel.Path+"/"+OverrideFile, policy.MaxFileBytes+1)
	if err != nil {
		return err
	}
	if sum(compose) != rec.ComposeSum || sum(override) != rec.OverrideSum {
		return changed("a compose file of the release is not the one that was checked")
	}
	images, err := policy.ParseOverride(override)
	if err != nil {
		return refusal(err)
	}
	opt := a.options(rel.DeployPath, rec.Links)
	opt.AllowUnsafe, opt.Images = rec.AllowUnsafe, images
	res, err := policy.Check(compose, opt)
	if err != nil {
		return refusal(err)
	}
	if !maps.Equal(res.Images, images) {
		return changed("the image override does not pin every service of the release")
	}
	return checkSources(r, rel, res, rec.AllowUnsafe)
}

func checkSources(r deployrelease.Remote, rel deployrelease.Release, res policy.Result, allowUnsafe bool) error {
	for _, p := range res.Binds {
		info, err := r.Lstat(rel.Path + "/" + p)
		switch {
		case err != nil && allowUnsafe && errors.Is(err, fs.ErrNotExist):
		case err != nil || info.IsSymlink() || (!info.IsDir() && !info.Mode.IsRegular()):
			return source("a bind source is missing, a link or not a folder or file")
		}
	}
	for _, p := range res.Files {
		if !strings.HasPrefix(p, "/") {
			p = rel.Path + "/" + p
		}
		if info, err := r.Lstat(p); err != nil || !info.Mode.IsRegular() {
			return source("a file that Compose reads is missing or is not a regular file")
		}
	}
	return nil
}

func source(detail string) *deployrelease.Failure {
	return refusal(&policy.Violation{Code: CodeSource, Detail: detail})
}

func changed(detail string) *deployrelease.Failure {
	return refusal(&policy.Violation{Code: CodeChanged, Detail: detail})
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

func (a *Activator) Argv(rel deployrelease.Release, envFile string, tail ...string) []string {
	argv := []string{
		"docker", "compose",
		"--project-name=" + a.Project,
		"--project-directory=" + rel.Path,
		"--file=" + rel.Path + "/" + a.File,
		"--file=" + rel.Path + "/" + OverrideFile,
		"--env-file=" + envFile,
	}
	return append(argv, tail...)
}

func Isolated(home string) []string {
	return []string{"env", "-i", "PATH=" + safePath, "DOCKER_CONFIG=" + home + "/.docker"}
}

func (a *Activator) home(ctx context.Context, r deployrelease.Remote) (string, error) {
	res, err := r.Exec(ctx, []string{"printenv", "HOME"})
	home := strings.TrimSpace(string(res.Output))
	if err != nil || res.ExitCode != 0 || !homePattern.MatchString(home) {
		return "", fmt.Errorf("%w: the home folder of the deploy account is not known", ErrCommand)
	}
	return strings.TrimSuffix(home, "/"), nil
}

func (a *Activator) envFile(r deployrelease.Remote, rel deployrelease.Release) (string, error) {
	file := rel.DeployPath + "/" + sharedEnvFile
	info, err := r.Lstat(file)
	if err != nil {
		return noEnvFile, nil
	}
	if !info.Mode.IsRegular() {
		return "", fmt.Errorf("%w: shared/.env is not a regular file", ErrFile)
	}
	raw, err := r.ReadFile(file, maxEnvBytes+1)
	if err != nil || len(raw) > maxEnvBytes {
		return "", fmt.Errorf("%w: shared/.env cannot be read", ErrFile)
	}
	if err := checkEnv(string(raw)); err != nil {
		return "", err
	}
	return file, nil
}

func (a *Activator) run(ctx context.Context, r deployrelease.Remote, rel deployrelease.Release, limit time.Duration, tail ...string) error {
	home, err := a.home(ctx, r)
	if err != nil {
		return err
	}
	envFile, err := a.envFile(r, rel)
	if err != nil {
		return err
	}
	argv := append(Isolated(home), a.Argv(rel, envFile, tail...)...)
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
