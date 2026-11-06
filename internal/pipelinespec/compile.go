package pipelinespec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/proto"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type Value struct {
	Literal    string
	Expression string
}

type Plan struct {
	Name        string
	Concurrency *ConcurrencyPlan
	Jobs        []*RunJob
	Warnings    []Issue
	inputs      map[string]*cel.Type
}

type ConcurrencyPlan struct {
	Group            Value
	CancelInProgress bool
}

type RunJob struct {
	Key         string
	JobID       string
	Name        string
	Matrix      []MatrixValue
	Needs       []string
	Ingested    []string
	If          string
	Environment string
	MaxParallel int
	Spec        *runnerv1.JobSpec
	Expressions map[string]string
	Steps       []StepPlan
	Artifacts   []ArtifactPlan
	Cache       *CachePlan
	Digest      string
	decls       *declarations
	combo       combination
}

type StepPlan struct {
	ID      string
	If      string
	Secrets map[string]string
	Files   map[string]string
	Plugin  *PluginPlan
}

type PluginPlan struct {
	Image   string
	Inputs  map[string]string
	Secrets map[string]string
}

type ArtifactPlan struct {
	Name             string
	Paths            []string
	RetentionSeconds int64
}

type CachePlan struct {
	Key   Value
	Paths []string
}

func Compile(spec *Spec) (*Plan, error) {
	a := analyze(spec)
	if HasErrors(a.issues.items) {
		return nil, &Error{Issues: Errors(a.issues.sorted())}
	}
	plan := &Plan{Name: spec.Name, inputs: a.inputs}
	if c := spec.Concurrency; c != nil {
		plan.Concurrency = &ConcurrencyPlan{Group: exported(a.group), CancelInProgress: c.CancelInProgress}
	}
	keys := map[string][]string{}
	for _, id := range a.order {
		ja := a.jobs[id]
		for i, combo := range ja.combos {
			rj, err := a.runJob(ja, i, combo, keys)
			if err != nil {
				return nil, err
			}
			keys[id] = append(keys[id], rj.Key)
			plan.Jobs = append(plan.Jobs, rj)
		}
	}
	if HasErrors(a.issues.items) {
		return nil, &Error{Issues: Errors(a.issues.sorted())}
	}
	plan.Warnings = a.issues.sorted()
	return plan, nil
}

func exported(v value) Value {
	return Value{Literal: v.literal, Expression: v.expression}
}

func (a *analysis) runJob(ja *jobAnalysis, index int, combo combination, keys map[string][]string) (*RunJob, error) {
	key := runJobKey(ja.id, combo)
	rj := &RunJob{
		Key: key, JobID: ja.id, Name: ja.job.Name, Matrix: combo, If: ja.ifExpr,
		Environment: ja.job.Environment, MaxParallel: ja.maxParallel,
		Expressions: map[string]string{}, decls: ja.decls, combo: combo,
	}
	if rj.Name == "" {
		rj.Name = key
	} else if len(combo) > 0 {
		rj.Name += " " + strings.TrimPrefix(key, ja.id+" ")
	}
	for _, n := range ja.needs {
		rj.Needs = append(rj.Needs, keys[n]...)
	}
	rj.Ingested = append([]string(nil), ja.job.Ingested...)
	spec := a.jobSpec(ja, rj)
	for _, art := range ja.artifacts {
		rj.Artifacts = append(rj.Artifacts, ArtifactPlan{
			Name: art.names[index], Paths: art.paths, RetentionSeconds: art.retention,
		})
	}
	if ja.cache != nil {
		rj.Cache = &CachePlan{Key: exported(ja.cache.key), Paths: ja.cache.paths}
	}
	spec = jobspec.Normalize(spec)
	a.checkJobSpec(ja, spec)
	rj.Spec = spec
	digest, err := runJobDigest(rj)
	if err != nil {
		return nil, err
	}
	rj.Digest = digest
	return rj, nil
}

func (a *analysis) jobSpec(ja *jobAnalysis, rj *RunJob) *runnerv1.JobSpec {
	spec := &runnerv1.JobSpec{
		Image:          ja.job.Image,
		Env:            map[string]string{},
		Resources:      cloneResources(ja.resources),
		TimeoutSeconds: ja.timeout,
		Network:        &runnerv1.Network{},
		Isolation:      runnerv1.Isolation_ISOLATION_CONTAINER,
		Labels:         append([]string(nil), ja.labels...),
	}
	if ja.job.Network != nil {
		spec.Network.Egress = append([]string(nil), ja.job.Network.Egress...)
	}
	for _, name := range sortedNames(ja.env) {
		place(spec, rj, ja.env[name], name, "Env."+name, spec.Env, nil)
	}
	for i, sa := range ja.steps {
		s := sa.step
		step := &runnerv1.Step{Name: stepName(s), Run: s.Run, Env: map[string]string{}, TimeoutSeconds: sa.timeout}
		prefix := fmt.Sprintf("Steps[%d].", i)
		for _, name := range sortedNames(sa.env) {
			place(spec, rj, sa.env[name], name, prefix+"Env."+name, step.Env, []int32{int32(i)})
		}
		plan := StepPlan{ID: s.ID, If: sa.ifExpr, Secrets: map[string]string{}, Files: stepFiles(ja.files, sa)}
		for name, v := range sa.env {
			if v.secret != "" {
				plan.Secrets[name] = v.secret
			}
		}
		if s.Uses != "" {
			plan.Plugin = pluginPlan(rj, s, sa, prefix)
			step.Plugin = pluginStep(spec, plan.Plugin, int32(i))
		}
		spec.Steps = append(spec.Steps, step)
		rj.Steps = append(rj.Steps, plan)
	}
	placeFiles(spec, rj.Steps)
	return spec
}

