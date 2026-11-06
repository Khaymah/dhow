package pipelinespec

import (
	"strings"

	"github.com/google/cel-go/cel"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type jobAnalysis struct {
	id          string
	job         *Job
	path        string
	combos      []combination
	needs       []string
	decls       *declarations
	exprs       *exprChecker
	ifExpr      string
	env         map[string]value
	files       map[string]string
	timeout     int32
	labels      []string
	resources   *runnerv1.Resources
	steps       []stepAnalysis
	artifacts   []artifactAnalysis
	cache       *cacheAnalysis
	usesSecrets bool
	maxParallel int
}

type stepAnalysis struct {
	step    *Step
	ifExpr  string
	env     map[string]value
	with    map[string]value
	files   map[string]string
	timeout int32
}

type artifactAnalysis struct {
	name      value
	paths     []string
	retention int64
	names     []string
}

type cacheAnalysis struct {
	key   value
	paths []string
}

func (a *analysis) expand(id string, job *Job) *jobAnalysis {
	ja := &jobAnalysis{id: id, job: job, path: joinPath("jobs", id), env: map[string]value{}}
	var m *Matrix
	if job.Strategy != nil {
		m = job.Strategy.Matrix
		ja.maxParallel = job.Strategy.MaxParallel
		if ja.maxParallel < 0 || ja.maxParallel > MaxMatrixJobs {
			a.issues.add(CodeMaxParallelInvalid, ja.path+".strategy.max_parallel")
		}
	}
	combos, ok := expandMatrix(m, ja.path+".strategy.matrix", a.issues)
	matrix := matrixTypes(combos)
	if !ok && m != nil {
		for _, k := range sortedNames(m.Axes) {
			matrix[k] = cel.DynType
		}
	}
	ja.combos = combos
	needs := map[string]bool{}
	for _, n := range uniqueKnownJobs(job.Needs, a.spec.Jobs) {
		dep := a.spec.Jobs[n]
		needs[n] = dep.Strategy != nil && dep.Strategy.Matrix != nil
		ja.needs = append(ja.needs, n)
	}
	ja.decls = &declarations{inputs: a.inputs, matrix: matrix, needs: needs}
	ja.exprs = newExprChecker(a.issues, ja.decls, a.budget)
	return ja
}

func uniqueKnownJobs(needs []string, jobs map[string]*Job) []string {
	out := make([]string, 0, len(needs))
	seen := map[string]bool{}
	for _, n := range needs {
		if _, ok := jobs[n]; ok && !seen[n] {
			out = append(out, n)
			seen[n] = true
		}
	}
	return out
}

func (a *analysis) job(ja *jobAnalysis) {
	job, p := ja.job, ja.path
	if job.Name != "" && !validText(job.Name) {
		a.issues.add(CodeNameInvalid, p+".name")
	}
	if job.If != "" {
		ja.ifExpr = ja.exprs.condition(scopeJobIf, job.If, p+".if")
	}
	ja.env = a.envMap(ja, job.Env, scopeJobValue, p+".env")
	ja.files = a.filesMap(ja, job.Files, scopeJobValue, p+".files")
	if job.IDToken != nil {
		a.issues.add(CodeIDTokenUnsupported, p+".id_token")
	}
	a.runner(job.Runner, p+".runner")
	ja.labels = a.labels(job)
	ja.timeout = a.timeout(job.Timeout, p+".timeout")
	if job.Environment != "" && !environmentPattern.MatchString(job.Environment) {
		a.issues.add(CodeEnvironmentInvalid, p+".environment")
	}
	if job.Network != nil {
		a.egress(job.Network.Egress, p+".network.egress")
	}
	ja.resources = a.resources(job.Resources, p+".resources")
	a.ingested(ja)
	a.steps(ja)
	a.image(ja)
	a.artifacts(ja)
	a.cache(ja)
	a.countFiles(ja, p+".files")
}

func (a *analysis) labels(job *Job) []string {
	if job.Runner != nil && len(job.Runner.Labels) > 0 {
		return job.Runner.Labels
	}
	if d := a.spec.Defaults; d != nil && d.Runner != nil {
		return d.Runner.Labels
	}
	return nil
}

func (a *analysis) timeout(raw, path string) int32 {
	if raw == "" {
		if d := a.spec.Defaults; d != nil && d.Timeout != "" {
			t, _ := jobTimeout(d.Timeout)
			return t
		}
		return jobspec.DefaultTimeoutSeconds
	}
	t, ok := jobTimeout(raw)
	if !ok {
		a.issues.add(CodeTimeoutInvalid, path)
	}
	return t
}

func (a *analysis) image(ja *jobAnalysis) {
	img, p := ja.job.Image, ja.path+".image"
	hasRun := false
	for _, s := range ja.steps {
		hasRun = hasRun || s.step.Run != ""
	}
	if img == "" {
		if hasRun {
			a.issues.add(CodeImageRequired, p)
		}
		return
	}
	if strings.Contains(img, exprOpen) {
		a.issues.add(CodeExpressionNotAllowed, p)
		return
	}
	switch jobspec.ImageReason(jobspec.QualifyImage(img)) {
	case "":
	case jobspec.ReasonNotPinned:
		a.issues.warn(CodeImageNotPinned, p)
	default:
		a.issues.add(CodeImageInvalid, p)
	}
}

func (a *analysis) envMap(ja *jobAnalysis, env map[string]Scalar, scope exprScope, path string) map[string]value {
	out := map[string]value{}
	if len(env) > jobspec.MaxEnvEntries {
		a.issues.add(CodeTooManyEnv, path)
		return out
	}
	for _, name := range sortedNames(env) {
		p := joinPath(path, name)
		switch {
		case !envName.MatchString(name):
			a.issues.add(CodeEnvNameInvalid, p)
			continue
		case len(name) > jobspec.MaxEnvNameLength:
			a.issues.add(CodeEnvNameInvalid, p)
			continue
		case strings.HasPrefix(strings.ToUpper(name), jobspec.ReservedEnvPrefix):
			a.issues.add(CodeEnvNameReserved, p)
			continue
		}
		if v, ok := a.scalarValue(ja, env[name], scope, p, jobspec.MaxEnvValueBytes); ok {
			out[name] = v
		}
	}
	return out
}

func (a *analysis) filesMap(ja *jobAnalysis, files map[string]Scalar, scope exprScope, path string) map[string]string {
	out := map[string]string{}
	for _, name := range sortedNames(files) {
		p := joinPath(path, name)
		if !jobspec.ValidSecretFile(name) {
			a.issues.add(CodeSecretFileInvalid, p)
			continue
		}
		v, ok := a.scalarValue(ja, files[name], scope, p, jobspec.MaxEnvValueBytes)
		if !ok {
			continue
		}
		if v.secret == "" {
			a.issues.add(CodeSecretFileNotSecret, p)
			continue
		}
		out[name] = v.secret
	}
	return out
}

func (a *analysis) countFiles(ja *jobAnalysis, path string) {
	names := map[string]bool{}
	for n := range ja.files {
		names[n] = true
	}
	for _, sa := range ja.steps {
		for n := range sa.files {
			names[n] = true
		}
	}
	if len(names) > jobspec.MaxSecretFiles {
		a.issues.add(CodeTooManySecretFiles, path)
	}
}

func (a *analysis) scalarValue(ja *jobAnalysis, s Scalar, scope exprScope, path string, limit int) (value, bool) {
	switch {
	case len(s.Text) > limit:
		a.issues.add(CodeValueTooLong, path)
		return value{}, false
	case strings.ContainsRune(s.Text, 0):
		a.issues.add(CodeValueInvalid, path)
		return value{}, false
	}
	if s.Kind != KindString && s.Kind != 0 {
		return value{literal: s.Text}, true
	}
	v, ok := ja.exprs.value(scope, s.Text, path)
	if ok && v.secret != "" {
		ja.usesSecrets = true
	}
	return v, ok
}

func (a *analysis) egress(entries []string, path string) {
	if len(entries) > jobspec.MaxEgressEntries {
		a.issues.add(CodeTooManyEgress, path)
		return
	}
	for i, e := range entries {
		if _, err := jobspec.ParseEgress(e); err != nil {
			a.issues.add(CodeEgressInvalid, indexPath(path, i))
		}
	}
}

func (a *analysis) resources(r *Resources, path string) *runnerv1.Resources {
	out := &runnerv1.Resources{}
	if r == nil {
		return out
	}
	if r.CPU != nil {
		v, err := ParseCPU(*r.CPU)
		if err != nil || v < jobspec.MinCPUMillis || v > jobspec.MaxCPUMillis {
			a.issues.add(CodeResourceInvalid, path+".cpu")
		}
		out.CpuMillis = v
	}
	out.MemoryBytes = a.size(r.Memory, path+".memory", jobspec.MinMemoryBytes, jobspec.MaxMemoryBytes)
	out.DiskBytes = a.size(r.Disk, path+".disk", jobspec.MinDiskBytes, jobspec.MaxDiskBytes)
	if r.Pids != 0 && (r.Pids < jobspec.MinPids || r.Pids > jobspec.MaxPids) {
		a.issues.add(CodeResourceInvalid, path+".pids")
	}
	out.Pids = int64(r.Pids)
	return out
}

func (a *analysis) size(raw, path string, lo, hi int64) int64 {
	if raw == "" {
		return 0
	}
	v, err := ParseSize(raw)
	if err != nil || v < lo || v > hi {
		a.issues.add(CodeResourceInvalid, path)
	}
	return v
}
