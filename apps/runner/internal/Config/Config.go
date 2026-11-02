package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Khaymah/dhow/internal/buildinfo"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	EnvURL                = "DHOW_RUNNER_URL"
	EnvCAFile             = "DHOW_RUNNER_CA_FILE"
	EnvStateDir           = "DHOW_RUNNER_STATE_DIR"
	EnvServerOrigin       = "DHOW_RUNNER_SERVER_ORIGIN"
	EnvDriver             = "DHOW_RUNNER_DRIVER"
	EnvCapacity           = "DHOW_RUNNER_CAPACITY"
	EnvMaxCPU             = "DHOW_RUNNER_MAX_CPU"
	EnvMaxMemory          = "DHOW_RUNNER_MAX_MEMORY"
	EnvMaxPids            = "DHOW_RUNNER_MAX_PIDS"
	EnvMaxDisk            = "DHOW_RUNNER_MAX_DISK"
	EnvMaxTimeout         = "DHOW_RUNNER_MAX_TIMEOUT"
	EnvDiskMode           = "DHOW_RUNNER_DISK_MODE"
	EnvCancelGrace        = "DHOW_RUNNER_CANCEL_GRACE"
	EnvEgressAllowPrivate = "DHOW_RUNNER_EGRESS_ALLOW_PRIVATE_CIDRS"
	EnvAllowUnpinned      = "DHOW_RUNNER_ALLOW_UNPINNED_IMAGES"
	EnvRunscPath          = "DHOW_RUNNER_RUNSC_PATH"
	EnvCheckoutImage      = "DHOW_RUNNER_CHECKOUT_IMAGE"
	EnvLogLevel           = "DHOW_RUNNER_LOG_LEVEL"
	EnvCredentialsDir     = "CREDENTIALS_DIRECTORY"
	EnvInvocationID       = "INVOCATION_ID"
)

type Limits struct {
	CPUMillis   int64
	MemoryBytes int64
	Pids        int64
	DiskBytes   int64
	Timeout     time.Duration
}

type Config struct {
	URL                 *url.URL
	Origin              string
	CAFile              string
	StateDir            string
	Driver              string
	Capacity            int
	Limits              Limits
	DiskMode            string
	CancelGrace         time.Duration
	EgressAllowPrivate  []netip.Prefix
	AllowUnpinnedImages bool
	RunscPath           string
	CheckoutImage       string
	LogLevel            slog.Level
	Dev                 bool
	FakeRecording       string
	CredentialsDir      string
	Privileged          bool
}

type Options struct {
	Dev        bool
	URL        string
	Machine    Machine
	Privileged bool
}

var (
	ErrNotRegistered  = errors.New("this runner is not registered yet; run `dhow-runner register --url <server> --token-file -` first")
	ErrNeedsURL       = errors.New("is required (the server address, for example https://dhow.example.com)")
	ErrRelativePath   = errors.New("must be an absolute path")
	ErrUnknownDriver  = errors.New("must be podman, kubernetes or fake")
	ErrUnknownDisk    = errors.New("must be tmpfs or directory")
	ErrDiskUnenforced = errors.New("is directory, which has no enforced disk limit yet, so only tmpfs is accepted (ADR-0056)")
	ErrAboveMachine   = errors.New("is more than this machine has")
	ErrOutOfRange     = errors.New("is out of range")
	ErrCAFileNoCerts  = errors.New("has no PEM certificate")
	ErrRunscNotBinary = errors.New("must point to an executable file")
	ErrCheckoutImage  = errors.New("must be a full image reference pinned by digest (registry/name@sha256:...)")
)

func FromEnv() func(string) string { return os.Getenv }

func SettableEnv() []string {
	return append([]string{
		EnvURL, EnvCAFile, EnvStateDir, EnvServerOrigin, EnvDriver, EnvCapacity, EnvMaxCPU, EnvMaxMemory, EnvMaxPids,
		EnvMaxDisk, EnvMaxTimeout, EnvDiskMode, EnvCancelGrace, EnvEgressAllowPrivate, EnvAllowUnpinned, EnvRunscPath,
		EnvLogLevel, EnvCheckoutImage,
	}, buildEnv()...)
}

