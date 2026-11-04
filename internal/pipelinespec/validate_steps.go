package pipelinespec

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"

	"github.com/Khaymah/dhow/internal/jobspec"
)

func (a *analysis) steps(ja *jobAnalysis) {
	steps, path := ja.job.Steps, ja.path+".steps"
	switch {
	case len(steps) == 0:
		a.issues.add(CodeStepsRequired, path)
		return
	case len(steps) > jobspec.MaxSteps:
		a.issues.add(CodeTooManySteps, path)
		return
	}
	ids := map[string]bool{}
	for i, s := range steps {
		p := indexPath(path, i)
		if s == nil {
			s = &Step{}
		}
		sa := stepAnalysis{step: s}
		if s.ID != "" {
			switch {
			case !jobIDPattern.MatchString(s.ID):
				a.issues.add(CodeStepIDInvalid, p+".id")
			case ids[s.ID]:
				a.issues.add(CodeStepIDDuplicate, p+".id")
			}
			ids[s.ID] = true
		}
		if s.Name != "" && !validText(s.Name) {
			a.issues.add(CodeNameInvalid, p+".name")
		}
		if s.If != "" {
			sa.ifExpr = ja.exprs.condition(scopeStepIf, s.If, p+".if")
		}
		a.stepBody(ja, &sa, p)
		sa.env = a.envMap(ja, s.Env, scopeStepValue, p+".env")
		sa.files = a.filesMap(ja, s.Files, scopeStepValue, p+".files")
		if s.Uses != "" && len(s.Files) > 0 {
			a.issues.add(CodeSecretFilesOnPlugin, p+".files")
		}
		if s.Timeout != "" {
			t, ok := jobTimeout(s.Timeout)
			if !ok || (ja.timeout > 0 && t > ja.timeout) {
				a.issues.add(CodeTimeoutInvalid, p+".timeout")
			}
			sa.timeout = t
		}
		ja.steps = append(ja.steps, sa)
	}
}

func (a *analysis) stepBody(ja *jobAnalysis, sa *stepAnalysis, p string) {
	s := sa.step
	switch {
	case (s.Run == "") == (s.Uses == ""):
		a.issues.add(CodeStepRunOrUses, p)
	case s.Run != "":
		a.run(s.Run, p+".run")
	default:
		a.uses(s.Uses, p+".uses")
	}
	if len(s.With) == 0 {
		return
	}
	if s.Uses == "" {
		a.issues.add(CodeWithWithoutUses, p+".with")
		return
	}
	if len(s.With) > MaxPluginInputs {
		a.issues.add(CodeTooManyInputs, p+".with")
		return
	}
	sa.with = map[string]value{}
	for _, name := range sortedNames(s.With) {
		wp := joinPath(p+".with", name)
		if !pluginInputPattern.MatchString(name) {
			a.issues.add(CodeInputInvalid, wp)
			continue
		}
		if v, ok := a.scalarValue(ja, s.With[name], scopeStepValue, wp, MaxPluginInputBytes); ok {
			sa.with[name] = v
		}
	}
}

func (a *analysis) run(script, path string) {
	switch {
	case strings.Contains(script, exprOpen):
		a.issues.add(CodeRunExpression, path)
	case strings.TrimSpace(script) == "", len(script) > jobspec.MaxRunBytes, strings.ContainsRune(script, 0):
		a.issues.add(CodeRunInvalid, path)
	}
}

func (a *analysis) uses(ref, path string) {
	if strings.Contains(ref, exprOpen) {
		a.issues.add(CodeExpressionNotAllowed, path)
		return
	}
	switch jobspec.ImageReason(jobspec.QualifyImage(ref)) {
	case "":
	case jobspec.ReasonNotPinned:
		a.issues.add(CodeUsesNotPinned, path)
	default:
		a.issues.add(CodeUsesInvalid, path)
	}
}

