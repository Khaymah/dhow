package agent

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	agentv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/agent/v1"
)

const (
	outputChunkBytes = 32 << 10
	defaultPath      = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

type process struct {
	cmd *exec.Cmd
	mu  sync.Mutex
}

func (p *process) signal(sig unix.Signal) {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = unix.Kill(-cmd.Process.Pid, sig)
}

type job struct {
	index   int32
	argv    []string
	script  string
	shell   bool
	env     map[string]string
	secrets []*agentv1.SecretFile
}

func stepOf(s *agentv1.RunStep) job {
	return job{index: s.GetIndex(), argv: []string{Shell, "-e", "-s"}, script: s.GetScript(), shell: true, env: s.GetEnv(), secrets: s.GetSecretFiles()}
}

func execOf(e *agentv1.RunExec) job {
	return job{index: e.GetIndex(), argv: e.GetArgv(), env: e.GetEnv(), secrets: e.GetSecretFiles()}
}

func (a *Agent) runStep(ctx context.Context, step job) error {
	if len(step.argv) == 0 || step.argv[0] == "" {
		return a.conn.Write(stepExit(step.index, 1, 0, agentv1.StepFailure_STEP_FAILURE_START_FAILED, nil))
	}
	secrets, err := writeSecretFiles(a.secretsDir, step.secrets)
	if err != nil {
		return a.conn.Write(stepExit(step.index, 1, 0, agentv1.StepFailure_STEP_FAILURE_SECRET_FILES, nil))
	}
	if !clearOutput(a.outputPath) {
		wipeSecretFiles(a.secretsDir, secrets)
		return a.conn.Write(stepExit(step.index, 1, 0, agentv1.StepFailure_STEP_FAILURE_OUTPUTS, nil))
	}
	code, sig, failure, err := a.runProcess(ctx, step)
	wipeSecretFiles(a.secretsDir, secrets)
	if err != nil {
		return err
	}
	outputs, ok := readOutput(a.outputPath)
	if !ok && failure == agentv1.StepFailure_STEP_FAILURE_UNSPECIFIED {
		failure = agentv1.StepFailure_STEP_FAILURE_OUTPUTS
	}
	return a.conn.Write(stepExit(step.index, code, sig, failure, outputs))
}

func (a *Agent) runProcess(ctx context.Context, step job) (int32, int32, agentv1.StepFailure, error) {
	cmd := exec.CommandContext(ctx, step.argv[0], step.argv[1:]...) //nolint:gosec // The runner builds every argv itself: the shell, the checkout or a plugin entrypoint.
	cmd.Env = stepEnv(step.env)
	if step.shell {
		cmd.Stdin = stdinReader(step.script)
	}
	cmd.SysProcAttr = &unix.SysProcAttr{Setpgid: true}
	outR, outW, err := os.Pipe()
	if err != nil {
		return 1, 0, agentv1.StepFailure_STEP_FAILURE_START_FAILED, nil
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		closeAll(outR, outW)
		return 1, 0, agentv1.StepFailure_STEP_FAILURE_START_FAILED, nil
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	startErr := cmd.Start()
	closeAll(outW, errW)
	if startErr != nil {
		closeAll(outR, errR)
		return 1, 0, startFailure(startErr), nil
	}
	a.setCurrent(&process{cmd: cmd})
	var wg sync.WaitGroup
	wg.Add(2)
	go a.pump(&wg, step.index, agentv1.OutputStream_OUTPUT_STREAM_STDOUT, outR)
	go a.pump(&wg, step.index, agentv1.OutputStream_OUTPUT_STREAM_STDERR, errR)
	waitErr := cmd.Wait()
	a.clearCurrent()
	sweepErr := a.sweep()
	if sweepErr != nil {
		closeAll(outR, errR)
	}
	wg.Wait()
	closeAll(outR, errR)
	if sweepErr != nil {
		return 0, 0, 0, sweepErr
	}
	code, sig := exitStatus(waitErr)
	return code, sig, agentv1.StepFailure_STEP_FAILURE_UNSPECIFIED, nil
}

func closeAll(files ...*os.File) {
	for _, f := range files {
		_ = f.Close()
	}
}

func startFailure(err error) agentv1.StepFailure {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return agentv1.StepFailure_STEP_FAILURE_SHELL_NOT_FOUND
	}
	return agentv1.StepFailure_STEP_FAILURE_START_FAILED
}

func (a *Agent) pump(wg *sync.WaitGroup, index int32, stream agentv1.OutputStream, r io.Reader) {
	defer wg.Done()
	buf := make([]byte, outputChunkBytes)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_ = a.conn.Write(&agentv1.Frame{Body: &agentv1.Frame_Output{Output: &agentv1.Output{
				StepIndex: index, Stream: stream, Data: append([]byte(nil), buf[:n]...),
			}}})
		}
		if err != nil {
			return
		}
	}
}

func stepEnv(env map[string]string) []string {
	out := make([]string, 0, len(env)+1)
	keys := make([]string, 0, len(env))
	hasPath := false
	for k := range env {
		keys = append(keys, k)
		if k == "PATH" {
			hasPath = true
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	if !hasPath {
		out = append(out, "PATH="+defaultPath)
	}
	return out
}

func stdinReader(script string) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, script)
		_ = pw.Close()
	}()
	return pr
}

func exitStatus(err error) (int32, int32) {
	if err == nil {
		return 0, 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		ws, ok := ee.Sys().(syscall.WaitStatus)
		if ok && ws.Signaled() {
			sig := int32(ws.Signal()) //nolint:gosec // Signal numbers are below 64.
			return 128 + sig, sig
		}
		return int32(ee.ExitCode()), 0 //nolint:gosec // Process exit codes are in the range 0-255.
	}
	return 1, 0
}

func stepExit(index, code, sig int32, failure agentv1.StepFailure, outputs []byte) *agentv1.Frame {
	return &agentv1.Frame{Body: &agentv1.Frame_StepExit{StepExit: &agentv1.StepExit{
		Index: index, ExitCode: code, Signal: sig, Failure: failure, Outputs: outputs,
	}}}
}
