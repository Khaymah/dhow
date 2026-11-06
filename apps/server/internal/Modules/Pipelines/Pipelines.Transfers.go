package pipelines

import (
	"context"
	"slices"

	artifactindex "github.com/Khaymah/dhow/apps/server/internal/Core/ArtifactIndex"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/pipelinespec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func feedsDeploy(plan *pipelinespec.RunJob, spec *runnerv1.JobSpec) bool {
	return plan.Environment != "" || slices.ContainsFunc(spec.GetSteps(), func(st *runnerv1.Step) bool { return st.GetMutating() })
}

func (s *Service) attachTransfers(
	ctx context.Context, q *sqlc.Queries, st *step, rj sqlc.RunJob, plan *pipelinespec.RunJob, spec *runnerv1.JobSpec,
	evalCtx pipelinespec.Context,
) (string, error) {
	if len(rj.Needs) == 0 && len(plan.Ingested) == 0 && len(plan.Artifacts) == 0 && plan.Cache == nil {
		return "", nil
	}
	if s.artifacts == nil {
		if len(plan.Ingested) == 0 && len(plan.Artifacts) == 0 && plan.Cache == nil {
			return "", nil
		}
		return models.ReasonArtifactsUnavailable, nil
	}
	prepared, err := s.artifacts.Prepare(ctx, q, artifactindex.Request{
		OrganizationID: st.run.OrganizationId, RunID: st.run.Id, RunJobID: rj.Id, Needs: rj.Needs,
		Ingested: plan.Ingested, FeedsDeploy: feedsDeploy(plan, spec),
	})
	if err != nil {
		return "", err
	}
	if prepared.Refused == artifactindex.ReasonIngestedGone {
		return models.ReasonIngestedGone, nil
	}
	if prepared.Refused != "" {
		return models.ReasonArtifactUntrusted, nil
	}
	for _, in := range prepared.Inputs {
		spec.Inputs = append(spec.Inputs, &runnerv1.ArtifactInput{
			Name: in.Name, Digest: in.Digest, SizeBytes: in.SizeBytes, DownloadToken: in.DownloadToken,
		})
	}
	for _, a := range plan.Artifacts {
		spec.Artifacts = append(spec.Artifacts, &runnerv1.ArtifactUpload{
			Name: a.Name, Paths: slices.Clone(a.Paths), MaxBytes: prepared.ArtifactMaxBytes, RetentionSeconds: a.RetentionSeconds,
		})
	}
	if plan.Cache == nil {
		return "", nil
	}
	key, err := plan.EvalCacheKey(ctx, evalCtx, spec.GetEnv())
	if err != nil {
		return models.ReasonExpressionFailed, nil
	}
	cache := &runnerv1.Cache{
		KeyExpression: key.Template, Paths: slices.Clone(plan.Cache.Paths), ReadOnly: st.run.IsFork, MaxBytes: prepared.CacheMaxBytes,
	}
	for _, h := range key.Hashes {
		cache.Hashes = append(cache.Hashes, &runnerv1.CacheHash{Patterns: slices.Clone(h)})
	}
	spec.Cache = cache
	return "", nil
}
