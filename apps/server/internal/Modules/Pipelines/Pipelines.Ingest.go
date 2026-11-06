package pipelines

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

func artifactRefName(ref string) (string, bool) {
	switch {
	case strings.HasPrefix(ref, gitref.PrefixBranch):
		return refName(ref, gitref.PrefixBranch), true
	case strings.HasPrefix(ref, gitref.PrefixTag):
		return refName(ref, gitref.PrefixTag), true
	default:
		return "", false
	}
}

func artifactEnabled(a *pipelinespec.ArtifactTrigger, ref string) bool {
	if a == nil {
		return false
	}
	name, ok := artifactRefName(ref)
	return ok && matchesAny(a.Refs, name)
}

func checkArtifactTrigger(t triggers.Trigger) error {
	if t.OrganizationID == uuid.Nil || t.ProjectID == uuid.Nil || len(t.Artifacts) == 0 || len(t.Artifacts) > pipelinespec.MaxIngested {
		return fmt.Errorf("%w: an artifact trigger needs a project and 1 to %d artifacts", ErrInvalidTrigger, pipelinespec.MaxIngested)
	}
	ref, err := gitref.Parse(t.Ref)
	if err != nil || (ref.Kind != gitref.KindBranch && ref.Kind != gitref.KindTag) || !gitref.ValidSha(t.Sha) || gitref.IsZeroSha(t.Sha) {
		return fmt.Errorf("%w: ref or sha", ErrInvalidTrigger)
	}
	for _, a := range t.Artifacts {
		if a.ID == uuid.Nil || a.Name == "" {
			return fmt.Errorf("%w: an artifact without an id or a name", ErrInvalidTrigger)
		}
	}
	return nil
}

func declaresArtifact(source, ref string, t triggers.Trigger) bool {
	spec, _ := pipelinespec.Parse([]byte(source))
	if spec == nil || spec.On == nil || !artifactEnabled(spec.On.Artifact, ref) {
		return false
	}
	return slices.ContainsFunc(t.Artifacts, func(a triggers.IngestedRef) bool { return slices.Contains(spec.On.Artifact.Names, a.Name) })
}

func (s *Service) knownToIgnore(ctx context.Context, q *sqlc.Queries, project sqlc.GetRunProjectRow, t triggers.Trigger) (bool, error) {
	row, err := q.GetManualDefinition(ctx, sqlc.GetManualDefinitionParams{ProjectID: project.Id, OrganizationID: project.OrganizationId})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.DefinitionSource.Valid && !declaresArtifact(row.DefinitionSource.String, t.Ref, t), nil
}

func (s *Service) deliverArtifact(ctx context.Context, q *sqlc.Queries, t triggers.Trigger) (triggers.Accepted, error) {
	if err := checkArtifactTrigger(t); err != nil {
		return triggers.Accepted{}, err
	}
	project, err := q.GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: t.ProjectID, OrganizationID: t.OrganizationID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return triggers.Accepted{Reason: triggers.ReasonProjectArchived}, nil
	case err != nil:
		return triggers.Accepted{}, err
	case project.State != projectActive:
		return triggers.Accepted{Reason: triggers.ReasonProjectArchived}, nil
	case project.RepositoryId == nil:
		return triggers.Accepted{Reason: triggers.ReasonRepositoryNotLinked}, nil
	case project.DefaultBranch.String == "":
		return triggers.Accepted{Reason: triggers.ReasonDefaultBranchUnknown}, nil
	}
	repo := *project.RepositoryId
	base := gitref.PrefixBranch + project.DefaultBranch.String
	head, err := q.LatestTrustedSha(ctx, sqlc.LatestTrustedShaParams{RepositoryID: &repo, Ref: base})
	if errors.Is(err, pgx.ErrNoRows) {
		return triggers.Accepted{Reason: triggers.ReasonDefaultBranchUnknown}, nil
	}
	if err != nil {
		return triggers.Accepted{}, err
	}
	if ignore, err := s.knownToIgnore(ctx, q, project, t); err != nil || ignore {
		return triggers.Accepted{Reason: triggers.ReasonNoArtifactTrigger}, err
	}
	if err := q.LockRunTrigger(ctx, sqlc.LockRunTriggerParams{RepositoryID: repo, Ref: t.Ref}); err != nil {
		return triggers.Accepted{}, err
	}
	run, err := s.insertRun(ctx, q, newRun{
		project: project, trigger: triggers.EventArtifact, ref: t.Ref, baseRef: base, sha: t.Sha, baseSha: head, definitionSha: head,
		actor: t.Actor, repoID: &repo, provider: project.Provider.String,
	})
	if err != nil {
		return triggers.Accepted{}, err
	}
	for _, a := range t.Artifacts {
		err := q.InsertRunIngestedArtifact(ctx, sqlc.InsertRunIngestedArtifactParams{
			OrganizationID: run.OrganizationId, RunID: run.Id, Name: a.Name, IngestedArtifactID: a.ID,
		})
		if err != nil {
			return triggers.Accepted{}, err
		}
	}
	return triggers.Accepted{RunID: run.Id, Created: true}, nil
}

func (s *Service) ingestedMissing(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, plan *pipelinespec.RunJob) (bool, error) {
	if len(plan.Ingested) == 0 {
		return false, nil
	}
	names, err := q.ListRunIngestedNames(ctx, sqlc.ListRunIngestedNamesParams{RunID: run.Id, OrganizationID: run.OrganizationId})
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(plan.Ingested, func(want string) bool { return !slices.Contains(names, want) }), nil
}
