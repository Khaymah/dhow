package executor

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"syscall"
	"time"

	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	plugin "github.com/Khaymah/dhow/apps/runner/internal/Executor/Plugin"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	EnvFencingToken = "DHOW_FENCING_TOKEN"
	EnvJobID        = "DHOW_JOB_ID"
	EnvAttemptID    = "DHOW_ATTEMPT_ID"
	EnvStepIndex    = "DHOW_STEP_INDEX"
)

type stepEnd struct {
	res sandbox.StepResult
	err error
	ok  bool
}

type execPlan struct {
	argv    []string
	env     map[string]string
	files   map[string][]byte
	allowed []string
	silent  bool
}

func jobContext(ctx context.Context, spec *runnerv1.JobSpec) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(spec.GetTimeoutSeconds())*time.Second)
}

func (a *attempt) interrupted(jobCtx context.Context) (result, bool) {
	select {
	case <-a.fence.Lost():
		return a.lostResult(), true
	case <-a.fence.Canceled():
		return canceledResult(), true
	case <-a.pipe.Fatal():
		return a.fatalResult(), true
	case <-a.shutdown:
		return lost(runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED), true
	default:
	}
	if jobCtx.Err() != nil {
		return timedOut(), true
	}
	return result{}, false
}

func (a *attempt) beginStep(ctx context.Context, idx int32, mutating bool) (result, bool) {
	err := a.retry(ctx, func(rctx context.Context) error {
		ctrl, err := a.e.o.API.BeginStep(rctx, a.ref, idx, mutating)
		a.fence.observe(ctrl)
		return err
	})
	switch {
	case err == nil:
		return result{}, true
	case client.Is(err, client.CodeCancelRequested):
		return canceledResult(), false
	case errors.Is(err, ErrSelfFenced):
		return lost(runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST), false
	case client.IsLeaseLost(err), errors.Is(err, ErrLeaseLost):
		a.fence.markLost(ErrLeaseLost)
		return gone(), false
	case a.stopping():
		return lost(runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED), false
	default:
		a.log.Error("the server refused to begin a step", "step", idx, "code", client.Code(err).String(), "reason", client.Reason(err))
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
}

func (a *attempt) runStep(ctx, jobCtx context.Context, idx int32, step *runnerv1.Step, plan execPlan) (result, bool) {
	logIdx := &idx
	if plan.silent {
		logIdx = nil
	} else {
		a.pipe.Systemf(logIdx, "dhow: step %d: %s", idx+1, step.GetName())
	}
	stdout := a.pipe.Writer(runnerv1.LogStream_LOG_STREAM_STDOUT, logIdx)
	stderr := a.pipe.Writer(runnerv1.LogStream_LOG_STREAM_STDERR, logIdx)
	stepCtx := jobCtx
	if t := step.GetTimeoutSeconds(); t > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(jobCtx, time.Duration(t)*time.Second)
		defer cancel()
	}
	execCtx, stopExec := context.WithCancel(ctx)
	defer stopExec()
	env, files := plan.env, plan.files
	if !plan.silent {
		env, files = a.stepEnv(idx)
		maps.Copy(env, plan.env)
	}
	done := make(chan stepEnd, 1)
	box := a.currentBox()
	go func() {
		res, err := box.Exec(execCtx, sandbox.StepSpec{
			Index: idx, Name: step.GetName(), Script: step.GetRun(), Argv: plan.argv, Env: env, Files: files, Stdout: stdout, Stderr: stderr,
		})
		done <- stepEnd{res: res, err: err, ok: true}
	}()
	end, why, stopped := a.waitStep(ctx, jobCtx, stepCtx, done, stopExec)
	if stopped {
		a.pipe.Unblock()
	}
	_ = stdout.Close()
	_ = stderr.Close()
	for _, v := range files {
		clear(v)
	}
	if end.ok {
		clear(env)
		clear(files)
	}
	if plan.silent {
		return a.silentOutcome(end, why, stopped)
	}
	if !stopped && end.err == nil && end.res.ExitCode == 0 {
		end = a.collectOutputs(end, plan.allowed)
	}
	stepReason := stepReasonOf(end, why, stopped)
	if !stopped || (why.state != runnerv1.AttemptResult_ATTEMPT_RESULT_LOST && !why.silent) {
		a.endStep(ctx, idx, end.res.ExitCode, stepReason)
	}
	if stopped {
		if why.state == runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED && why.reason == runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED {
			why.exitCode = end.res.ExitCode
			a.pipe.Systemf(&idx, "dhow: step %d ran past its timeout and was stopped", idx+1)
		}
		return why, false
	}
	return a.stepOutcome(idx, end)
}

