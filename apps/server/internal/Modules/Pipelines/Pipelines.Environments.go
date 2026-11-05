package pipelines

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const digestMarker = "@sha256:"

func (s *Service) UseGate(g deployments.Gate) { s.gate = g }

type RefTarget struct {
	Kind        string
	Name        string
	PullRequest bool
}

func TargetOf(trigger, ref, baseRef string) RefTarget {
	if trigger == triggerPullRequest {
		name, _ := strings.CutPrefix(baseRef, gitref.PrefixBranch)
		return RefTarget{Kind: deployments.RefBranch, Name: name, PullRequest: true}
	}
	parsed, err := gitref.Parse(ref)
	if err != nil {
		return RefTarget{}
	}
	switch parsed.Kind {
	case gitref.KindBranch:
		return RefTarget{Kind: deployments.RefBranch, Name: parsed.Name}
	case gitref.KindTag:
		return RefTarget{Kind: deployments.RefTag, Name: parsed.Name}
	default:
		return RefTarget{}
	}
}

func (s *Service) environmentNames(run sqlc.PipelineRun) map[string]string {
	out := map[string]string{}
	if !run.DefinitionSource.Valid {
		return out
	}
	c, err := s.compileRun(run)
	if err != nil {
		return out
	}
	for key, plan := range c.byKey {
		if plan.Environment != "" {
			out[key] = plan.Environment
		}
	}
	return out
}

func (s *Service) lockEnvironment(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, name string) (deployments.Environment, bool, error) {
	t := TargetOf(run.Trigger, run.Ref, run.BaseRef.String)
	return s.gate.Lock(ctx, q, deployments.Target{
		OrganizationID: run.OrganizationId, ProjectID: run.ProjectId, Name: name,
		RefKind: t.Kind, RefName: t.Name, PullRequest: t.PullRequest,
	})
}

func (s *Service) environmentVars(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, env uuid.UUID) (map[string]string, error) {
	if s.vars == nil || run.IsFork {
		return map[string]string{}, nil
	}
	return s.vars.Variables(ctx, q, run.OrganizationId, run.ProjectId, &env)
}

type refRefusal struct {
	Ref         string `json:"Ref"`
	Environment string `json:"Environment"`
}

func refDetail(run sqlc.PipelineRun, env string) []byte {
	t := TargetOf(run.Trigger, run.Ref, run.BaseRef.String)
	ref := t.Name
	if ref == "" {
		ref = run.Ref
	}
	raw, err := json.Marshal(refRefusal{Ref: ref, Environment: env})
	if err != nil || len(raw) > failureDetailMax {
		return nil
	}
	return raw
}

func writtenImage(image string, pins map[string]string) string {
	found := image
	for _, written := range slices.Sorted(maps.Keys(pins)) {
		if pins[written] != image {
			continue
		}
		if !strings.Contains(written, digestMarker) {
			return written
		}
		found = written
	}
	return found
}

func qualifyPrefix(p string) string {
	host, _, found := strings.Cut(p, "/")
	switch {
	case found && (strings.ContainsAny(host, ".:") || host == "localhost"):
		return p
	case !found:
		return jobspec.DefaultRegistry + "/" + jobspec.DefaultNamespace + "/" + p
	default:
		return jobspec.DefaultRegistry + "/" + p
	}
}

func PluginAllowed(image string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	qualified := jobspec.QualifyImage(image)
	return slices.ContainsFunc(prefixes, func(raw string) bool {
		p := qualifyPrefix(raw)
		if strings.HasSuffix(p, "/") {
			return strings.HasPrefix(qualified, p)
		}
		return qualified == p || slices.ContainsFunc([]string{"/", ":", "@"}, func(sep string) bool {
			return strings.HasPrefix(qualified, p+sep)
		})
	})
}

func staticRefusal(env deployments.Environment, spec *runnerv1.JobSpec, pins map[string]string) string {
	if env.RequirePinnedImages {
		if img := spec.GetImage(); img != "" && !strings.Contains(writtenImage(img, pins), digestMarker) {
			return models.ReasonEnvironmentPinnedImages
		}
	}
	for _, st := range spec.GetSteps() {
		img := st.GetPlugin().GetImage()
		if img == "" {
			continue
		}
		if env.RequirePinnedImages && !strings.Contains(img, digestMarker) {
			return models.ReasonEnvironmentPinnedImages
		}
		if !PluginAllowed(img, env.AllowedPlugins) {
			return models.ReasonEnvironmentPlugin
		}
	}
	return ""
}
