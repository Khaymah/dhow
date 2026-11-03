package pipelinespec

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/cel-go/cel"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	inputString   = "string"
	inputBoolean  = "boolean"
	inputChoice   = "choice"
	isoContainer  = "container"
	isoGVisor     = "gvisor"
	maxSecretName = jobspec.MaxSecretNameLength
	maxInputName  = 64
)

var (
	jobIDPattern       = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	envName            = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	environmentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	artifactPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	pluginInputPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type analysis struct {
	spec        *Spec
	issues      *issueList
	inputs      map[string]*cel.Type
	order       []string
	jobs        map[string]*jobAnalysis
	group       value
	runJobCount int
	budget      *exprBudget
}

func Check(src []byte) (*Spec, []Issue) {
	spec, issues := Parse(src)
	if spec == nil {
		return nil, issues
	}
	return spec, Validate(spec)
}

func Validate(spec *Spec) []Issue {
	return analyze(spec).issues.sorted()
}

func analyze(spec *Spec) *analysis {
	positions := spec.positions
	if positions == nil {
		positions = map[string]Position{}
	}
	a := &analysis{
		spec: spec, issues: &issueList{positions: positions}, jobs: map[string]*jobAnalysis{}, budget: &exprBudget{},
	}
	if spec.Version != SupportedVersion {
		a.issues.add(CodeVersionUnsupported, "version")
	}
	if spec.Name != "" && !validText(spec.Name) {
		a.issues.add(CodeNameInvalid, "name")
	}
	a.triggers()
	a.defaults()
	a.concurrency()
	a.allJobs()
	return a
}

func validText(v string) bool {
	return strings.TrimSpace(v) != "" && utf8.ValidString(v) && utf8.RuneCountInString(v) <= MaxNameLength &&
		strings.IndexFunc(v, unicode.IsControl) < 0 && !strings.Contains(v, exprOpen)
}

func (a *analysis) triggers() {
	on := a.spec.On
	if on == nil || (on.Push == nil && on.Tag == nil && on.PullRequest == nil && on.Manual == nil && on.API == nil) {
		a.issues.add(CodeTriggersRequired, "on")
		return
	}
	if on.Push != nil {
		a.patterns(on.Push.Branches, "on.push.branches")
		a.patterns(on.Push.Paths, "on.push.paths")
	}
	if on.PullRequest != nil {
		a.patterns(on.PullRequest.Branches, "on.pull_request.branches")
		a.patterns(on.PullRequest.Paths, "on.pull_request.paths")
	}
	if on.Tag != nil {
		a.patterns(on.Tag.Patterns, "on.tag.patterns")
	}
	a.inputs = map[string]*cel.Type{}
	if on.Manual != nil {
		a.manualInputs(on.Manual.Inputs, "on.manual.inputs")
	}
}

func (a *analysis) patterns(list []string, path string) {
	if len(list) > MaxPatterns {
		a.issues.add(CodeTooManyPatterns, path)
		return
	}
	for i, p := range list {
		if !ValidPattern(p) {
			a.issues.add(CodePatternInvalid, indexPath(path, i))
		}
	}
}

func (a *analysis) manualInputs(inputs map[string]*Input, path string) {
	if len(inputs) > MaxManualInputs {
		a.issues.add(CodeTooManyInputs, path)
		return
	}
	for _, name := range sortedNames(inputs) {
		p := joinPath(path, name)
		in := inputs[name]
		if !celIdent(name) || len(name) > maxInputName || !validInput(in) {
			a.issues.add(CodeInputInvalid, p)
			continue
		}
		if in.Type == inputBoolean {
			a.inputs[name] = cel.BoolType
		} else {
			a.inputs[name] = cel.StringType
		}
	}
}

func validInput(in *Input) bool {
	if in.Description != "" && (!utf8.ValidString(in.Description) || utf8.RuneCountInString(in.Description) > MaxDescription) {
		return false
	}
	switch in.Type {
	case "", inputString:
		return len(in.Options) == 0 && (in.Default == nil || len(in.Default.Text) <= MaxContextVarBytes)
	case inputBoolean:
		return len(in.Options) == 0 && (in.Default == nil || in.Default.Kind == KindBool)
	case inputChoice:
		return validChoice(in)
	default:
		return false
	}
}

func validChoice(in *Input) bool {
	if len(in.Options) == 0 || len(in.Options) > MaxChoiceOptions {
		return false
	}
	seen := map[string]bool{}
	for _, o := range in.Options {
		if !validText(o) || seen[o] {
			return false
		}
		seen[o] = true
	}
	return in.Default == nil || seen[in.Default.Text]
}

func (a *analysis) defaults() {
	d := a.spec.Defaults
	if d == nil {
		return
	}
	a.runner(d.Runner, "defaults.runner")
	if d.Timeout != "" {
		if _, ok := jobTimeout(d.Timeout); !ok {
			a.issues.add(CodeTimeoutInvalid, "defaults.timeout")
		}
	}
}

func (a *analysis) runner(r *Runner, path string) {
	if r == nil {
		return
	}
	if len(r.Labels) > jobspec.MaxLabels {
		a.issues.add(CodeTooManyLabels, path+".labels")
	} else {
		for i, l := range r.Labels {
			if !jobspec.ValidLabel(l) {
				a.issues.add(CodeLabelInvalid, indexPath(path+".labels", i))
			}
		}
	}
	switch r.Isolation {
	case "", isoContainer:
	case isoGVisor:
		a.issues.add(CodeIsolationNotSupported, path+".isolation")
	default:
		a.issues.add(CodeIsolationInvalid, path+".isolation")
	}
}

func jobTimeout(s string) (int32, bool) {
	d, err := ParseDuration(s)
	if err != nil || d < time.Duration(jobspec.MinTimeoutSeconds)*time.Second ||
		d > time.Duration(jobspec.MaxTimeoutSeconds)*time.Second {
		return 0, false
	}
	return int32(d / time.Second), true
}

func (a *analysis) concurrency() {
	c := a.spec.Concurrency
	if c == nil {
		return
	}
	if strings.TrimSpace(c.Group) == "" {
		a.issues.add(CodeGroupRequired, "concurrency.group")
		return
	}
	if len(c.Group) > MaxPluginInputBytes {
		a.issues.add(CodeValueTooLong, "concurrency.group")
		return
	}
	checker := newExprChecker(a.issues, &declarations{inputs: a.inputs}, a.budget)
	if v, ok := checker.value(scopeGroup, c.Group, "concurrency.group"); ok {
		a.group = v
	}
}

func (a *analysis) allJobs() {
	jobs := a.spec.Jobs
	switch {
	case len(jobs) == 0:
		a.issues.add(CodeJobsRequired, "jobs")
		return
	case len(jobs) > MaxJobs:
		a.issues.add(CodeTooManyJobs, "jobs")
		return
	}
	for id, j := range jobs {
		if j == nil {
			jobs[id] = &Job{}
		}
	}
	graph := make(map[string][]string, len(jobs))
	for _, id := range sortedNames(jobs) {
		p := joinPath("jobs", id)
		if !jobIDPattern.MatchString(id) {
			a.issues.add(CodeJobIDInvalid, p)
		}
		graph[id] = a.needs(jobs[id].Needs, p+".needs")
	}
	order, cyclic := topoOrder(graph)
	for _, id := range cyclic {
		a.issues.add(CodeDAGCycle, joinPath("jobs", id)+".needs")
	}
	a.order = order
	for _, id := range sortedNames(jobs) {
		ja := a.expand(id, jobs[id])
		a.jobs[id] = ja
		a.runJobCount += len(ja.combos)
	}
	if a.runJobCount > MaxRunJobs {
		a.issues.add(CodeTooManyRunJobs, "jobs")
		return
	}
	for _, id := range sortedNames(jobs) {
		a.job(a.jobs[id])
	}
	a.artifactNames()
}

func (a *analysis) needs(needs []string, path string) []string {
	if len(needs) > MaxNeeds {
		a.issues.add(CodeNeedDuplicate, path)
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(needs))
	for i, n := range needs {
		p := indexPath(path, i)
		_, exists := a.spec.Jobs[n]
		switch {
		case !exists:
			a.issues.add(CodeNeedUnknown, p)
		case seen[n]:
			a.issues.add(CodeNeedDuplicate, p)
		default:
			out = append(out, n)
		}
		seen[n] = true
	}
	return out
}

func ValidEnvironmentName(name string) bool { return environmentPattern.MatchString(name) }
