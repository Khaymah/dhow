package pipelinespec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	celenv "github.com/google/cel-go/common/env"
	"github.com/google/cel-go/common/overloads"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/interpreter"
)

type exprScope int

const (
	scopeGroup exprScope = iota + 1
	scopeJobIf
	scopeJobValue
	scopeStepIf
	scopeStepValue
	scopeCacheKey
	scopeArtifactName
)

const (
	exprOpen      = "${{"
	exprClose     = "}}"
	secretsIdent  = "secrets"
	hashFilesFunc = "hash_files"
)

var (
	ErrExpression        = errors.New("invalid expression")
	ErrExpressionResult  = errors.New("expression result has the wrong type")
	ErrExpressionCost    = errors.New("expression exceeded its cost limit")
	ErrExpressionTimeout = errors.New("expression exceeded its time limit")
	ErrExpressionEval    = errors.New("expression failed to evaluate")
)

type celVar struct {
	name string
	typ  *cel.Type
}

func contextVars() []celVar {
	return []celVar{
		{"git.sha", cel.StringType},
		{"git.ref", cel.StringType},
		{"git.branch", cel.StringType},
		{"git.tag", cel.StringType},
		{"git.is_tag", cel.BoolType},
		{"git.message", cel.StringType},
		{"git.author", cel.StringType},
		{"git.event", cel.StringType},
		{"git.pr.number", cel.IntType},
		{"git.pr.base_ref", cel.StringType},
		{"git.pr.head_ref", cel.StringType},
		{"git.pr.is_fork", cel.BoolType},
		{"project.id", cel.StringType},
		{"project.name", cel.StringType},
		{"vars", cel.MapType(cel.StringType, cel.StringType)},
	}
}

type declarations struct {
	inputs map[string]*cel.Type
	matrix map[string]*cel.Type
	needs  map[string]bool
}

func (s exprScope) result() *cel.Type {
	if s == scopeJobIf || s == scopeStepIf {
		return cel.BoolType
	}
	return cel.StringType
}

func (s exprScope) secretsCode() string {
	switch s {
	case scopeJobIf, scopeStepIf:
		return CodeSecretsInIf
	case scopeJobValue, scopeStepValue:
		return CodeSecretNotWholeValue
	default:
		return CodeSecretsNotAllowed
	}
}

func excludedFunctions() []*celenv.Function {
	names := []string{
		overloads.Matches, overloads.TypeConvertTimestamp, overloads.TypeConvertDuration,
		overloads.TimeGetFullYear, overloads.TimeGetMonth, overloads.TimeGetDayOfYear, overloads.TimeGetDayOfMonth,
		overloads.TimeGetDate, overloads.TimeGetDayOfWeek, overloads.TimeGetHours, overloads.TimeGetMinutes,
		overloads.TimeGetSeconds, overloads.TimeGetMilliseconds,
	}
	out := make([]*celenv.Function, 0, len(names))
	for _, n := range names {
		out = append(out, celenv.NewFunction(n))
	}
	return out
}

func baseOptions() []cel.EnvOption {
	subset := celenv.NewLibrarySubset().AddExcludedFunctions(excludedFunctions()...)
	return []cel.EnvOption{
		cel.StdLib(cel.StdLibSubset(subset)),
		cel.ParserExpressionSizeLimit(MaxExpressionBytes),
		cel.ParserRecursionLimit(MaxExpressionDepth),
		cel.ExpressionNodeLimit(MaxExpressionNodes),
		cel.ParserErrorRecoveryLimit(4),
	}
}

func newEnv(scope exprScope, d *declarations, extra ...cel.EnvOption) (*cel.Env, error) {
	opts := baseOptions()
	if scope != scopeArtifactName {
		for _, v := range contextVars() {
			opts = append(opts, cel.Variable(v.name, v.typ))
		}
		for _, name := range sortedNames(d.inputs) {
			opts = append(opts, cel.Variable("inputs."+name, d.inputs[name]))
		}
	}
	if scope != scopeGroup {
		for _, name := range sortedNames(d.matrix) {
			opts = append(opts, cel.Variable("matrix."+name, d.matrix[name]))
		}
	}
	switch scope {
	case scopeJobIf, scopeJobValue, scopeStepIf, scopeStepValue:
		opts = append(opts, needsVars(d.needs)...)
	}
	switch scope {
	case scopeStepIf, scopeStepValue, scopeCacheKey:
		opts = append(opts, cel.Variable("env", cel.MapType(cel.StringType, cel.StringType)))
	}
	if scope == scopeCacheKey && len(extra) > 0 {
		opts = append(opts, extra...)
	} else if scope == scopeCacheKey {
		opts = append(opts, cel.Function(hashFilesFunc,
			cel.Overload("hash_files_string", []*cel.Type{cel.StringType}, cel.StringType),
			cel.Overload("hash_files_list", []*cel.Type{cel.ListType(cel.StringType)}, cel.StringType),
		))
	}
	env, err := cel.NewCustomEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("build expression environment: %w", err)
	}
	return env, nil
}