func stepFiles(job map[string]string, sa stepAnalysis) map[string]string {
	out := map[string]string{}
	if sa.step.Uses == "" {
		for name, secret := range job {
			out[name] = secret
		}
	}
	for name, secret := range sa.files {
		out[name] = secret
	}
	return out
}

func placeFiles(spec *runnerv1.JobSpec, steps []StepPlan) {
	type target struct{ name, secret string }
	byTarget := map[target][]int32{}
	for i, sp := range steps {
		for name, secret := range sp.Files {
			t := target{name, secret}
			byTarget[t] = append(byTarget[t], int32(i))
		}
	}
	keys := make([]target, 0, len(byTarget))
	for t := range byTarget {
		keys = append(keys, t)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		return keys[i].secret < keys[j].secret
	})
	for _, t := range keys {
		spec.Secrets = append(spec.Secrets, &runnerv1.SecretRef{
			Name: t.secret, Target: &runnerv1.SecretRef_AsFile{AsFile: t.name}, StepIndexes: byTarget[t],
		})
	}
}

func place(spec *runnerv1.JobSpec, rj *RunJob, v value, name, ref string, env map[string]string, steps []int32) {
	switch {
	case v.secret != "":
		spec.Secrets = append(spec.Secrets, &runnerv1.SecretRef{
			Name: v.secret, Target: &runnerv1.SecretRef_AsEnv{AsEnv: name}, StepIndexes: steps,
		})
	case v.isExpression():
		rj.Expressions[ref] = v.expression
	default:
		env[name] = v.literal
	}
}

func pluginPlan(rj *RunJob, s *Step, sa stepAnalysis, prefix string) *PluginPlan {
	p := &PluginPlan{Image: jobspec.QualifyImage(s.Uses), Inputs: map[string]string{}, Secrets: map[string]string{}}
	for _, name := range sortedNames(sa.with) {
		v := sa.with[name]
		switch {
		case v.secret != "":
			p.Secrets[name] = v.secret
		case v.isExpression():
			rj.Expressions[prefix+"With."+name] = v.expression
		default:
			p.Inputs[name] = v.literal
		}
	}
	return p
}

func pluginStep(spec *runnerv1.JobSpec, p *PluginPlan, index int32) *runnerv1.Plugin {
	out := &runnerv1.Plugin{Image: p.Image, Inputs: map[string]string{}}
	for name, v := range p.Inputs {
		out.Inputs[name] = v
	}
	for _, name := range sortedNames(p.Secrets) {
		spec.Secrets = append(spec.Secrets, &runnerv1.SecretRef{
			Name: p.Secrets[name], Target: &runnerv1.SecretRef_AsEnv{AsEnv: jobspec.PluginInputEnvPrefix + strings.ToUpper(name)},
			StepIndexes: []int32{index},
		})
	}
	return out
}

func stepName(s *Step) string {
	if s.Name != "" {
		return s.Name
	}
	source := s.Run
	if s.Uses != "" {
		ref := jobspec.QualifyImage(s.Uses)
		ref, _, _ = strings.Cut(ref, "@")
		_, source, _ = strings.Cut(ref, "/")
	}
	for line := range strings.SplitSeq(source, "\n") {
		line = strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, line))
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > jobspec.MaxStepNameLength {
			line = string([]rune(line)[:jobspec.MaxStepNameLength])
		}
		return line
	}
	return "step"
}

func (a *analysis) checkJobSpec(ja *jobAnalysis, spec *runnerv1.JobSpec) {
	err := jobspec.Validate(spec)
	if err == nil {
		return
	}
	var verr *jobspec.Error
	if !errors.As(err, &verr) {
		a.issues.add(CodeSpecInvalid, ja.path)
		return
	}
	for _, v := range verr.Violations {
		if v.Field == "Spec" {
			a.issues.add(CodeSpecTooLarge, ja.path)
			continue
		}
		a.issues.add(CodeSpecInvalid, ja.path)
	}
}

type digestInput struct {
	Key, JobID, Name, If, Environment string
	Matrix                            []MatrixValue
	Needs                             []string
	Ingested                          []string `json:",omitempty"`
	MaxParallel                       int
	Spec                              []byte
	Expressions                       map[string]string
	Steps                             []StepPlan
	Artifacts                         []ArtifactPlan
	Cache                             *CachePlan
}

func runJobDigest(rj *RunJob) (string, error) {
	spec, err := proto.MarshalOptions{Deterministic: true}.Marshal(rj.Spec)
	if err != nil {
		return "", fmt.Errorf("encode job spec: %w", err)
	}
	raw, err := json.Marshal(digestInput{
		Key: rj.Key, JobID: rj.JobID, Name: rj.Name, If: rj.If, Environment: rj.Environment,
		Matrix: rj.Matrix, Needs: rj.Needs, Ingested: rj.Ingested, MaxParallel: rj.MaxParallel, Spec: spec,
		Expressions: rj.Expressions, Steps: rj.Steps, Artifacts: rj.Artifacts, Cache: rj.Cache,
	})
	if err != nil {
		return "", fmt.Errorf("encode run job: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func cloneResources(r *runnerv1.Resources) *runnerv1.Resources {
	return &runnerv1.Resources{
		CpuMillis: r.GetCpuMillis(), MemoryBytes: r.GetMemoryBytes(), Pids: r.GetPids(), DiskBytes: r.GetDiskBytes(),
	}
}
