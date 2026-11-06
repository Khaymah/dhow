package artifacts

import (
	"context"
	"fmt"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	artifactindex "github.com/Khaymah/dhow/apps/server/internal/Core/ArtifactIndex"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

func (s *Service) addIngested(
	ctx context.Context, tx *sqlc.Queries, r artifactindex.Request, expires time.Time, out artifactindex.Prepared,
) (artifactindex.Prepared, error) {
	if len(r.Ingested) == 0 {
		return out, nil
	}
	rows, err := tx.ListRunIngestedArtifacts(ctx, sqlc.ListRunIngestedArtifactsParams{
		OrganizationID: r.OrganizationID, RunID: r.RunID, Names: r.Ingested,
	})
	if err != nil {
		return out, err
	}
	if len(rows) != len(r.Ingested) {
		return artifactindex.Prepared{Refused: artifactindex.ReasonIngestedGone}, nil
	}
	for _, a := range rows {
		d, ok := blob.DigestOf(a.Digest)
		if !ok {
			return out, fmt.Errorf("ingested artifact %s has a malformed digest", a.Name)
		}
		out.Inputs = append(out.Inputs, artifactindex.Input{
			Name: a.Name, Digest: d.String(), SizeBytes: a.SizeBytes, DownloadToken: s.grants.issue(grantInput, r.RunJobID, d, expires),
		})
	}
	return out, nil
}

func (s *Service) sweepIngested(ctx context.Context) (pinned, expired int64, err error) {
	q := s.queries()
	pinned, err = q.PinIngestedArtifacts(ctx, config.IngestPinMax.Seconds())
	if err != nil {
		return 0, 0, fmt.Errorf("pin ingested artifacts: %w", err)
	}
	for {
		n, err := q.ExpireIngestedArtifacts(ctx, config.BlobSweepBatch)
		if err != nil {
			return pinned, expired, fmt.Errorf("expire ingested artifacts: %w", err)
		}
		expired += n
		if n < config.BlobSweepBatch {
			return pinned, expired, nil
		}
	}
}
