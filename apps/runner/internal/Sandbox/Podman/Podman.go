package podman

import (
	"context"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	dockerTransport   = "docker://"
	controlDirName    = "control"
	controlSocketName = "control.sock"
	egressSocketName  = "egress.sock"
	workspaceDirName  = "workspace"
)

var (
	ErrShortImageName = errors.New("the image must start with its registry host, because short names are off")
	ErrImageID        = errors.New("podman did not return the image ID")
	ErrDynamicAgent   = errors.New("the runner binary asks for a dynamic loader, which would come from the job's image; build it with CGO_ENABLED=0")
	imageIDPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Driver struct {
	binary      string
	attemptsDir string
	agentPath   string
	paths       paths
	levels      []runnerv1.Isolation
	skipPull    bool
	log         *slog.Logger
	mu          sync.Mutex
	live        map[string][]idRange
	slots       *idSlots
}

type Options struct {
	Config     config.Config
	Log        *slog.Logger
	Binary     string
	AgentPath  string
	Levels     []runnerv1.Isolation
	SkipChecks bool
	SkipPull   bool
}

func New(ctx context.Context, o Options) (*Driver, error) {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	binary := o.Binary
	if binary == "" {
		resolved, err := exec.LookPath("podman")
		if err != nil {
			return nil, fmt.Errorf("find podman on PATH: %w", err)
		}
		binary = resolved
	}
	agentPath := o.AgentPath
	if agentPath == "" {
		self, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("find the runner binary to mount as the agent: %w", err)
		}
		agentPath = self
	}
	if err := staticAgent(agentPath); err != nil {
		return nil, err
	}
	if !o.SkipChecks {
		if bad := failed(Preflight(ctx, binary)); len(bad) > 0 {
			return nil, &PreflightError{Failed: bad}
		}
	}
	p, err := writeConfig(o.Config.StateDir)
	if err != nil {
		return nil, err
	}
	levels := o.Levels
	if len(levels) == 0 {
		levels = []runnerv1.Isolation{runnerv1.Isolation_ISOLATION_CONTAINER}
	}
	return &Driver{binary: binary, attemptsDir: o.Config.AttemptsDir(), agentPath: agentPath, paths: p, levels: levels, skipPull: o.SkipPull, log: o.Log, live: map[string][]idRange{}}, nil
}

func (d *Driver) Name() string { return "podman" }

func (d *Driver) Isolation() []runnerv1.Isolation { return d.levels }

func (d *Driver) runner() runner { return d.paths.env(d.binary) }

func (d *Driver) Podman(ctx context.Context, args ...string) ([]byte, error) {
	return d.runner().run(ctx, args...)
}

func (d *Driver) Cleanup(ctx context.Context, runnerID string) error {
	r := d.runner()
	out, err := r.run(ctx, "ps", "--all", "--filter", "label="+sandbox.LabelRunner+"="+runnerID, "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	for _, id := range strings.Fields(string(out)) {
		if _, err := r.run(ctx, "rm", "--force", "--volumes", "--", id); err != nil {
			d.log.Warn("could not remove a sandbox left by an earlier run", "error", err.Error())
		}
	}
	d.unmountLeftovers(ctx)
	return d.removeOwned(ctx, d.attemptsDir)
}

func (d *Driver) removeOwned(ctx context.Context, dir string) error {
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, err := d.runner().run(ctx, "unshare", "rm", "-rf", "--", dir); err != nil {
		return fmt.Errorf("remove %s, which job user namespaces own parts of: %w", dir, err)
	}
	return nil
}

func sandboxName(spec sandbox.PrepareSpec) (string, string) {
	if spec.Seq == 0 {
		return "dhow-" + spec.AttemptID, controlDirName
	}
	seq := strconv.Itoa(spec.Seq)
	return "dhow-" + spec.AttemptID + "-" + seq, controlDirName + "-" + seq
}

func (d *Driver) Prepare(ctx context.Context, spec sandbox.PrepareSpec) (sandbox.Sandbox, error) {
	r := d.runner()
	ws, ok := d.slotOf(spec.Workspace)
	if !ok {
		return nil, fmt.Errorf("%w: the sandbox has no workspace from this driver", sandbox.ErrSandbox)
	}
	d.mu.Lock()
	inner, host := d.slots.innerStart(ws.slot), d.slots.hostStart(ws.slot)
	d.mu.Unlock()
	name, control := sandboxName(spec)
	l := layout{
		name: name, kind: spec.Kind, agentPath: d.agentPath, controlDir: filepath.Join(spec.Dir, control),
		workDir: ws.dir, idStart: inner, hostStart: host,
	}
	if err := config.EnsurePrivateDir(l.controlDir); err != nil {
		return nil, err
	}
	inputs, err := stageInputs(spec.Dir, spec.Seq, spec.Inputs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", sandbox.ErrSandbox, err)
	}
	l.inputsDir = inputs
	imageID, labels, err := d.pull(ctx, spec.Image, spec.AuthFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", sandbox.ErrImagePull, err)
	}
	if _, err := r.run(ctx, createArgs(spec, l, d.paths.seccompFile, imageID)...); err != nil {
		return nil, fmt.Errorf("%w: %w", sandbox.ErrSandbox, err)
	}
	in, err := inspect(ctx, r, l.name)
	if err != nil {
		d.remove(ctx, l.name)
		return nil, fmt.Errorf("%w: %w", sandbox.ErrSandbox, err)
	}
	if err := verify(in, spec, l); err != nil {
		d.remove(ctx, l.name)
		return nil, fmt.Errorf("%w: %w", sandbox.ErrSandbox, err)
	}
	box, err := newBox(ctx, d, l, spec)
	if err == nil {
		box.labels = labels
	}
	if err != nil {
		d.remove(ctx, l.name)
		return nil, fmt.Errorf("%w: %w", sandbox.ErrSandbox, err)
	}
	return box, nil
}

func (d *Driver) pull(ctx context.Context, image, authFile string) (string, map[string]string, error) {
	if !qualified(image) {
		return "", nil, ErrShortImageName
	}
	r := d.runner()
	var out []byte
	var err error
	if d.skipPull {
		out, err = r.run(ctx, "image", "inspect", "--format", "{{.Id}}", "--", image)
	} else {
		args := []string{"pull", "--quiet"}
		if authFile != "" {
			args = append(args, "--authfile", authFile)
		}
		out, err = r.run(ctx, append(args, "--", dockerTransport+image)...)
	}
	if err != nil {
		return "", nil, err
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 || !imageIDPattern.MatchString(lines[len(lines)-1]) {
		return "", nil, fmt.Errorf("%w: %q", ErrImageID, trim(out))
	}
	id := lines[len(lines)-1]
	raw, err := r.run(ctx, "image", "inspect", "--format", "{{json .Labels}}", "--", id)
	if err != nil {
		return "", nil, err
	}
	labels := map[string]string{}
	if err := json.Unmarshal(raw, &labels); err != nil {
		return "", nil, fmt.Errorf("decode the image labels: %w", err)
	}
	return id, labels, nil
}

func staticAgent(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("read the agent binary: %w", err)
	}
	defer func() { _ = f.Close() }()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return ErrDynamicAgent
		}
	}
	return nil
}

func qualified(image string) bool {
	host, _, ok := strings.Cut(image, "/")
	return ok && (strings.ContainsAny(host, ".:") || host == "localhost")
}

func (d *Driver) remove(ctx context.Context, name string) {
	if _, err := d.runner().run(ctx, "rm", "--force", "--volumes", "--", name); err != nil {
		d.log.Warn("could not remove a half-created sandbox", "error", err.Error())
	}
}
