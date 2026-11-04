package sandbox

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	LabelRunner  = "dhow.runner"
	LabelAttempt = "dhow.attempt"
)

var (
	ErrImagePull    = errors.New("the job image could not be pulled")
	ErrSandbox      = errors.New("the sandbox could not be created")
	ErrShellMissing = errors.New("the job image has no /bin/sh")
	ErrOutputs      = errors.New("the step wrote an output file that is too large or not a regular file")
	ErrDestroyed    = errors.New("the sandbox is gone")
	ErrSurvivors    = errors.New("processes of the sandbox survived the kill")
	ErrNoIDRange    = errors.New("every user id range of the runner is in use")
	ErrWorkspace    = errors.New("the workspace is no longer the nosuid,nodev tmpfs of the job's disk size")
)

type Kind int

const (
	KindJob Kind = iota
	KindCheckout
	KindPlugin
	KindTransfer
)

const (
	PluginManifestLabel = "io.dhow.plugin.manifest"
	AgentPath           = "/run/dhow/agent"
	CheckoutStepIndex   = int32(-1)
)

type Resources struct {
	CPUMillis   int64
	MemoryBytes int64
	Pids        int64
	DiskBytes   int64
}

type WorkspaceSpec struct {
	RunnerID  string
	AttemptID string
	Dir       string
	DiskMode  string
	DiskBytes int64
}

type Workspace interface {
	Check(ctx context.Context) error
	Destroy(ctx context.Context) error
}

type PrepareSpec struct {
	RunnerID  string
	AttemptID string
	Seq       int
	Kind      Kind
	Image     string
	Isolation runnerv1.Isolation
	Resources Resources
	Dir       string
	DiskMode  string
	Egress    []string
	Workspace Workspace
	AuthFile  string
}

type StepSpec struct {
	Index   int32
	Name    string
	Script  string
	Argv    []string
	Env     map[string]string
	Files   map[string][]byte
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Started func()
}

type StepResult struct {
	ExitCode int32
	Reason   runnerv1.FailureReason
	Signal   syscall.Signal
	Outputs  []byte
}

type Driver interface {
	Name() string
	Isolation() []runnerv1.Isolation
	Cleanup(ctx context.Context, runnerID string) error
	Workspace(ctx context.Context, spec WorkspaceSpec) (Workspace, error)
	Prepare(ctx context.Context, spec PrepareSpec) (Sandbox, error)
}

type Labeled interface {
	ImageLabels() map[string]string
}

type Sandbox interface {
	Start(ctx context.Context) error
	Exec(ctx context.Context, step StepSpec) (StepResult, error)
	Signal(ctx context.Context, sig syscall.Signal) error
	Kill(ctx context.Context) error
	Destroy(ctx context.Context) error
	Inspect(ctx context.Context) (Inspection, error)
}

type Egress interface {
	EgressListener() net.Listener
}

type Inspection struct {
	Running   bool
	Destroyed bool
}

func ExitCodeForSignal(sig syscall.Signal) int32 {
	const signalExitBase = 128
	return signalExitBase + int32(sig) //nolint:gosec // Signal numbers are below 65.
}
