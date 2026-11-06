package deployrelease

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	ExitOK              = 0
	ExitFailed          = 1
	ExitInput           = 2
	ExitRolledBack      = 10
	ExitHealthFailed    = 11
	ExitRollbackFailed  = 12
	ExitHostKeyMismatch = 13
	ExitUnsafeArtifact  = 14
	ExitTargetLocked    = 15
	ExitCurrentNotLink  = 16
	ExitConnect         = 17
	ExitStateCorrupt    = 18
	ExitCheckFailed     = 19

	FirstFailureKeep = "keep"
	FirstFailureFail = "fail"

	DefaultHookTimeout = 120 * time.Second
	MaxHookTimeout     = 600 * time.Second
	LockTTL            = 30 * time.Minute
	RollbackBudget     = jobspec.DeployRollbackBudget
	CleanupBudget      = jobspec.DeployCleanupBudget
	DefaultLockTimeout = 10 * time.Second
)

var (
	ErrHostKeyMismatch = errors.New("the host key of the server is not the pinned one")
	ErrUnsafeArtifact  = errors.New("the artifact has an entry that is not safe to put on a server")
	ErrTargetLocked    = errors.New("another deployment holds the lock of this target")
	ErrCurrentNotLink  = errors.New("current is not a symlink, so Dhow does not touch it")
	ErrConfig          = errors.New("the deployment settings are not valid")
	ErrDigest          = errors.New("the artifact is not the one whose digest was handed over")
	ErrSharedConflict  = errors.New("the release has a file with the name of a shared path")
	ErrNoSpace         = errors.New("the server does not have the disk space for the release")
	ErrAuthFailed      = errors.New("the server did not accept the key of the target")
	ErrStateCorrupt    = errors.New("a state file of Dhow on the server is damaged")
)

type Failure struct {
	Exit   int
	Reason string
	Text   string
	Err    error
}

func (f *Failure) Error() string {
	if f.Err == nil {
		return f.Text
	}
	return f.Text + ": " + f.Err.Error()
}

func (f *Failure) Unwrap() error { return f.Err }

func fail(exit int, reason, text string, err error) *Failure {
	return &Failure{Exit: exit, Reason: reason, Text: text, Err: err}
}

type Config struct {
	DeployPath    string
	ReleaseID     string
	HostID        string
	HostName      string
	Keep          int
	Shared        []string
	ReloadCommand string
	PreSwitch     string
	PostSwitch    string
	HookTimeout   time.Duration
	Health        Health
	FirstFailure  string
	ArchivePath   string
	ArchiveDigest string
	Limits        archive.Limits
}

func (c Config) Validate() error {
	switch {
	case !jobspec.ValidDeployPath(c.DeployPath):
		return fmt.Errorf("%w: the deploy path", ErrConfig)
	case !jobspec.ValidReleaseID(c.ReleaseID):
		return fmt.Errorf("%w: the release id", ErrConfig)
	case c.Keep < jobspec.MinKeepReleases || c.Keep > jobspec.MaxKeepReleases:
		return fmt.Errorf("%w: keep_releases", ErrConfig)
	case c.HostID == "" || c.HostName == "":
		return fmt.Errorf("%w: the host", ErrConfig)
	case c.FirstFailure != FirstFailureKeep && c.FirstFailure != FirstFailureFail:
		return fmt.Errorf("%w: on_first_failure", ErrConfig)
	case c.HookTimeout <= 0 || c.HookTimeout > MaxHookTimeout:
		return fmt.Errorf("%w: the hook timeout", ErrConfig)
	case c.ArchivePath == "":
		return fmt.Errorf("%w: the artifact", ErrConfig)
	}
	for _, h := range []string{c.ReloadCommand, c.PreSwitch, c.PostSwitch} {
		if !jobspec.ValidHook(h) {
			return fmt.Errorf("%w: a hook", ErrConfig)
		}
	}
	for _, n := range c.Shared {
		if !jobspec.ValidSharedName(n) {
			return fmt.Errorf("%w: shared", ErrConfig)
		}
	}
	if err := c.Health.Validate(); err != nil {
		return err
	}
	return nil
}

type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

var _ Logger = (*slog.Logger)(nil)
