//go:build dhowtest

package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	PrepareImagePull = "image_pull"
	PrepareSandbox   = "sandbox"
)

type Output struct {
	Stream      string `json:"Stream"`
	Data        string `json:"Data"`
	FromEnv     string `json:"FromEnv"`
	Encoding    string `json:"Encoding"`
	DelayMillis int    `json:"DelayMillis"`
}

type Step struct {
	Output     []Output `json:"Output"`
	Outputs    string   `json:"Outputs"`
	BadOutputs bool     `json:"BadOutputs"`
	ExitCode   int32    `json:"ExitCode"`
	Reason     string   `json:"Reason"`
	HangMillis int      `json:"HangMillis"`
	IgnoreTerm bool     `json:"IgnoreTerm"`
	IgnoreKill bool     `json:"IgnoreKill"`
}

type Recording struct {
	Steps              []Step                       `json:"Steps"`
	Checkout           *Step                        `json:"Checkout"`
	Images             map[string]map[string]string `json:"Images"`
	WorkspaceError     bool                         `json:"WorkspaceError"`
	PrepareError       string                       `json:"PrepareError"`
	PrepareDelayMillis int                          `json:"PrepareDelayMillis"`
	KillSurvivors      bool                         `json:"KillSurvivors"`
	DestroySurvivors   bool                         `json:"DestroySurvivors"`
}

type EventKind string

const (
	EventCleanup EventKind = "cleanup"
	EventPrepare EventKind = "prepare"
	EventStart   EventKind = "start"
	EventExec    EventKind = "exec"
	EventExit    EventKind = "exit"
	EventSignal  EventKind = "signal"
	EventKill    EventKind = "kill"
	EventDestroy EventKind = "destroy"

	EventWorkspace        EventKind = "workspace"
	EventWorkspaceDestroy EventKind = "workspace_destroy"
)

type Event struct {
	Kind      EventKind
	AttemptID string
	Step      int32
	Signal    syscall.Signal
	EnvNames  []string
	FileNames []string
	FileData  [][]byte
	Env       map[string]string
	Argv      []string
	Image     string
	Box       sandbox.Kind
	Seq       int
	At        time.Time
}

type Driver struct {
	rec      Recording
	mu       sync.Mutex
	events   []Event
	onEvent  func(Event)
	levels   []runnerv1.Isolation
	lastSpec sandbox.PrepareSpec
}

type Options struct {
	OnEvent   func(Event)
	Isolation []runnerv1.Isolation
}

func New(rec Recording, o Options) *Driver {
	levels := o.Isolation
	if len(levels) == 0 {
		levels = []runnerv1.Isolation{runnerv1.Isolation_ISOLATION_CONTAINER}
	}
	return &Driver{rec: rec, onEvent: o.OnEvent, levels: levels}
}

func Load(path string) (Recording, error) {
	if path == "" {
		return Recording{}, nil
	}
	raw, err := os.ReadFile(path) //nolint:gosec // A test recording chosen by the test.
	if err != nil {
		return Recording{}, fmt.Errorf("read the fake recording: %w", err)
	}
	var rec Recording
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Recording{}, fmt.Errorf("parse the fake recording: %w", err)
	}
	return rec, nil
}

func (d *Driver) Name() string { return "fake" }

func (d *Driver) Isolation() []runnerv1.Isolation { return d.levels }

func (d *Driver) record(e Event) {
	e.At = time.Now()
	d.mu.Lock()
	d.events = append(d.events, e)
	cb := d.onEvent
	d.mu.Unlock()
	if cb != nil {
		cb(e)
	}
}

func (d *Driver) Events() []Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Event(nil), d.events...)
}

func (d *Driver) LastSpec() sandbox.PrepareSpec {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastSpec
}

func (d *Driver) Cleanup(_ context.Context, _ string) error {
	d.record(Event{Kind: EventCleanup})
	return nil
}

func (d *Driver) Prepare(ctx context.Context, spec sandbox.PrepareSpec) (sandbox.Sandbox, error) {
	d.mu.Lock()
	d.lastSpec = spec
	d.mu.Unlock()
	d.record(Event{Kind: EventPrepare, AttemptID: spec.AttemptID, Image: spec.Image, Box: spec.Kind, Seq: spec.Seq})
	if spec.Workspace == nil {
		return nil, sandbox.ErrSandbox
	}
	if err := sleep(ctx, nil, d.rec.PrepareDelayMillis); err != nil {
		return nil, err
	}
	switch d.rec.PrepareError {
	case PrepareImagePull:
		return nil, sandbox.ErrImagePull
	case PrepareSandbox:
		return nil, sandbox.ErrSandbox
	}
	return &box{d: d, attempt: spec.AttemptID, labels: d.rec.Images[spec.Image], killed: make(chan struct{})}, nil
}

type workspace struct {
	d       *Driver
	attempt string
	once    sync.Once
}

func (d *Driver) Workspace(_ context.Context, spec sandbox.WorkspaceSpec) (sandbox.Workspace, error) {
	d.record(Event{Kind: EventWorkspace, AttemptID: spec.AttemptID})
	if d.rec.WorkspaceError {
		return nil, sandbox.ErrSandbox
	}
	return &workspace{d: d, attempt: spec.AttemptID}, nil
}

func (w *workspace) Destroy(_ context.Context) error {
	w.once.Do(func() { w.d.record(Event{Kind: EventWorkspaceDestroy, AttemptID: w.attempt}) })
	return nil
}

func sleep(ctx context.Context, stop <-chan struct{}, millis int) error {
	if millis <= 0 {
		return nil
	}
	t := time.NewTimer(time.Duration(millis) * time.Millisecond)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-stop:
		return sandbox.ErrDestroyed
	case <-ctx.Done():
		return fmt.Errorf("fake sandbox: %w", ctx.Err())
	}
}