func Load(getenv func(string) string, o Options) (Config, error) {
	cfg := Config{Dev: o.Dev, Privileged: o.Privileged}
	var errs []error
	add := func(name string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	raw := o.URL
	if raw == "" {
		raw = getenv(EnvURL)
	}
	if raw != "" {
		u, err := ParseServerURL(raw, o.Dev)
		add(EnvURL, err)
		cfg.URL = u
	}
	origin, err := loadOrigin(getenv(EnvServerOrigin), cfg.URL, o.Dev)
	add(EnvServerOrigin, err)
	cfg.Origin = origin
	cfg.CAFile = getenv(EnvCAFile)
	if cfg.CAFile != "" {
		add(EnvCAFile, checkCAFile(cfg.CAFile))
	}
	cfg.StateDir, err = loadStateDir(getenv(EnvStateDir))
	add(EnvStateDir, err)
	cfg.Driver, err = oneOf(getenv(EnvDriver), DriverPodman, ErrUnknownDriver, DriverPodman, DriverKubernetes, DriverFake)
	add(EnvDriver, err)
	cfg.DiskMode, err = oneOf(getenv(EnvDiskMode), DiskModeTmpfs, ErrUnknownDisk, DiskModeTmpfs, DiskModeDirectory)
	if cfg.DiskMode == DiskModeDirectory {
		err = ErrDiskUnenforced
	}
	add(EnvDiskMode, err)
	cfg.Capacity, err = parseIntRange(getenv(EnvCapacity), DefaultCapacity, MinCapacity, MaxCapacity)
	add(EnvCapacity, err)
	cfg.CancelGrace, err = parseDurationRange(getenv(EnvCancelGrace), DefaultCancelGrace, MinCancelGrace, MaxCancelGrace)
	add(EnvCancelGrace, err)
	cfg.EgressAllowPrivate, err = parsePrefixes(getenv(EnvEgressAllowPrivate))
	add(EnvEgressAllowPrivate, err)
	cfg.AllowUnpinnedImages, err = parseBool(getenv(EnvAllowUnpinned))
	add(EnvAllowUnpinned, err)
	cfg.RunscPath, err = loadRunsc(getenv(EnvRunscPath))
	add(EnvRunscPath, err)
	cfg.LogLevel, err = parseLevel(getenv(EnvLogLevel))
	add(EnvLogLevel, err)
	cfg.CheckoutImage, err = loadCheckoutImage(getenv(EnvCheckoutImage), buildinfo.CheckoutImage())
	add(EnvCheckoutImage, err)
	cfg.FakeRecording, err = loadFake(getenv, cfg.Driver)
	add(EnvDriver, err)
	cfg.CredentialsDir, err = loadCredentialsDir(getenv(EnvCredentialsDir), getenv(EnvInvocationID))
	add(EnvCredentialsDir, err)
	limits, limitErrs := loadLimits(getenv, o.Machine, cfg)
	cfg.Limits = limits
	errs = append(errs, limitErrs...)
	if err := checkStateDir(cfg.StateDir, o.Privileged); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvStateDir, err))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) IdentityPath() string { return filepath.Join(c.StateDir, IdentityFileName) }

func (c Config) PendingIdentityPath() string { return filepath.Join(c.StateDir, PendingFileName) }

func (c Config) SealedIdentityPath() string { return filepath.Join(c.StateDir, SealedIdentityFileName) }

func (c Config) SealedPendingPath() string { return filepath.Join(c.StateDir, SealedPendingFileName) }

func (c Config) CredentialPath() string {
	if c.CredentialsDir == "" {
		return ""
	}
	return filepath.Join(c.CredentialsDir, IdentityCredential)
}

func (c Config) LockPath() string { return filepath.Join(c.StateDir, LockFileName) }

func (c Config) JournalPath() string { return filepath.Join(c.StateDir, JournalFileName) }

func (c Config) AttemptsDir() string { return filepath.Join(c.StateDir, AttemptsDirName) }

