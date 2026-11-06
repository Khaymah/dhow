package pipelines

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	artifactindex "github.com/Khaymah/dhow/apps/server/internal/Core/ArtifactIndex"
	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	rollbackJobKey   = "rollback"
	inputRelease     = "release"
	releaseSucceeded = "succeeded"
	kindDeploy       = "deploy"
)

func rollbackSource(image string, req deployments.SyntheticRequest) []byte {
	var b strings.Builder
	b.WriteString("version: 1\non:\n  api: {}\njobs:\n  " + rollbackJobKey + ":\n")
	fmt.Fprintf(&b, "    environment: %s\n    steps:\n      - uses: %s\n        with:\n          target: %s\n          %s: %s\n          %s: %s\n",
		quoted(req.EnvironmentName), quoted(image), quoted(req.TargetName), jobspec.TargetModeInput, jobspec.TargetModeRollback,
		inputRelease, quoted(req.ReleaseID))
	names := make([]string, 0, len(req.HealthInputs))
	for k := range req.HealthInputs {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(&b, "          %s: %s\n", k, quoted(req.HealthInputs[k]))
	}
	return []byte(b.String())
}

func (s *Service) attachRollback(ctx context.Context, q *sqlc.Queries, st *step, rj sqlc.RunJob, spec *runnerv1.JobSpec) (string, error) {
	if st.run.Trigger != triggerRollback {
		return "", nil
	}
	for _, t := range spec.GetTargets() {
		idx := int(t.GetStepIndex())
		if idx < 0 || idx >= len(spec.GetSteps()) {
			continue
		}
		inputs := spec.GetSteps()[idx].GetPlugin().GetInputs()
		if inputs[jobspec.TargetModeInput] != jobspec.TargetModeRollback {
			continue
		}
		reason, err := s.attachRollbackStep(ctx, q, st, rj, spec, t, idx)
		if err != nil || reason != "" {
			return reason, err
		}
	}
	return "", nil
}

func (s *Service) attachRollbackStep(
	ctx context.Context, q *sqlc.Queries, st *step, rj sqlc.RunJob, spec *runnerv1.JobSpec, t *runnerv1.StepTarget, idx int,
) (string, error) {
	target, err := uuid.Parse(t.GetTargetId())
	if err != nil {
		return models.ReasonRollbackReleaseGone, nil
	}
	inputs := spec.GetSteps()[idx].GetPlugin().GetInputs()
	rel, err := q.GetRelease(ctx, sqlc.GetReleaseParams{TargetID: target, ReleaseID: inputs[inputRelease], OrganizationID: st.run.OrganizationId})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (rel.Kind != kindDeploy || rel.State != releaseSucceeded)) {
		return models.ReasonRollbackReleaseGone, nil
	}
	if err != nil {
		return "", err
	}
	if s.artifacts == nil || len(rel.ArtifactDigest) != 32 {
		return "", nil
	}
	in, ok, err := s.artifacts.PrepareDigest(ctx, q, artifactindex.DigestRequest{
		OrganizationID: st.run.OrganizationId, ProjectID: st.run.ProjectId, RunJobID: rj.Id, Digest: rel.ArtifactDigest,
	})
	if err != nil || !ok {
		return "", err
	}
	if want := hex.EncodeToString(rel.ArtifactDigest); in.Digest != want {
		return models.ReasonRollbackReleaseGone, nil
	}
	spec.Inputs = append(spec.Inputs, &runnerv1.ArtifactInput{Name: in.Name, Digest: in.Digest, SizeBytes: in.SizeBytes, DownloadToken: in.DownloadToken})
	inputs[inputArtifact] = in.Name
	return "", nil
}
