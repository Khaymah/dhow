package deployplugin

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

type Common struct {
	Step         jobspec.TargetStep
	Deploy       deployrelease.Config
	BatchSize    int
	BatchPause   time.Duration
	KeyFile      string
	ReportPath   string
	ProxyURL     string
	AllowPrivate []netip.Prefix
	Input        func(name string) string
}

func Load(kind string) (Common, error) { return Parse(os.Getenv, kind) }

func Parse(getenv func(string) string, kind string) (Common, error) {
	step, err := jobspec.ParseTargetStep(getenv)
	if err != nil {
		return Common{}, err
	}
	if !uniqueHosts(step.Hosts) {
		return Common{}, fmt.Errorf("%w: two servers of the target have the same id", ErrInput)
	}
	if step.Kind != kind {
		return Common{}, fmt.Errorf("%w: the target is not a %s target", ErrInput, kind)
	}
	in := func(name string) string { return getenv(envInputPrefix + strings.ToUpper(name)) }
	cfg, err := deployConfig(step, in)
	if err != nil {
		return Common{}, err
	}
	cfg.ArchivePath, cfg.ArchiveDigest = getenv(envArtifactFile), getenv(envArtifactSum)
	if cfg.ArchivePath == "" || !jobspec.ValidDigest(cfg.ArchiveDigest) {
		return Common{}, fmt.Errorf("%w: the artifact", ErrInput)
	}
	allow, err := netguard.ParseCIDRs(getenv(envAllowPrivate))
	if err != nil {
		return Common{}, fmt.Errorf("%w: %w", ErrInput, err)
	}
	batch, pause := max(step.Settings.BatchSize, 1), time.Duration(step.Settings.BatchPauseSeconds)*time.Second
	if err := deployrelease.ValidateRollout(len(step.Hosts), batch, pause, cfg.Health); err != nil {
		return Common{}, fmt.Errorf("%w: %w", ErrInput, err)
	}
	return Common{
		Step: step, Deploy: cfg, BatchSize: batch, BatchPause: pause, KeyFile: step.KeyFile, ReportPath: getenv(envReport),
		ProxyURL: getenv(envProxy), AllowPrivate: allow, Input: in,
	}, cfg.Validate()
}

func uniqueHosts(hosts []jobspec.TargetHost) bool {
	seen := map[string]bool{}
	for _, h := range hosts {
		if seen[h.HostID] {
			return false
		}
		seen[h.HostID] = true
	}
	return true
}

func deployConfig(step jobspec.TargetStep, in func(string) string) (deployrelease.Config, error) {
	host := step.Hosts[0]
	cfg := deployrelease.Config{
		DeployPath: step.DeployPath, ReleaseID: step.ReleaseID, HostID: host.HostID, HostName: host.Host, Keep: step.KeepReleases,
		Shared: step.Settings.Shared, ReloadCommand: step.Settings.ReloadCommand, PreSwitch: step.Settings.PreSwitchCommand,
		PostSwitch: step.Settings.PostSwitchCommand, HookTimeout: deployrelease.DefaultHookTimeout, FirstFailure: OrDefault(in("on_first_failure"), deployrelease.FirstFailureKeep),
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

func OrDefault(v, d string) string {
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
	if h.StatusMin, h.StatusMax, err = parseStatus(OrDefault(in("health_check_status"), "200-399")); err != nil {
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
		d, err := time.ParseDuration(OrDefault(in(f.name), f.def))
		if err != nil {
			return h, fmt.Errorf("%w: %s", ErrInput, f.name)
		}
		f.set(d)
	}
	if h.Retries, err = strconv.Atoi(OrDefault(in("health_check_retries"), "10")); err != nil {
		return h, fmt.Errorf("%w: health_check_retries", ErrInput)
	}
	if h.Stable, err = strconv.Atoi(OrDefault(in("health_check_stable"), "3")); err != nil {
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