func loadCheckoutImage(v, baked string) (string, error) {
	if v == "" {
		v = baked
	}
	if v == "" {
		return "", nil
	}
	if jobspec.ImageReason(v) != "" || jobspec.QualifyImage(v) != v {
		return "", ErrCheckoutImage
	}
	return v, nil
}

func loadStateDir(v string) (string, error) {
	if v == "" {
		return DefaultStateDir, nil
	}
	if !filepath.IsAbs(v) {
		return "", ErrRelativePath
	}
	return filepath.Clean(v), nil
}

func loadCredentialsDir(v, invocation string) (string, error) {
	if v == "" || invocation == "" {
		return "", nil
	}
	if !filepath.IsAbs(v) {
		return "", ErrRelativePath
	}
	dir := filepath.Clean(v)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&groupOtherW != 0 {
		return "", nil
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || (st.Uid != 0 && int(st.Uid) != os.Geteuid()) {
		return "", nil
	}
	return dir, nil
}

func loadRunsc(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if !filepath.IsAbs(v) {
		return "", ErrRelativePath
	}
	info, err := os.Stat(v)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", v, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", ErrRunscNotBinary
	}
	return filepath.Clean(v), nil
}

func oneOf(v, def string, bad error, allowed ...string) (string, error) {
	if v == "" {
		return def, nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", bad
}

func loadLimits(getenv func(string) string, m Machine, cfg Config) (Limits, []error) {
	var errs []error
	add := func(name string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	var l Limits
	var err error
	l.CPUMillis, err = parseCPU(getenv(EnvMaxCPU), m.CPUMillis)
	add(EnvMaxCPU, err)
	memory, memErrs := parseJobMemory(getenv, cfg.DiskMode)
	errs = append(errs, memErrs...)
	l.MemoryBytes, l.DiskBytes = memory.MemoryBytes, memory.DiskBytes
	pids, err := parseIntRange(getenv(EnvMaxPids), jobspec.DefaultPids, jobspec.MinPids, jobspec.MaxPids)
	add(EnvMaxPids, err)
	l.Pids = int64(pids)
	l.Timeout, err = parseDurationRange(getenv(EnvMaxTimeout), DefaultMaxTimeout, MinTimeout, MaxTimeout)
	add(EnvMaxTimeout, err)
	if len(errs) > 0 {
		return Limits{}, errs
	}
	return l, checkMachine(l, m, cfg)
}

func checkMachine(l Limits, m Machine, cfg Config) []error {
	var errs []error
	if m.CPUMillis > 0 && l.CPUMillis > m.CPUMillis {
		errs = append(errs, fmt.Errorf("%s: %d millicores %w (%d)", EnvMaxCPU, l.CPUMillis, ErrAboveMachine, m.CPUMillis))
	}
	if m.PidMax > 0 && l.Pids > m.PidMax {
		errs = append(errs, fmt.Errorf("%s: %d %w (pid_max %d)", EnvMaxPids, l.Pids, ErrAboveMachine, m.PidMax))
	}
	memory := JobMemory{MemoryBytes: l.MemoryBytes, DiskBytes: l.DiskBytes, Tmpfs: cfg.DiskMode == DiskModeTmpfs}
	if err := CheckJobMemory(memory, m.MemoryBytes); err != nil {
		errs = append(errs, err)
	}
	if m.DiskFree != nil {
		jobs := int64(max(cfg.Capacity, 1))
		need := jobs * LogSpillBytes
		if cfg.DiskMode == DiskModeDirectory {
			need += jobs * l.DiskBytes
		}
		free, err := m.DiskFree(existingParent(cfg.StateDir))
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", EnvStateDir, err))
		case need > free:
			errs = append(errs, fmt.Errorf("%s: %d jobs need %d bytes for log spill files%s, which %w (%d free under %s)",
				EnvStateDir, jobs, need, diskNote(cfg.DiskMode), ErrAboveMachine, free, cfg.StateDir))
		}
	}
	return errs
}

func diskNote(mode string) string {
	if mode == DiskModeDirectory {
		return " and " + EnvMaxDisk
	}
	return ""
}

func existingParent(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil || d == filepath.Dir(d) {
			return d
		}
	}
}