func needsVars(needs map[string]bool) []cel.EnvOption {
	flat := cel.MapType(cel.StringType, cel.StringType)
	byKey := cel.MapType(cel.StringType, flat)
	out := make([]cel.EnvOption, 0, len(needs))
	for _, id := range sortedNames(needs) {
		if !celIdent(id) {
			continue
		}
		t := flat
		if needs[id] {
			t = byKey
		}
		out = append(out, cel.Variable("needs."+id+".outputs", t))
	}
	return out
}

func celIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

type value struct {
	literal    string
	expression string
	secret     string
	ast        *cel.Ast
}

func (v value) isExpression() bool { return v.expression != "" }

func splitExpression(raw string) (string, bool, error) {
	if !strings.Contains(raw, exprOpen) {
		return raw, false, nil
	}
	trimmed := strings.TrimSpace(raw)
	inner, ok := strings.CutPrefix(trimmed, exprOpen)
	if ok {
		inner, ok = strings.CutSuffix(inner, exprClose)
	}
	if !ok || strings.Contains(inner, exprOpen) || strings.TrimSpace(inner) == "" {
		return "", false, ErrExpression
	}
	return strings.TrimSpace(inner), true, nil
}

func conditionSource(raw string) (string, error) {
	src, isExpr, err := splitExpression(raw)
	if err != nil {
		return "", err
	}
	if !isExpr {
		src = strings.TrimSpace(raw)
	}
	if src == "" {
		return "", ErrExpression
	}
	return src, nil
}

type exprBudget struct {
	count     int
	cost      uint64
	exhausted bool
}

type exprChecker struct {
	issues *issueList
	envs   map[exprScope]*cel.Env
	decls  *declarations
	budget *exprBudget
}

func newExprChecker(issues *issueList, d *declarations, b *exprBudget) *exprChecker {
	if b == nil {
		b = &exprBudget{}
	}
	return &exprChecker{issues: issues, envs: map[exprScope]*cel.Env{}, decls: d, budget: b}
}

func (c *exprChecker) spend(cost uint64, path string) bool {
	b := c.budget
	if b.exhausted {
		return false
	}
	b.count++
	b.cost += cost
	if b.count > MaxExpressions || b.cost > MaxTotalExpressionCost {
		b.exhausted = true
		c.issues.add(CodeExpressionBudget, path)
		return false
	}
	return true
}

func (c *exprChecker) env(scope exprScope) (*cel.Env, error) {
	if e, ok := c.envs[scope]; ok {
		return e, nil
	}
	e, err := newEnv(scope, c.decls)
	if err != nil {
		return nil, err
	}
	c.envs[scope] = e
	return e, nil
}

func (c *exprChecker) value(scope exprScope, raw, path string) (value, bool) {
	src, isExpr, err := splitExpression(raw)
	if err != nil {
		c.issues.add(CodeExpressionSyntax, path)
		return value{}, false
	}
	if !isExpr {
		return value{literal: raw}, true
	}
	return c.check(scope, src, path)
}

func (c *exprChecker) condition(scope exprScope, raw, path string) string {
	src, err := conditionSource(raw)
	if err != nil {
		c.issues.add(CodeExpressionSyntax, path)
		return ""
	}
	v, _ := c.check(scope, src, path)
	return v.expression
}

