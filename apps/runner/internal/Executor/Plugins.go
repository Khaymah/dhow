package executor

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"

	plugin "github.com/Khaymah/dhow/apps/runner/internal/Executor/Plugin"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (a *attempt) runCheckout(ctx, jobCtx context.Context) (result, bool) {
	co := a.grant.GetSpec().GetCheckout()
	a.pipe.Systemf(nil, "dhow: checkout: %s at %s", co.GetRef(), co.GetSha())
	env := map[string]string{
		sandbox.CheckoutEnvURL: co.GetRepositoryUrl(), sandbox.CheckoutEnvRef: co.GetRef(), sandbox.CheckoutEnvSha: co.GetSha(),
		sandbox.CheckoutEnvDepth: strconv.Itoa(int(co.GetDepth())),
	}
	files := map[string][]byte{}
	if name := co.GetCredentialSecret(); name != "" {
		v, ok := a.secrets[name]
		if !ok {
			a.pipe.Systemf(nil, "dhow: CHECKOUT_FAILED: the server sent no checkout credential")
			return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_FAILED, 0), false
		}
		files[sandbox.CheckoutCredentialFile] = append([]byte(nil), v...)
		env[sandbox.CheckoutEnvCredential] = "1"
	}
	if kh := co.GetKnownHost(); kh != "" {
		files[sandbox.CheckoutKnownHostsFile] = []byte(kh + "\n")
	}
	if co.GetTransport() == runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_HTTPS {
		maps.Copy(env, proxyEnv())
	}
	plan := execPlan{argv: []string{sandbox.AgentPath, sandbox.CheckoutCommand}, env: env, files: files, silent: true}
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
	case sandbox.CheckoutExitOK:
		return result{}, true
	case sandbox.CheckoutExitRefMoved:
		return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_REF_MOVED, end.res.ExitCode), false
	case sandbox.CheckoutExitHostKeyMismatch:
		return failed(runnerv1.FailureReason_FAILURE_REASON_HOST_KEY_MISMATCH, end.res.ExitCode), false
	case sandbox.CheckoutExitDiskFull:
		return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_DISK_FULL, end.res.ExitCode), false
	default:
		a.pipe.Systemf(nil, "dhow: CHECKOUT_FAILED: the checkout exited with code %d", end.res.ExitCode)
		return failed(runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_FAILED, end.res.ExitCode), false
	}
}

func (a *attempt) runPlugin(ctx, jobCtx context.Context, idx int32) (result, bool) {
	step := a.grant.GetSpec().GetSteps()[idx]
	var labels map[string]string
	if l, ok := a.currentBox().(sandbox.Labeled); ok {
		labels = l.ImageLabels()
	}
	m, parseErr := plugin.Parse(labels[sandbox.PluginManifestLabel])
	if r, ok := a.beginStep(ctx, idx, step.GetMutating() || (parseErr == nil && m.Mutating)); !ok {
		return r, false
	}
	if parseErr != nil {
		return a.refusePlugin(ctx, idx, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_MANIFEST_INVALID, parseErr)
	}
	defer a.dropArchives(a.spentArchives(idx))
	with, env, files := a.pluginEnv(idx, step)
	inputs, err := m.Env(with)
	if err != nil {
		clear(with)
		return a.refusePlugin(ctx, idx, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID, err)
	}
	delivered, err := a.artifactEnv(m, with, idx)
	clear(with)
	if err != nil {
		return a.refusePlugin(ctx, idx, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID, err)
	}
	target, err := a.targetEnv(idx)
	if err != nil {
		return a.refusePlugin(ctx, idx, runnerv1.FailureReason_FAILURE_REASON_SPEC_REJECTED, err)
	}
	maps.Copy(env, inputs)
	maps.Copy(env, delivered)
	maps.Copy(env, target)
	allowed := append([]string{}, m.Outputs...)
	return a.runStep(ctx, jobCtx, idx, step, execPlan{argv: m.Entrypoint, env: env, files: files, allowed: allowed, isolated: true, report: target != nil})
}

func (a *attempt) pluginEnv(idx int32, step *runnerv1.Step) (map[string]string, map[string]string, map[string][]byte) {
	spec := a.grant.GetSpec()
	with := maps.Clone(step.GetPlugin().GetInputs())
	if with == nil {
		with = map[string]string{}
	}
	env := map[string]string{plugin.EnvOutput: sandbox.OutputPath}
	maps.Copy(env, step.GetEnv())
	files := map[string][]byte{}
	for _, ref := range spec.GetSecrets() {
		if ref.GetName() == jobspec.CheckoutSecret || !slices.Contains(ref.GetStepIndexes(), idx) || foreignTargetKey(spec, ref.GetName(), idx) {
			continue
		}
		v := a.secrets[ref.GetName()]
		switch t := ref.GetTarget().(type) {
		case *runnerv1.SecretRef_AsEnv:
			if name, ok := strings.CutPrefix(t.AsEnv, jobspec.PluginInputEnvPrefix); ok {
				with[strings.ToLower(name)] = string(v)
				continue
			}
			env[t.AsEnv] = string(v)
		case *runnerv1.SecretRef_AsFile:
			files[t.AsFile] = append([]byte(nil), v...)
		}
	}
	if usesProxy(spec) {
		maps.Copy(env, proxyEnv())
	}
	return with, env, files
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
		a.outputs[k] = v
	}
	if len(a.outputs) > plugin.MaxOutputs {
		a.outputs = nil
		end.err = sandbox.ErrOutputs
	}
	return end
}