func (a *attempt) waitStep(ctx, jobCtx, stepCtx context.Context, done <-chan stepEnd, stopExec func()) (stepEnd, result, bool) {
	select {
	case end := <-done:
		return end, result{}, false
	case <-a.fence.Lost():
		return a.awaitExit(done, stopExec), a.lostResult(), true
	case <-a.fence.Canceled():
		return a.terminate(ctx, done, stopExec), canceledResult(), true
	case <-stepCtx.Done():
		end := a.terminate(ctx, done, stopExec)
		if jobCtx.Err() != nil {
			return end, timedOut(), true
		}
		return end, failed(runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, end.res.ExitCode), true
	case <-a.pipe.Fatal():
		a.killBox(ctx)
		return a.awaitExit(done, stopExec), a.fatalResult(), true
	case <-a.shutdown:
		a.killBox(ctx)
		return a.awaitExit(done, stopExec), lost(runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED), true
	}
}

func stepReasonOf(end stepEnd, why result, stopped bool) runnerv1.FailureReason {
	switch {
	case stopped && why.state == runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED:
		return runnerv1.FailureReason_FAILURE_REASON_CANCELED
	case stopped && (why.state == runnerv1.AttemptResult_ATTEMPT_RESULT_TIMED_OUT || why.reason == runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED):
		return runnerv1.FailureReason_FAILURE_REASON_TIMED_OUT
	case end.err != nil && errors.Is(end.err, sandbox.ErrShellMissing):
		return runnerv1.FailureReason_FAILURE_REASON_SHELL_NOT_FOUND
	case end.err != nil && errors.Is(end.err, sandbox.ErrOutputs):
		return runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED
	case end.res.ExitCode == 0 && end.err == nil:
		return runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED
	case end.res.Reason != runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED:
		return end.res.Reason
	default:
		return runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED
	}
}

func (a *attempt) stepOutcome(idx int32, end stepEnd) (result, bool) {
	switch {
	case end.err != nil && errors.Is(end.err, sandbox.ErrShellMissing):
		a.pipe.Systemf(&idx, "dhow: SHELL_NOT_FOUND: the job image has no /bin/sh")
		return failed(runnerv1.FailureReason_FAILURE_REASON_SHELL_NOT_FOUND, 0), false
	case end.err != nil && errors.Is(end.err, sandbox.ErrOutputs):
		a.pipe.Systemf(&idx, "dhow: step %d wrote $DHOW_OUTPUT outside the limits (key=value lines, %d keys, %d bytes a value)", idx+1, plugin.MaxOutputs, plugin.MaxOutputValue)
		return failed(runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, 1), false
	case end.err != nil:
		a.log.Error("a step could not run", "step", idx, "error", a.masked(end.err))
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	case end.res.ExitCode != 0:
		reason := end.res.Reason
		if reason == runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED {
			reason = runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED
		}
		a.pipe.Systemf(&idx, "dhow: step %d exited with code %d", idx+1, end.res.ExitCode)
		return failed(reason, end.res.ExitCode), false
	default:
		return result{}, true
	}
}

