package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	wire "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Wire"
	agentv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/agent/v1"
)

const (
	ControlSocketPath = "/run/dhow/control/control.sock"
	SecretsDir        = "/run/dhow/secrets"
	OutputPath        = sandbox.OutputPath
	Shell             = "/bin/sh"
	agentVersion      = "1"
	dialTimeout       = 30 * time.Second
)

var ErrChannelClosed = errors.New("the control channel closed, so the agent exits and the container with it")

type Options struct {
	SocketPath   string
	Stdio        Stdio
	Dialer       func(ctx context.Context, path string) (io.ReadWriteCloser, error)
	Harden       func() error
	Sweep        func() error
	SecretsDir   string
	OutputPath   string
	EgressSocket string
	EgressAddrs  []string
	Clock        func() time.Time
}

type Stdio struct {
	Out io.Writer
	Err io.Writer
}

type Agent struct {
	conn       *wire.Conn
	harden     func() error
	sweep      func() error
	secretsDir string
	outputPath string
	mu         sync.Mutex
	current    *process
	input      *os.File
	inputStep  int32
	stopping   bool
	busy       bool
}

func Run(ctx context.Context, o Options) error {
	if o.SocketPath == "" {
		o.SocketPath = ControlSocketPath
	}
	if o.Harden == nil {
		o.Harden = hardenAgent
	}
	if o.Dialer == nil {
		o.Dialer = dialUnix
	}
	if o.SecretsDir == "" {
		o.SecretsDir = SecretsDir
	}
	if o.OutputPath == "" {
		o.OutputPath = OutputPath
	}
	if o.EgressSocket == "" {
		o.EgressSocket = EgressSocketPath
	}
	if o.EgressAddrs == nil {
		o.EgressAddrs = egressAddrs()
	}
	if o.Sweep == nil {
		sweep, err := defaultSweep()
		if err != nil {
			return err
		}
		o.Sweep = sweep
	}
	if err := o.Harden(); err != nil {
		return err
	}
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	rwc, err := o.Dialer(dctx, o.SocketPath)
	if err != nil {
		return fmt.Errorf("connect to the runner control channel: %w", err)
	}
	a := &Agent{conn: wire.NewConn(rwc), harden: o.Harden, sweep: o.Sweep, secretsDir: o.SecretsDir, outputPath: o.OutputPath}
	defer func() { _ = a.conn.Close() }()
	stopForwarder, err := startForwarder(ctx, o.EgressSocket, o.EgressAddrs)
	if err != nil {
		return err
	}
	defer stopForwarder()
	if err := a.conn.Write(hello()); err != nil {
		return fmt.Errorf("greet the runner: %w", err)
	}
	return a.loop(ctx)
}

func hello() *agentv1.Frame {
	return &agentv1.Frame{Body: &agentv1.Frame_Hello{Hello: &agentv1.Hello{
		ProtocolVersion: wire.ProtocolVersion, AgentVersion: agentVersion,
	}}}
}

func (a *Agent) loop(ctx context.Context) error {
	var steps sync.WaitGroup
	defer steps.Wait()
	for {
		frame, err := a.conn.Read()
		if err != nil {
			a.stop()
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return ErrChannelClosed
			}
			return fmt.Errorf("read a control frame: %w", err)
		}
		switch body := frame.GetBody().(type) {
		case *agentv1.Frame_RunStep:
			if err := a.dispatch(ctx, &steps, stepOf(body.RunStep)); err != nil {
				return err
			}
		case *agentv1.Frame_RunExec:
			if err := a.dispatch(ctx, &steps, execOf(body.RunExec)); err != nil {
				return err
			}
		case *agentv1.Frame_Input:
			a.feed(body.Input)
		case *agentv1.Frame_Signal:
			a.killCurrent(signalOf(body.Signal.GetSignal()))
		case *agentv1.Frame_Shutdown:
			a.stop()
			return nil
		case *agentv1.Frame_AddMask:
		default:
		}
	}
}

func (a *Agent) dispatch(ctx context.Context, steps *sync.WaitGroup, step job) error {
	if !a.claim() {
		return a.conn.Write(stepExit(step.index, 1, 0, agentv1.StepFailure_STEP_FAILURE_START_FAILED, nil))
	}
	if step.stdin {
		r, w, err := os.Pipe()
		if err != nil {
			a.release()
			return a.conn.Write(stepExit(step.index, 1, 0, agentv1.StepFailure_STEP_FAILURE_START_FAILED, nil))
		}
		step.input = r
		a.mu.Lock()
		a.input, a.inputStep = w, step.index
		a.mu.Unlock()
	}
	steps.Go(func() {
		defer a.closeInput()
		exit, err := a.runStep(ctx, step)
		if err != nil {
			_ = a.conn.Close()
			return
		}
		a.release()
		if err := a.conn.Write(exit); err != nil {
			_ = a.conn.Close()
		}
	})
	return nil
}

func (a *Agent) feed(in *agentv1.Input) {
	a.mu.Lock()
	w, step := a.input, a.inputStep
	a.mu.Unlock()
	if w == nil || step != in.GetStepIndex() {
		return
	}
	if len(in.GetData()) > 0 {
		if _, err := w.Write(in.GetData()); err != nil {
			a.closeInput()
			return
		}
	}
	if in.GetEof() {
		a.closeInput()
	}
}

func (a *Agent) closeInput() {
	a.mu.Lock()
	w := a.input
	a.input = nil
	a.mu.Unlock()
	if w != nil {
		_ = w.Close()
	}
}

func signalOf(s agentv1.ProcessSignal) unix.Signal {
	switch s {
	case agentv1.ProcessSignal_PROCESS_SIGNAL_KILL:
		return unix.SIGKILL
	default:
		return unix.SIGTERM
	}
}

func (a *Agent) killCurrent(sig unix.Signal) {
	a.mu.Lock()
	p := a.current
	a.mu.Unlock()
	if p != nil {
		p.signal(sig)
	}
}

func hardenAgent() error {
	ignoreJobSignals()
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("turn off ptrace inside the job (PR_SET_DUMPABLE): %w", err)
	}
	return nil
}

func dialUnix(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	var d net.Dialer
	for {
		conn, err := d.DialContext(ctx, "unix", path)
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (a *Agent) setCurrent(p *process) {
	a.mu.Lock()
	a.current = p
	stopping := a.stopping
	a.mu.Unlock()
	if stopping {
		p.signal(unix.SIGKILL)
	}
}

func (a *Agent) stop() {
	a.mu.Lock()
	a.stopping = true
	a.mu.Unlock()
	a.killCurrent(unix.SIGKILL)
}

func (a *Agent) claim() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy {
		return false
	}
	a.busy = true
	return true
}

func (a *Agent) release() {
	a.mu.Lock()
	a.busy = false
	a.mu.Unlock()
}

func (a *Agent) clearCurrent() {
	a.mu.Lock()
	a.current = nil
	a.mu.Unlock()
}