func (c *exprChecker) check(scope exprScope, src, path string) (value, bool) {
	if len(src) > MaxExpressionBytes {
		c.issues.add(CodeExpressionTooLong, path)
		return value{}, false
	}
	if !c.spend(0, path) {
		return value{}, false
	}
	env, err := c.env(scope)
	if err != nil {
		c.issues.add(CodeExpressionSyntax, path)
		return value{}, false
	}
	parsed, iss := env.Parse(src)
	if iss.Err() != nil {
		c.issues.add(CodeExpressionSyntax, path)
		return value{}, false
	}
	if name, whole := wholeSecret(parsed); whole && (scope == scopeJobValue || scope == scopeStepValue) {
		if !envName.MatchString(name) || len(name) > maxSecretName {
			c.issues.add(CodeValueInvalid, path)
			return value{}, false
		}
		return value{secret: name}, true
	}
	if usesIdent(parsed, secretsIdent) {
		c.issues.add(scope.secretsCode(), path)
		return value{}, false
	}
	checked, iss := env.Check(parsed)
	if iss.Err() != nil {
		c.issues.add(checkCode(iss.Err()), path)
		return value{}, false
	}
	if !acceptsResult(checked.OutputType(), scope.result()) {
		c.issues.add(CodeExpressionType, path)
		return value{}, false
	}
	est, err := env.EstimateCost(checked, sizeEstimator{})
	if err != nil || est.Max > MaxExpressionCost {
		c.issues.add(CodeExpressionTooExpensive, path)
		return value{}, false
	}
	if !c.spend(est.Max, path) {
		return value{}, false
	}
	return value{expression: src, ast: checked}, true
}

func checkCode(err error) string {
	if strings.Contains(err.Error(), "undeclared reference") {
		return CodeExpressionUnknownName
	}
	return CodeExpressionType
}

func acceptsResult(got, want *cel.Type) bool {
	return got.IsExactType(want) || got.IsExactType(cel.DynType)
}

func wholeSecret(a *cel.Ast) (string, bool) {
	e := a.NativeRep().Expr()
	if e.Kind() != celast.SelectKind {
		return "", false
	}
	sel := e.AsSelect()
	if sel.IsTestOnly() || sel.Operand().Kind() != celast.IdentKind || sel.Operand().AsIdent() != secretsIdent {
		return "", false
	}
	return sel.FieldName(), true
}

func usesIdent(a *cel.Ast, name string) bool {
	found := false
	celast.PreOrderVisit(a.NativeRep().Expr(), celast.NewExprVisitor(func(e celast.Expr) {
		if e.Kind() == celast.IdentKind && e.AsIdent() == name {
			found = true
		}
	}))
	return found
}

type program struct {
	prg   cel.Program
	scope exprScope
}

func prepare(scope exprScope, d *declarations, src string, extra ...cel.EnvOption) (*program, error) {
	env, err := newEnv(scope, d, extra...)
	if err != nil {
		return nil, err
	}
	checked, iss := env.Compile(src)
	if iss.Err() != nil {
		return nil, ErrExpression
	}
	if usesIdent(checked, secretsIdent) {
		return nil, ErrExpression
	}
	prg, err := env.Program(checked,
		cel.CostLimit(MaxExpressionCost),
		cel.InterruptCheckFrequency(InterruptCheckEvery),
	)
	if err != nil {
		return nil, fmt.Errorf("plan expression: %w", err)
	}
	return &program{prg: prg, scope: scope}, nil
}

func (p *program) run(ctx context.Context, act map[string]any, timeout time.Duration) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, _, err := p.prg.ContextEval(ctx, act)
	if err != nil {
		return nil, evalError(ctx, err)
	}
	if types.IsUnknownOrError(out) {
		return nil, ErrExpression
	}
	native := out.Value()
	switch p.scope.result() {
	case cel.BoolType:
		if _, ok := native.(bool); !ok {
			return nil, ErrExpressionResult
		}
	default:
		if _, ok := native.(string); !ok {
			return nil, ErrExpressionResult
		}
	}
	return native, nil
}

func evaluate(ctx context.Context, scope exprScope, d *declarations, src string, act map[string]any, extra ...cel.EnvOption) (any, error) {
	p, err := prepare(scope, d, src, extra...)
	if err != nil {
		return nil, err
	}
	return p.run(ctx, act, ExpressionTimeout)
}

func evalError(ctx context.Context, err error) error {
	var cancelled interpreter.EvalCancelledError
	switch {
	case errors.As(err, &cancelled) && cancelled.Cause == interpreter.CostLimitExceeded:
		return ErrExpressionCost
	case errors.As(err, &cancelled), ctx.Err() != nil:
		return ErrExpressionTimeout
	default:
		return ErrExpressionEval
	}
}