func (a *attempt) endStep(ctx context.Context, idx, exitCode int32, reason runnerv1.FailureReason) {
	err := a.retry(ctx, func(rctx context.Context) error {
		ctrl, err := a.e.o.API.EndStep(rctx, a.ref, idx, exitCode, reason)
		a.fence.observe(ctrl)
		return err
	})
	switch {
	case err == nil:
	case client.IsLeaseLost(err):
		a.fence.markLost(ErrLeaseLost)
	default:
		a.log.Warn("could not report the end of a step", "step", idx, "code", client.Code(err).String(), "reason", client.Reason(err))
	}
}

func (a *attempt) killBox(ctx context.Context) {
	if box := a.currentBox(); box != nil {
		a.mu.Lock()
		if a.killedAt.IsZero() {
			a.killedAt = time.Now()
		}
		a.mu.Unlock()
		if err := box.Kill(ctx); err != nil {
			a.log.Error("could not kill the sandbox", "error", a.masked(err))
			a.noteSurvivors(err)
		}
	}
}

func (a *attempt) noteSurvivors(err error) {
	if !errors.Is(err, sandbox.ErrSurvivors) {
		return
	}
	a.mu.Lock()
	seen := a.survivors
	a.survivors = true
	a.mu.Unlock()
	if !seen && a.pipe != nil {
		a.pipe.Systemf(nil, "dhow: SANDBOX_FAILED: processes of the job survived the kill")
	}
}

func waitFor(done <-chan stepEnd, d time.Duration) (stepEnd, bool) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case end := <-done:
		return end, true
	case <-t.C:
		return stepEnd{}, false
	}
}

func (a *attempt) terminate(ctx context.Context, done <-chan stepEnd, stopExec func()) stepEnd {
	box := a.currentBox()
	_ = box.Signal(ctx, syscall.SIGTERM)
	if end, ok := waitFor(done, a.e.o.Config.CancelGrace); ok {
		return end
	}
	_ = box.Signal(ctx, syscall.SIGKILL)
	if end, ok := waitFor(done, a.e.o.Timing.CancelKillWait); ok {
		return end
	}
	a.killBox(ctx)
	return a.awaitExit(done, stopExec)
}

func (a *attempt) awaitExit(done <-chan stepEnd, stopExec func()) stepEnd {
	if end, ok := waitFor(done, a.e.o.Timing.CancelKillWait); ok {
		return end
	}
	stopExec()
	end, _ := waitFor(done, a.e.o.Timing.CancelKillWait)
	return end
}

func (a *attempt) stepEnv(idx int32) (map[string]string, map[string][]byte) {
	spec := a.grant.GetSpec()
	env := map[string]string{}
	maps.Copy(env, spec.GetEnv())
	maps.Copy(env, spec.GetSteps()[idx].GetEnv())
	env[EnvFencingToken] = strconv.FormatInt(a.ref.GetFencingToken(), 10)
	env[EnvJobID] = a.grant.GetJobId()
	env[EnvAttemptID] = a.grant.GetAttemptId()
	env[EnvStepIndex] = strconv.Itoa(int(idx))
	env[plugin.EnvOutput] = sandbox.OutputPath
	files := map[string][]byte{}
	for _, ref := range spec.GetSecrets() {
		if ref.GetName() == jobspec.CheckoutSecret {
			continue
		}
		if len(ref.GetStepIndexes()) > 0 && !slices.Contains(ref.GetStepIndexes(), idx) {
			continue
		}
		v := a.secrets[ref.GetName()]
		switch t := ref.GetTarget().(type) {
		case *runnerv1.SecretRef_AsEnv:
			env[t.AsEnv] = string(v)
		case *runnerv1.SecretRef_AsFile:
			files[t.AsFile] = append([]byte(nil), v...)
		}
	}
	if len(spec.GetNetwork().GetEgress()) > 0 {
		maps.Copy(env, proxyEnv())
	}
	return env, files
}
