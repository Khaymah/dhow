package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	StrategyAtomicSymlink = "atomic-symlink"

	envInputPrefix  = "DHOW_INPUT_"
	envArtifactFile = "DHOW_INPUT_ARTIFACT_FILE"
	envArtifactSum  = "DHOW_INPUT_ARTIFACT_DIGEST"
	envReport       = "DHOW_DEPLOY_REPORT"
	envProxy        = "HTTPS_PROXY"
	envAllowPrivate = "DHOW_DEPLOY_ALLOW_PRIVATE_CIDRS"
	maxArchiveBytes = 8 << 30
	maxArchiveFiles = 200_000
)

var ErrInput = errors.New("an input of the deploy plugin is not valid")

type Settings struct {
	Step         jobspec.TargetStep
	Deploy       deployrelease.Config
	KeyFile      string
	ReportPath   string
	ProxyURL     string
	AllowPrivate []netip.Prefix
}

func Load() (Settings, error) { return Parse(os.Getenv) }

func Parse(getenv func(string) string) (Settings, error) {
	step, err := jobspec.ParseTargetStep(getenv)
	if err != nil {
		return Settings{}, err
	}
	if len(step.Hosts) != 1 {
		return Settings{}, fmt.Errorf("%w: this version of the plugin deploys to one server, and the target has %d", ErrInput, len(step.Hosts))
	}
	if step.Kind != jobspec.TargetKindSSH {
		return Settings{}, fmt.Errorf("%w: the target is not an ssh target", ErrInput)
	}
	in := func(name string) string { return getenv(envInputPrefix + strings.ToUpper(name)) }
	if s := in("strategy"); s != "" && s != StrategyAtomicSymlink {
		return Settings{}, fmt.Errorf("%w: strategy %q is not supported, only %s", ErrInput, s, StrategyAtomicSymlink)
	}
	cfg, err := deployConfig(step, in)
	if err != nil {
		return Settings{}, err
	}
	cfg.ArchivePath, cfg.ArchiveDigest = getenv(envArtifactFile), getenv(envArtifactSum)
	if cfg.ArchivePath == "" || !jobspec.ValidDigest(cfg.ArchiveDigest) {
		return Settings{}, fmt.Errorf("%w: the artifact", ErrInput)
	}
	allow, err := netguard.ParseCIDRs(getenv(envAllowPrivate))
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %w", ErrInput, err)
	}
	s := Settings{Step: step, Deploy: cfg, KeyFile: step.KeyFile, ReportPath: getenv(envReport), ProxyURL: getenv(envProxy), AllowPrivate: allow}
	return s, cfg.Validate()
}

func deployConfig(step jobspec.TargetStep, in func(string) string) (deployrelease.Config, error) {
	host := step.Hosts[0]
	cfg := deployrelease.Config{
		DeployPath: step.DeployPath, ReleaseID: step.ReleaseID, HostID: host.HostID, HostName: host.Host, Keep: step.KeepReleases,
		Shared: step.Settings.Shared, ReloadCommand: step.Settings.ReloadCommand, PreSwitch: step.Settings.PreSwitchCommand,
		PostSwitch: step.Settings.PostSwitchCommand, HookTimeout: deployrelease.DefaultHookTimeout, FirstFailure: orDefault(in("on_first_failure"), deployrelease.FirstFailureKeep),
		Limits: archive.Limits{MaxEntries: maxArchiveFiles, MaxBytes: maxArchiveBytes},
	}
	if v := in("keep_releases"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("%w: keep_releases", ErrInput)
		}
		cfg.Keep = min(cfg.Keep, n)
	}
	h, err := healthConfig(in)
	cfg.Health = h
	return cfg, err
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func healthConfig(in func(string) string) (deployrelease.Health, error) {
	h := deployrelease.Health{URL: in("health_check_url")}
	if !h.Enabled() {
		return h, nil
	}
	var err error
	if h.StatusMin, h.StatusMax, err = parseStatus(orDefault(in("health_check_status"), "200-399")); err != nil {
		return h, err
	}
	for _, f := range []struct {
		name, def string
		set       func(time.Duration)
	}{
		{"health_check_timeout", "5s", func(d time.Duration) { h.Timeout = d }},
		{"health_check_interval", "5s", func(d time.Duration) { h.Interval = d }},
		{"health_check_start_delay", "0s", func(d time.Duration) { h.StartDelay = d }},
	} {
		d, err := time.ParseDuration(orDefault(in(f.name), f.def))
		if err != nil {
			return h, fmt.Errorf("%w: %s", ErrInput, f.name)
		}
		f.set(d)
	}
	if h.Retries, err = strconv.Atoi(orDefault(in("health_check_retries"), "10")); err != nil {
		return h, fmt.Errorf("%w: health_check_retries", ErrInput)
	}
	if h.Stable, err = strconv.Atoi(orDefault(in("health_check_stable"), "3")); err != nil {
		return h, fmt.Errorf("%w: health_check_stable", ErrInput)
	}
	return h, nil
}

func parseStatus(v string) (int, int, error) {
	first, last, ranged := strings.Cut(v, "-")
	from, err := strconv.Atoi(first)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: health_check_status", ErrInput)
	}
	to := from
	if ranged {
		if to, err = strconv.Atoi(last); err != nil {
			return 0, 0, fmt.Errorf("%w: health_check_status", ErrInput)
		}
	}
	return from, to, nil
}
