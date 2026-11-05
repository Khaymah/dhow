package pipelinespec

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var ErrSecretScope = errors.New("a step holds a secret that was not declared on it")

func (r *RunJob) Build(ctx context.Context, c Context) (*runnerv1.JobSpec, error) {
	spec, _, err := r.BuildWithOrigins(ctx, c)
	return spec, err
}

func (r *RunJob) BuildWithOrigins(ctx context.Context, c Context) (*runnerv1.JobSpec, []int, error) {
	spec, ok := proto.Clone(r.Spec).(*runnerv1.JobSpec)
	if !ok {
		return nil, nil, ErrExpression
	}
	if spec.Env == nil {
		spec.Env = map[string]string{}
	}
	derived := make([]jobspec.DerivedPath, 0, len(r.Expressions))
	for ref := range r.Expressions {
		v, err := r.EvalValue(ctx, ref, c)
		if err != nil {
			return nil, nil, err
		}
		if d, ok := placeValue(spec, ref, v); ok {
			derived = append(derived, d)
		}
	}
	c.Env = spec.GetEnv()
	kept := make([]*runnerv1.Step, 0, len(spec.GetSteps()))
	origin := make([]int, 0, len(spec.GetSteps()))
	moved := make(map[int32]int32, len(spec.GetSteps()))
	next := int32(0)
	for i := int32(0); int(i) < len(spec.GetSteps()); i++ {
		keep, err := r.EvalStepIf(ctx, int(i), c)
		if err != nil {
			return nil, nil, err
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
	spec.DerivedValues = remapDerived(derived, moved)
	if err := r.checkStepSecrets(spec, origin); err != nil {
		return nil, nil, err
	}
	return spec, origin, nil
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

func remapDerived(paths []jobspec.DerivedPath, moved map[int32]int32) []string {
	out := make([]string, 0, len(paths))
	for _, d := range paths {
		if d.Step > math.MaxInt32 {
			continue
		}
		if d.Step >= 0 {
			n, ok := moved[int32(d.Step)]
			if !ok {
				continue
			}
			d.Step = int(n)
		}
		out = append(out, d.String())
	}
	slices.Sort(out)
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
	if file, ok := ref.GetTarget().(*runnerv1.SecretRef_AsFile); ok {
		return ref.GetName() != "" && p.Files[file.AsFile] == ref.GetName()
	}
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

func placeValue(spec *runnerv1.JobSpec, ref, v string) (jobspec.DerivedPath, bool) {
	d, ok := jobspec.ParseDerived(ref)
	if !ok {
		return d, false
	}
	if d.Step < 0 {
		spec.Env[d.Name] = v
		return d, true
	}
	if d.Step >= len(spec.GetSteps()) {
		return d, false
	}
	step := spec.GetSteps()[d.Step]
	if d.Field == "Env" {
		if step.Env == nil {
			step.Env = map[string]string{}
		}
		step.Env[d.Name] = v
		return d, true
	}
	if step.GetPlugin() == nil {
		return d, false
	}
	if step.Plugin.Inputs == nil {
		step.Plugin.Inputs = map[string]string{}
	}
	step.Plugin.Inputs[d.Name] = v
	return d, true
}
