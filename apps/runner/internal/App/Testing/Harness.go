//go:build dhowtest

package runnertest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	app "github.com/Khaymah/dhow/apps/runner/internal/App"
	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	executor "github.com/Khaymah/dhow/apps/runner/internal/Executor"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	logs "github.com/Khaymah/dhow/apps/runner/internal/Logs"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	TestImage   = "registry.example.com/dhow/test@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	stopTimeout = 30 * time.Second
)

type Env map[string]string

func (e Env) Get(k string) string { return e[k] }

type Runner struct {
	T        testing.TB
	Server   *Server
	StateDir string
	Env      Env
	Sealer   *Sealer
}

func FastBackoff() client.Backoff {
	return client.Backoff{Min: 20 * time.Millisecond, Max: 200 * time.Millisecond}
}

func FastTiming() executor.Timing {
	t := executor.DefaultTiming()
	t.CancelKillWait = 300 * time.Millisecond
	t.CompleteWindow = 5 * time.Second
	t.ShutdownGrace = 2 * time.Second
	t.LogDrain = 5 * time.Second
	return t
}

func Register(t testing.TB, srv *Server, url string) *Runner {
	t.Helper()
	base := t.TempDir()
	state := filepath.Join(base, "state")
	env := Env{
		config.EnvStateDir: state, config.EnvCAFile: srv.CAFile, config.EnvServerOrigin: Origin, config.EnvDriver: config.DriverFake,
		config.EnvCancelGrace: "1s",
	}
	tokenFile := filepath.Join(base, "token")
	if err := os.WriteFile(tokenFile, []byte(srv.Token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	err := app.Main(context.Background(), []string{"register", "--url", url, "--token-file", tokenFile, "--name", "test-runner"}, app.Env{
		Getenv: env.Get, Stdio: app.Stdio{In: bytes.NewReader(nil), Out: &out, Err: &errOut},
		Machine: func() (config.Machine, error) { return config.Machine{}, nil }, Harden: func() error { return nil },
	})
	if err != nil {
		t.Fatalf("register: %v (%s)", err, errOut.String())
	}
	return &Runner{T: t, Server: srv, StateDir: state, Env: env}
}

func (r *Runner) Config() config.Config {
	r.T.Helper()
	cfg, err := config.Load(r.Env.Get, config.Options{})
	if err != nil {
		r.T.Fatal(err)
	}
	return cfg
}

func (r *Runner) Identity() *identity.Identity {
	r.T.Helper()
	id, err := r.Store().Load()
	if err != nil {
		r.T.Fatal(err)
	}
	return id
}

type Running struct {
	Driver   *fake.Driver
	Outcomes chan executor.Outcome
	cancel   context.CancelFunc
	done     chan error
}

type StartOptions struct {
	URL       string
	Timing    *executor.Timing
	Logs      func(*logs.Options)
	Workspace string
}

func (r *Runner) Start(rec fake.Recording, o StartOptions) *Running {
	r.T.Helper()
	cfg := r.Config()
	if o.URL != "" {
		u, err := config.ParseServerURL(o.URL, false)
		if err != nil {
			r.T.Fatal(err)
		}
		cfg.URL = u
	}
	timing := FastTiming()
	if o.Timing != nil {
		timing = *o.Timing
	}
	driver := fake.New(rec, fake.Options{WorkspaceRoot: o.Workspace})
	outcomes := make(chan executor.Outcome, 16)
	runner, err := app.NewRunner(app.RunnerOptions{
		Config: cfg, Identity: r.Identity(), Driver: driver, Timing: timing, Backoff: FastBackoff(), Wait: time.Second,
		Outcomes: outcomes, Logs: o.Logs,
	})
	if err != nil {
		r.T.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	running := &Running{Driver: driver, Outcomes: outcomes, cancel: cancel, done: make(chan error, 1)}
	go func() { running.done <- runner.Run(ctx) }()
	r.T.Cleanup(func() { _ = running.Stop() })
	return running
}

func (r *Running) Stop() error {
	r.cancel()
	select {
	case err := <-r.done:
		r.done <- err
		return err
	case <-time.After(stopTimeout):
		return context.DeadlineExceeded
	}
}

func (r *Running) Ended(t testing.TB, within time.Duration) error {
	t.Helper()
	select {
	case err := <-r.done:
		r.done <- err
		return err
	case <-time.After(within):
		t.Fatalf("the runner still runs after %s", within)
		return nil
	}
}

func (r *Running) Next(t testing.TB, within time.Duration) executor.Outcome {
	t.Helper()
	select {
	case out := <-r.Outcomes:
		return out
	case <-time.After(within):
		t.Fatalf("no attempt finished within %s", within)
		return executor.Outcome{}
	}
}

func Spec(steps ...*runnerv1.Step) *runnerv1.JobSpec {
	if len(steps) == 0 {
		steps = []*runnerv1.Step{{Name: "build", Run: "make build"}}
	}
	return jobspec.Normalize(&runnerv1.JobSpec{Image: TestImage, Steps: steps})
}

func WaitEvent(t testing.TB, d *fake.Driver, kind fake.EventKind, within time.Duration) fake.Event {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		for _, e := range d.Events() {
			if e.Kind == kind {
				return e
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no %s event within %s", kind, within)
	return fake.Event{}
}

func Eventually(t testing.TB, within time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
