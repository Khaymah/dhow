package ingest

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
)

func provenanceOf(raw []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode the provenance of an ingested artifact: %w", err)
	}
	return out, nil
}

func artifactOf(a sqlc.IngestedArtifact, tokenName string, run *models.Run) (models.Artifact, error) {
	prov, err := provenanceOf(a.Provenance)
	if err != nil {
		return models.Artifact{}, err
	}
	return models.Artifact{
		ID: a.Id, Name: a.Name, Digest: hex.EncodeToString(a.Digest), SizeBytes: a.SizeBytes, Ref: a.Ref, RefKind: a.RefKind, Sha: a.Sha,
		Provenance: prov, ProvenanceVerified: a.ProvenanceVerified, TokenName: tokenName, Run: run, CreatedAt: a.CreatedAt,
		ExpiresAt: a.ExpiresAt, PinnedUntil: a.PinnedUntil,
	}, nil
}

func (s *Service) ListArtifacts(ctx context.Context, org, project uuid.UUID, page, limit int) (models.ArtifactPage, error) {
	q := s.queries()
	total, err := q.CountIngestedArtifacts(ctx, sqlc.CountIngestedArtifactsParams{OrganizationID: org, ProjectID: project})
	if err != nil {
		return models.ArtifactPage{}, err
	}
	rows, err := q.ListIngestedArtifacts(ctx, sqlc.ListIngestedArtifactsParams{
		OrganizationID: org, ProjectID: project, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // Bounded by the page parameters.
	})
	if err != nil {
		return models.ArtifactPage{}, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Id)
	}
	runs, err := s.runsOf(ctx, org, ids)
	if err != nil {
		return models.ArtifactPage{}, err
	}
	out := models.ArtifactPage{Items: make([]models.Artifact, 0, len(rows)), Total: total}
	for _, r := range rows {
		a, err := artifactOf(ingestedOf(r), r.TokenName, runs[r.Id])
		if err != nil {
			return models.ArtifactPage{}, err
		}
		out.Items = append(out.Items, a)
	}
	return out, nil
}

func ingestedOf(r sqlc.ListIngestedArtifactsRow) sqlc.IngestedArtifact {
	return sqlc.IngestedArtifact{
		Id: r.Id, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, IsDeleted: r.IsDeleted, OrganizationId: r.OrganizationId,
		ProjectId: r.ProjectId, TokenId: r.TokenId, Name: r.Name, Digest: r.Digest, SizeBytes: r.SizeBytes, Ref: r.Ref, RefKind: r.RefKind,
		Sha: r.Sha, Provenance: r.Provenance, ProvenanceVerified: r.ProvenanceVerified, ExpiresAt: r.ExpiresAt, PinnedUntil: r.PinnedUntil,
	}
}

func (s *Service) runsOf(ctx context.Context, org uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]*models.Run, error) {
	out := map[uuid.UUID]*models.Run{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.queries().ListIngestedRuns(ctx, sqlc.ListIngestedRunsParams{OrganizationID: org, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.IngestedArtifactId] = &models.Run{ID: r.RunId, Number: r.Number, State: r.State}
	}
	return out, nil
}
