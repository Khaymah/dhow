package pipelinespec

import (
	"context"
	"strings"

	"github.com/google/cel-go/cel"

	"github.com/Khaymah/dhow/internal/jobspec"
)

type Context struct {
	Git     Git
	Project Project
	Vars    map[string]string
	Inputs  map[string]any
	Needs   map[string]any
	Env     map[string]string

	SecretEnv []string
}

type Git struct {
	SHA     string
	Ref     string
	Branch  string
	Tag     string
	IsTag   bool
	Message string
	Author  string
	Event   string
	PR      PullRequest
}

type PullRequest struct {
	Number  int64
	BaseRef string
	HeadRef string
	IsFork  bool
}

type Project struct {
	ID   string
	Name string
}

func (p *Plan) EvalGroup(ctx context.Context, c Context) (string, error) {
	if p.Concurrency == nil {
		return "", nil
	}
	g := p.Concurrency.Group
	if g.Expression == "" {
		return g.Literal, nil
	}
	d := &declarations{inputs: p.inputs}
	act, err := activation(c, d, nil, nil)
	if err != nil {
		return "", err
	}
	out, err := evaluate(ctx, scopeGroup, d, g.Expression, act)
	if err != nil {
		return "", err
	}
	return boundedString(out, MaxContextTextBytes)
}

func (r *RunJob) EvalIf(ctx context.Context, c Context) (bool, error) {
	return r.evalBool(ctx, scopeJobIf, r.If, c)
}

func (r *RunJob) EvalStepIf(ctx context.Context, step int, c Context) (bool, error) {
	if step < 0 || step >= len(r.Steps) {
		return false, ErrExpression
	}
	return r.evalBool(ctx, scopeStepIf, r.Steps[step].If, c)
}

func (r *RunJob) evalBool(ctx context.Context, scope exprScope, src string, c Context) (bool, error) {
	if src == "" {
		return true, nil
	}
	out, err := r.evaluate(ctx, scope, src, c)
	if err != nil {
		return false, err
	}
	b, _ := out.(bool)
	return b, nil
}

func (r *RunJob) EvalValue(ctx context.Context, ref string, c Context) (string, error) {
	src, ok := r.Expressions[ref]
	if !ok {
		return "", ErrExpression
	}
	scope := scopeJobValue
	if strings.HasPrefix(ref, "Steps[") {
		scope = scopeStepValue
	}
	out, err := r.evaluate(ctx, scope, src, c)
	if err != nil {
		return "", err
	}
	return boundedString(out, jobspec.MaxEnvValueBytes)
}

func (r *RunJob) evaluate(ctx context.Context, scope exprScope, src string, c Context) (any, error) {
	return r.evaluateWith(ctx, scope, src, c, nil)
}

func (r *RunJob) evaluateWith(ctx context.Context, scope exprScope, src string, c Context, extra []cel.EnvOption) (any, error) {
	secret := make(map[string]bool, len(c.SecretEnv)+len(r.Spec.GetSecrets()))
	for _, name := range c.SecretEnv {
		secret[name] = true
	}
	for _, ref := range r.Spec.GetSecrets() {
		if name := ref.GetAsEnv(); name != "" {
			secret[name] = true
		}
	}
	act, err := activation(c, r.decls, r.combo, secret)
	if err != nil {
		return nil, err
	}
	return evaluate(ctx, scope, r.decls, src, act, extra...)
}

func boundedString(out any, limit int) (string, error) {
	s, _ := out.(string)
	if len(s) > limit {
		return "", ErrExpressionResult
	}
	return s, nil
}
