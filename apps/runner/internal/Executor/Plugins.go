package executor

import (
	"context"
	"errors"
	"maps"
	"strconv"

	checkout "github.com/Khaymah/dhow/apps/runner/internal/Checkout"
	plugin "github.com/Khaymah/dhow/apps/runner/internal/Plugin"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (a *attempt) runCheckout(ctx, jobCtx context.Context) (result, bool) {
	co := a.grant.GetSpec().GetCheckout()
	a.pipe.Systemf(nil, "dhow: checkout: %s at %s", co.GetRef(), co.GetSha())
	env := map[string]string{
		checkout.EnvURL: co.GetRepositoryUrl(), checkout.EnvRef: co.GetRef(), checkout.EnvSha: co.GetSha(),
		checkout.EnvDepth: strconv.Itoa(int(co.GetDepth())),
	}
	files := map[string][]byte{}
	if name := co.GetCredentialSecret(); name != "" {
		v, ok := a.secrets[name]
		if !ok {
			a.pipe.Systemf(nil, "dhow: CHECKOUT_FAILED: the server sent no checkout credential")
			return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_FAILED, 0), false
		}
		files[checkout.CredentialFile] = append([]byte(nil), v...)
		env[checkout.EnvCredential] = "1"
	}
	if kh := co.GetKnownHost(); kh != "" {
		files[checkout.KnownHostsFile] = []byte(kh + "\n")
	}
	if co.GetTransport() == runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_HTTPS {
		maps.Copy(env, proxyEnv())
	}
	plan := execPlan{argv: []string{sandbox.AgentPath, checkout.Command}, env: env, files: files, silent: true}
	return a.runStep(ctx, jobCtx, sandbox.CheckoutStepIndex, &runnerv1.Step{Name: "checkout"}, plan)
}

func (a *attempt) silentOutcome(end stepEnd, why result, stopped bool) (result, bool) {
	switch {
	case stopped:
		return why, false
	case end.err != nil:
		a.log.Error("the checkout could not run", "error", a.masked(end.err))
		return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_FAILED, 0), false
	}
	switch end.res.ExitCode {
	case checkout.ExitOK:
		return result{}, true
	case checkout.ExitRefMoved:
		return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_REF_MOVED, end.res.ExitCode), false
	case checkout.ExitHostKeyMismatch:
		return failed(runnerv1.FailureReason_FAILURE_REASON_HOST_KEY_MISMATCH, end.res.ExitCode), false
	case checkout.ExitDiskFull:
		return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_DISK_FULL, end.res.ExitCode), false
	default:
		a.pipe.Systemf(nil, "dhow: CHECKOUT_FAILED: the checkout exited with code %d", end.res.ExitCode)
		return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_FAILED, end.res.ExitCode), false
	}
}

func (a *attempt) runPlugin(ctx, jobCtx context.Context, idx int32) (result, bool) {
	step := a.grant.GetSpec().GetSteps()[idx]
	if r, ok := a.beginStep(ctx, idx, step.GetMutating()); !ok {
		return r, false
	}
	var labels map[string]string
	if l, ok := a.currentBox().(sandbox.Labeled); ok {
		labels = l.ImageLabels()
	}
	m, err := plugin.Parse(labels[sandbox.PluginManifestLabel])
	if err != nil {
		return a.refusePlugin(ctx, idx, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_MANIFEST_INVALID, err)
	}
	env, err := m.Env(step.GetPlugin().GetInputs())
	if err != nil {
		return a.refusePlugin(ctx, idx, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID, err)
	}
	allowed := append([]string{}, m.Outputs...)
	return a.runStep(ctx, jobCtx, idx, step, execPlan{argv: m.Entrypoint, env: env, allowed: allowed})
}

func (a *attempt) refusePlugin(ctx context.Context, idx int32, reason runnerv1.FailureReason, err error) (result, bool) {
	a.pipe.Systemf(&idx, "dhow: %s: %v", reasonName(reason), err)
	a.endStep(ctx, idx, 1, reason)
	return failed(reason, 1), false
}

func (a *attempt) collectOutputs(end stepEnd, allowed []string) stepEnd {
	if len(end.res.Outputs) == 0 {
		return end
	}
	got, err := plugin.ParseOutputs(end.res.Outputs, allowed)
	clear(end.res.Outputs)
	if err != nil {
		end.err = errors.Join(sandbox.ErrOutputs, err)
		return end
	}
	if a.outputs == nil {
		a.outputs = map[string]string{}
	}
	for k, v := range got {
		a.outputs[k] = a.masker.MaskString(v)
	}
	if len(a.outputs) > plugin.MaxOutputs {
		a.outputs = nil
		end.err = sandbox.ErrOutputs
	}
	return end
}
