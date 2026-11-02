package pipelines

import (
	"slices"
	"strings"

	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	providerGeneric = "generic"
	inputBoolean    = "boolean"
	inputChoice     = "choice"
)

func refName(full, prefix string) string {
	name, _ := strings.CutPrefix(full, prefix)
	return name
}

func matchesAny(patterns []string, name string) bool {
	if len(patterns) == 0 {
		return true
	}
	return slices.ContainsFunc(patterns, func(p string) bool { return pipelinespec.MatchPattern(p, name) })
}

func enabled(on *pipelinespec.Triggers, run sqlc.PipelineRun) bool {
	if on == nil {
		return false
	}
	switch run.Trigger {
	case triggers.EventPush:
		return on.Push != nil && strings.HasPrefix(run.Ref, gitref.PrefixBranch) &&
			matchesAny(on.Push.Branches, refName(run.Ref, gitref.PrefixBranch))
	case triggers.EventTag:
		return on.Tag != nil && strings.HasPrefix(run.Ref, gitref.PrefixTag) &&
			matchesAny(on.Tag.Patterns, refName(run.Ref, gitref.PrefixTag))
	case triggers.EventPullRequest:
		return on.PullRequest != nil && matchesAny(on.PullRequest.Branches, refName(run.BaseRef.String, gitref.PrefixBranch))
	case triggers.EventManual:
		return on.Manual != nil
	case triggers.EventAPI:
		return on.API != nil
	default:
		return false
	}
}

func decodeInputs(raw []byte) (map[string]any, error) {
	out := map[string]any{}
	return out, decode(raw, &out, "inputs")
}

func inputIssue(name string) models.Issue {
	return models.Issue{
		Code: models.ReasonInputsInvalid, Severity: string(pipelinespec.SeverityError), Path: "on.manual.inputs." + name,
		Message: "the run gives this input a missing or wrong value",
	}
}

func checkInputs(spec *pipelinespec.Spec, given map[string]any) (map[string]any, []models.Issue) {
	declared := map[string]*pipelinespec.Input{}
	if spec.On != nil && spec.On.Manual != nil {
		declared = spec.On.Manual.Inputs
	}
	out := map[string]any{}
	var issues []models.Issue
	for name := range given {
		if _, ok := declared[name]; !ok {
			issues = append(issues, inputIssue(name))
		}
	}
	for name, in := range declared {
		v, ok := given[name]
		if !ok && in.Default != nil {
			v, ok = in.Default.Value(), true
			if in.Type != inputBoolean {
				v = in.Default.Text
			}
		}
		if !ok {
			if in.Required {
				issues = append(issues, inputIssue(name))
			}
			continue
		}
		if !validInput(in, v) {
			issues = append(issues, inputIssue(name))
			continue
		}
		out[name] = v
	}
	slices.SortFunc(issues, func(a, b models.Issue) int { return strings.Compare(a.Path, b.Path) })
	return out, issues
}

func validInput(in *pipelinespec.Input, v any) bool {
	switch in.Type {
	case inputBoolean:
		_, ok := v.(bool)
		return ok
	case inputChoice:
		s, ok := v.(string)
		return ok && slices.Contains(in.Options, s)
	default:
		s, ok := v.(string)
		return ok && len(s) <= pipelinespec.MaxContextVarBytes
	}
}

func gitContext(run sqlc.PipelineRun) pipelinespec.Git {
	g := pipelinespec.Git{
		SHA: run.Sha.String, Ref: run.Ref, Author: run.Actor.String, Event: run.Trigger,
		PR: pipelinespec.PullRequest{Number: int64(run.PrNumber.Int32), IsFork: run.IsFork},
	}
	switch {
	case strings.HasPrefix(run.Ref, gitref.PrefixBranch):
		g.Branch = refName(run.Ref, gitref.PrefixBranch)
	case strings.HasPrefix(run.Ref, gitref.PrefixTag):
		g.Tag, g.IsTag = refName(run.Ref, gitref.PrefixTag), true
	}
	if run.Trigger == triggers.EventPullRequest {
		g.PR.BaseRef = refName(run.BaseRef.String, gitref.PrefixBranch)
		g.PR.HeadRef = run.Ref
	}
	return g
}

func evalContext(run sqlc.PipelineRun, project string, needs map[string]any) (pipelinespec.Context, error) {
	inputs, err := decodeInputs(run.Inputs)
	if err != nil {
		return pipelinespec.Context{}, err
	}
	return pipelinespec.Context{
		Git: gitContext(run), Project: pipelinespec.Project{ID: run.ProjectId.String(), Name: project},
		Vars: map[string]string{}, Inputs: inputs, Needs: needs, Env: map[string]string{},
	}, nil
}

func outputsOf(raw []byte) (map[string]string, error) {
	out := map[string]string{}
	return out, decode(raw, &out, "outputs")
}

func needsContext(rows []sqlc.RunJob) (map[string]any, error) {
	single := map[string]map[string]string{}
	matrix := map[string]map[string]map[string]string{}
	for _, rj := range rows {
		if rj.Kind != models.KindJob || rj.State != models.JobSucceeded {
			continue
		}
		outputs, err := outputsOf(rj.Outputs)
		if err != nil {
			return nil, err
		}
		if string(rj.Matrix) == "[]" || len(rj.Matrix) == 0 {
			single[rj.JobId] = outputs
			continue
		}
		if matrix[rj.JobId] == nil {
			matrix[rj.JobId] = map[string]map[string]string{}
		}
		matrix[rj.JobId][rj.Key] = outputs
	}
	out := make(map[string]any, len(single)+len(matrix))
	for id, v := range single {
		out[id] = v
	}
	for id, v := range matrix {
		out[id] = v
	}
	return out, nil
}