func validPath(p string) bool {
	if p == "" || len(p) > MaxPathLength || !utf8.ValidString(p) || strings.IndexFunc(p, unicode.IsControl) >= 0 {
		return false
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.ContainsAny(p, "\\") || strings.Contains(p, exprOpen) {
		return false
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func (a *analysis) paths(list []string, path string) []string {
	switch {
	case len(list) == 0:
		a.issues.add(CodePathsRequired, path)
		return nil
	case len(list) > MaxPaths:
		a.issues.add(CodeTooManyPaths, path)
		return nil
	}
	for i, p := range list {
		if !validPath(p) {
			a.issues.add(CodePathInvalid, indexPath(path, i))
		}
	}
	return list
}

func (a *analysis) artifacts(ja *jobAnalysis) {
	list, path := ja.job.Artifacts, ja.path+".artifacts"
	if len(list) > MaxArtifacts {
		a.issues.add(CodeTooManyArtifacts, path)
		return
	}
	for i, art := range list {
		p := indexPath(path, i)
		if art == nil {
			art = &Artifact{}
		}
		aa := artifactAnalysis{paths: a.paths(art.Paths, p+".paths")}
		if art.Retention != "" {
			d, err := ParseDuration(art.Retention)
			if err != nil || d < MinRetention || d > MaxRetention {
				a.issues.add(CodeRetentionInvalid, p+".retention")
			}
			aa.retention = int64(d / time.Second)
		}
		v, ok := ja.exprs.value(scopeArtifactName, art.Name, p+".name")
		if !ok {
			ja.artifacts = append(ja.artifacts, aa)
			continue
		}
		aa.name = v
		aa.names = a.artifactInstances(ja, v, p+".name")
		ja.artifacts = append(ja.artifacts, aa)
	}
}

func (a *analysis) artifactInstances(ja *jobAnalysis, v value, path string) []string {
	if !v.isExpression() {
		if !artifactPattern.MatchString(v.literal) {
			a.issues.add(CodeArtifactNameInvalid, path)
			return nil
		}
		names := make([]string, len(ja.combos))
		for i := range names {
			names[i] = v.literal
		}
		return names
	}
	var parts []namePart
	if v.ast == nil || !nameTemplate(v.ast.NativeRep().Expr(), &parts) {
		a.issues.add(CodeArtifactNameInvalid, path)
		return nil
	}
	names := make([]string, 0, len(ja.combos))
	for _, c := range ja.combos {
		var b strings.Builder
		for _, p := range parts {
			if p.key == "" {
				b.WriteString(p.literal)
				continue
			}
			s, _ := c.get(p.key)
			b.WriteString(s.Text)
		}
		name := b.String()
		if !artifactPattern.MatchString(name) {
			a.issues.add(CodeArtifactNameInvalid, path)
			return nil
		}
		names = append(names, name)
	}
	return names
}

type namePart struct {
	literal string
	key     string
}

func nameTemplate(e celast.Expr, parts *[]namePart) bool {
	if len(*parts) > MaxArtifactName {
		return false
	}
	switch e.Kind() {
	case celast.LiteralKind:
		s, ok := e.AsLiteral().Value().(string)
		*parts = append(*parts, namePart{literal: s})
		return ok
	case celast.IdentKind:
		key, ok := strings.CutPrefix(e.AsIdent(), "matrix.")
		*parts = append(*parts, namePart{key: key})
		return ok && key != ""
	case celast.SelectKind:
		sel := e.AsSelect()
		op := sel.Operand()
		ok := !sel.IsTestOnly() && op.Kind() == celast.IdentKind && op.AsIdent() == "matrix"
		*parts = append(*parts, namePart{key: sel.FieldName()})
		return ok
	case celast.CallKind:
		call := e.AsCall()
		if call.FunctionName() != operators.Add || len(call.Args()) != 2 || call.IsMemberFunction() {
			return false
		}
		return nameTemplate(call.Args()[0], parts) && nameTemplate(call.Args()[1], parts)
	default:
		return false
	}
}

func (a *analysis) artifactNames() {
	seen := map[string]bool{}
	for _, id := range sortedNames(a.jobs) {
		ja := a.jobs[id]
		for i, art := range ja.artifacts {
			for _, n := range art.names {
				if seen[n] {
					a.issues.add(CodeArtifactDuplicate, indexPath(ja.path+".artifacts", i)+".name")
					break
				}
				seen[n] = true
			}
		}
	}
}

func (a *analysis) cache(ja *jobAnalysis) {
	c, path := ja.job.Cache, ja.path+".cache"
	if c == nil {
		return
	}
	ca := &cacheAnalysis{paths: a.paths(c.Paths, path+".paths")}
	switch {
	case strings.TrimSpace(c.Key) == "":
		a.issues.add(CodeCacheKeyRequired, path+".key")
	case len(c.Key) > MaxPluginInputBytes:
		a.issues.add(CodeValueTooLong, path+".key")
	default:
		ca.key, _ = ja.exprs.value(scopeCacheKey, c.Key, path+".key")
	}
	ja.cache = ca
}
