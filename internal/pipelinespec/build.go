package pipelinespec

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var ErrSecretScope = errors.New("a step holds a secret that was not declared on it")

func (r *RunJob) Build(ctx context.Context, c Context) (*runnerv1.JobSpec, error) {
	spec, ok := proto.Clone(r.Spec).(*runnerv1.JobSpec)
	if !ok {
		return nil, ErrExpression
	}
	if spec.Env == nil {
		spec.Env = map[string]string{}
	}
	for ref := range r.Expressions {
		v, err := r.EvalValue(ctx, ref, c)
		if err != nil {
			return nil, err
		}
		placeValue(spec, ref, v)
	}
	c.Env = spec.GetEnv()
	kept := make([]*runnerv1.Step, 0, len(spec.GetSteps()))
	origin := make([]int, 0, len(spec.GetSteps()))
	moved := make(map[int32]int32, len(spec.GetSteps()))
	next := int32(0)
	for i := int32(0); int(i) < len(spec.GetSteps()); i++ {
		keep, err := r.EvalStepIf(ctx, int(i), c)
		if err != nil {
			return nil, err
		}
		if keep {
			moved[i] = next
			next++
			origin = append(origin, int(i))
			kept = append(kept, spec.GetSteps()[i])
		}
	}
	spec.Steps = kept
	spec.Secrets = remapSecrets(spec.GetSecrets(), moved)
	if err := r.checkStepSecrets(spec, origin); err != nil {
		return nil, err
	}
	return spec, nil
}

func remapSecrets(refs []*runnerv1.SecretRef, moved map[int32]int32) []*runnerv1.SecretRef {
	out := make([]*runnerv1.SecretRef, 0, len(refs))
	for _, ref := range refs {
		if len(ref.GetStepIndexes()) == 0 {
			out = append(out, ref)
			continue
		}
		steps := make([]int32, 0, len(ref.GetStepIndexes()))
		for _, old := range ref.GetStepIndexes() {
			if n, ok := moved[old]; ok {
				steps = append(steps, n)
			}
		}
		if len(steps) == 0 {
			continue
		}
		ref.StepIndexes = steps
		out = append(out, ref)
	}
	return out
}

func (r *RunJob) checkStepSecrets(spec *runnerv1.JobSpec, origin []int) error {
	for _, ref := range spec.GetSecrets() {
		for _, n := range ref.GetStepIndexes() {
			if n < 0 || int(n) >= len(origin) || origin[n] >= len(r.Steps) || !r.Steps[origin[n]].declares(ref) {
				return ErrSecretScope
			}
		}
	}
	return nil
}

func (p StepPlan) declares(ref *runnerv1.SecretRef) bool {
	target, ok := ref.GetTarget().(*runnerv1.SecretRef_AsEnv)
	if !ok || ref.GetName() == "" {
		return false
	}
	if input, found := strings.CutPrefix(target.AsEnv, jobspec.PluginInputEnvPrefix); found {
		name := strings.ToLower(input)
		return p.Plugin != nil && strings.ToUpper(name) == input && p.Plugin.Secrets[name] == ref.GetName()
	}
	return p.Secrets[target.AsEnv] == ref.GetName()
}

func placeValue(spec *runnerv1.JobSpec, ref, v string) {
	if name, found := strings.CutPrefix(ref, "Env."); found {
		spec.Env[name] = v
		return
	}
	rest, found := strings.CutPrefix(ref, "Steps[")
	if !found {
		return
	}
	index, field, found := strings.Cut(rest, "].")
	i, err := strconv.Atoi(index)
	if !found || err != nil || i < 0 || i >= len(spec.GetSteps()) {
		return
	}
	step := spec.GetSteps()[i]
	if name, ok := strings.CutPrefix(field, "Env."); ok {
		if step.Env == nil {
			step.Env = map[string]string{}
		}
		step.Env[name] = v
		return
	}
	if name, ok := strings.CutPrefix(field, "With."); ok && step.GetPlugin() != nil {
		if step.Plugin.Inputs == nil {
			step.Plugin.Inputs = map[string]string{}
		}
		step.Plugin.Inputs[name] = v
	}
}
