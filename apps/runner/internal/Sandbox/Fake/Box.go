//go:build dhowtest

package fake

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"sync"
	"syscall"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type box struct {
	d       *Driver
	attempt string
	labels  map[string]string
	mu      sync.Mutex
	killed  chan struct{}
	gone    bool
	running bool
	signals chan syscall.Signal
}

func (b *box) Start(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.gone {
		return sandbox.ErrDestroyed
	}
	b.running = true
	b.d.record(Event{Kind: EventStart, AttemptID: b.attempt})
	return nil
}

func (b *box) ImageLabels() map[string]string { return b.labels }

func (b *box) step(i int32) Step {
	if i == sandbox.CheckoutStepIndex && b.d.rec.Checkout != nil {
		return *b.d.rec.Checkout
	}
	if i >= 0 && int(i) < len(b.d.rec.Steps) {
		return b.d.rec.Steps[i]
	}
	return Step{}
}

func (b *box) Exec(ctx context.Context, spec sandbox.StepSpec) (sandbox.StepResult, error) {
	b.mu.Lock()
	if b.gone || !b.running {
		b.mu.Unlock()
		return sandbox.StepResult{}, sandbox.ErrDestroyed
	}
	signals := make(chan syscall.Signal, 4)
	b.signals = signals
	b.mu.Unlock()
	envNames := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		envNames = append(envNames, k)
	}
	slices.Sort(envNames)
	fileNames := make([]string, 0, len(spec.Files))
	fileData := make([][]byte, 0, len(spec.Files))
	for k, v := range spec.Files {
		fileNames = append(fileNames, k)
		fileData = append(fileData, v)
	}
	slices.Sort(fileNames)
	env := make(map[string]string, len(spec.Env))
	for k, v := range spec.Env {
		env[k] = v
	}
	b.d.record(Event{
		Kind: EventExec, AttemptID: b.attempt, Step: spec.Index, EnvNames: envNames, FileNames: fileNames,
		FileData: fileData, Env: env, Argv: slices.Clone(spec.Argv),
	})
	if spec.Started != nil {
		spec.Started()
	}
	rec := b.step(spec.Index)
	stop := make(chan struct{})
	var once sync.Once
	halt := func() { once.Do(func() { close(stop) }) }
	result := make(chan sandbox.StepResult, 1)
	go b.watch(ctx, signals, stop, halt, rec, result)
	for _, o := range rec.Output {
		if err := sleep(ctx, stop, o.DelayMillis); err != nil {
			return b.finish(spec.Index, <-result), nil
		}
		writeOutput(spec, o)
	}
	if err := sleep(ctx, stop, rec.HangMillis); err != nil {
		return b.finish(spec.Index, <-result), nil
	}
	halt()
	if r, ok := <-result; ok && r.Signal != 0 {
		return b.finish(spec.Index, r), nil
	}
	if rec.BadOutputs {
		return b.finish(spec.Index, sandbox.StepResult{ExitCode: rec.ExitCode}), sandbox.ErrOutputs
	}
	return b.finish(spec.Index, sandbox.StepResult{ExitCode: rec.ExitCode, Reason: reasonOf(rec.Reason), Outputs: []byte(rec.Outputs)}), nil
}

func (b *box) watch(ctx context.Context, signals chan syscall.Signal, stop <-chan struct{}, halt func(), rec Step, result chan<- sandbox.StepResult) {
	defer close(result)
	for {
		select {
		case <-stop:
			return
		case <-b.killed:
			result <- sandbox.StepResult{ExitCode: sandbox.ExitCodeForSignal(syscall.SIGKILL), Signal: syscall.SIGKILL}
			halt()
			return
		case <-ctx.Done():
			result <- sandbox.StepResult{ExitCode: sandbox.ExitCodeForSignal(syscall.SIGKILL), Signal: syscall.SIGKILL}
			halt()
			return
		case sig := <-signals:
			if (sig == syscall.SIGTERM && rec.IgnoreTerm) || (sig == syscall.SIGKILL && rec.IgnoreKill) {
				continue
			}
			result <- sandbox.StepResult{ExitCode: sandbox.ExitCodeForSignal(sig), Signal: sig}
			halt()
			return
		}
	}
}

func (b *box) finish(i int32, r sandbox.StepResult) sandbox.StepResult {
	b.mu.Lock()
	b.signals = nil
	b.mu.Unlock()
	b.d.record(Event{Kind: EventExit, AttemptID: b.attempt, Step: i, Signal: r.Signal})
	return r
}

func writeOutput(spec sandbox.StepSpec, o Output) {
	w := spec.Stdout
	if o.Stream == "stderr" {
		w = spec.Stderr
	}
	data := []byte(o.Data)
	if o.FromEnv != "" {
		data = append(data, encode(spec.Env[o.FromEnv], o.Encoding)...)
	}
	_, _ = w.Write(data)
}

func encode(v, how string) []byte {
	switch how {
	case "base64":
		return []byte(base64.StdEncoding.EncodeToString([]byte(v)))
	case "base64url":
		return []byte(base64.RawURLEncoding.EncodeToString([]byte(v)))
	case "hex":
		return []byte(hex.EncodeToString([]byte(v)))
	case "url":
		return []byte(url.QueryEscape(v))
	case "json":
		raw, _ := json.Marshal(v)
		return raw
	default:
		return []byte(v)
	}
}

func reasonOf(name string) runnerv1.FailureReason {
	if v, ok := runnerv1.FailureReason_value["FAILURE_REASON_"+name]; ok {
		return runnerv1.FailureReason(v)
	}
	return runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED
}

func (b *box) Signal(_ context.Context, sig syscall.Signal) error {
	b.mu.Lock()
	ch := b.signals
	b.mu.Unlock()
	b.d.record(Event{Kind: EventSignal, AttemptID: b.attempt, Signal: sig})
	if ch != nil {
		select {
		case ch <- sig:
		default:
		}
	}
	return nil
}

func (b *box) Kill(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.killed:
		return nil
	default:
	}
	close(b.killed)
	b.running = false
	b.d.record(Event{Kind: EventKill, AttemptID: b.attempt})
	if b.d.rec.KillSurvivors {
		return sandbox.ErrSurvivors
	}
	return nil
}

func (b *box) Destroy(ctx context.Context) error {
	if err := b.Kill(ctx); err != nil && !errors.Is(err, sandbox.ErrSurvivors) {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.gone {
		return nil
	}
	b.gone = true
	b.d.record(Event{Kind: EventDestroy, AttemptID: b.attempt})
	if b.d.rec.DestroySurvivors {
		return sandbox.ErrSurvivors
	}
	return nil
}

func (b *box) Inspect(_ context.Context) (sandbox.Inspection, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return sandbox.Inspection{Running: b.running, Destroyed: b.gone}, nil
}
