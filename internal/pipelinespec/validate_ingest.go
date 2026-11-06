package pipelinespec

func (a *analysis) artifactTrigger() {
	t := a.spec.On.Artifact
	a.ingestNames = map[string]bool{}
	if t == nil {
		return
	}
	if len(t.Names) == 0 || len(t.Names) > MaxIngested {
		a.issues.add(CodeArtifactTriggerInvalid, "on.artifact.names")
	}
	for i, n := range t.Names {
		p := indexPath("on.artifact.names", i)
		switch {
		case !artifactPattern.MatchString(n), a.ingestNames[n]:
			a.issues.add(CodeArtifactTriggerInvalid, p)
		default:
			a.ingestNames[n] = true
		}
	}
	a.patterns(t.Refs, "on.artifact.refs")
}

func (a *analysis) ingested(ja *jobAnalysis) {
	list, path := ja.job.Ingested, ja.path+".ingested"
	if len(list) == 0 {
		return
	}
	if len(list) > MaxIngested {
		a.issues.add(CodeIngestedInvalid, path)
		return
	}
	seen := map[string]bool{}
	for i, n := range list {
		p := indexPath(path, i)
		switch {
		case !artifactPattern.MatchString(n), seen[n]:
			a.issues.add(CodeIngestedInvalid, p)
		case !a.ingestNames[n]:
			a.issues.add(CodeIngestedNotDeclared, p)
		}
		seen[n] = true
	}
}

func (a *analysis) ingestedClashes() {
	produced := map[string]bool{}
	for _, id := range sortedNames(a.jobs) {
		for _, art := range a.jobs[id].artifacts {
			for _, n := range art.names {
				produced[n] = true
			}
		}
	}
	for _, id := range sortedNames(a.jobs) {
		ja := a.jobs[id]
		for i, n := range ja.job.Ingested {
			if produced[n] {
				a.issues.add(CodeIngestedInvalid, indexPath(ja.path+".ingested", i))
			}
		}
	}
}
