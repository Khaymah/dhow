package artifacts

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts/Gen"
)

const messageListed = "artifacts.list.loaded"

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func artifactToGen(a sqlc.Artifact) gen.Artifact {
	d, _ := blob.DigestOf(a.Digest)
	return gen.Artifact{
		Id: a.Id, RunId: a.RunId, Name: a.Name, RunJobKey: a.RunJobKey, SizeBytes: a.SizeBytes, Digest: d.String(),
		Untrusted: a.Untrusted, CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt,
	}
}

func (c *Controller) ListRunArtifacts(ctx context.Context, req gen.ListRunArtifactsRequestObject) (gen.ListRunArtifactsResponseObject, error) {
	q := c.service.queries()
	visible, err := q.RunVisible(ctx, sqlc.RunVisibleParams{RunID: req.RunId, OrganizationID: req.Id, ProjectID: req.ProjectId})
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, apperrors.NotFound()
	}
	rows, err := q.ListRunArtifacts(ctx, sqlc.ListRunArtifactsParams{
		OrganizationID: req.Id, ProjectID: req.ProjectId, RunID: req.RunId, MaxRows: config.ArtifactListMax,
	})
	if err != nil {
		return nil, err
	}
	items := make([]gen.Artifact, 0, len(rows))
	for _, a := range rows {
		items = append(items, artifactToGen(a))
	}
	msg := messageListed
	return gen.ListRunArtifacts200JSONResponse{Success: true, Message: &msg, Data: &items}, nil
}

func DownloadName(name string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, name)
	clean = strings.TrimLeft(clean, ".")
	if clean == "" {
		clean = config.ArtifactDownloadNameFallback
	}
	return clean + ".tar.gz"
}

func (c *Controller) DownloadArtifact(ctx context.Context, req gen.DownloadArtifactRequestObject) (gen.DownloadArtifactResponseObject, error) {
	a, err := c.service.queries().GetArtifact(ctx, sqlc.GetArtifactParams{ID: req.ArtifactId, OrganizationID: req.Id, ProjectID: req.ProjectId})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound()
	}
	if err != nil {
		return nil, err
	}
	d, ok := blob.DigestOf(a.Digest)
	if !ok {
		return nil, apperrors.ArtifactGone()
	}
	f, err := c.service.store.Open(ctx, d)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperrors.ArtifactGone()
	}
	if err != nil {
		return nil, err
	}
	disposition := `attachment; filename="` + DownloadName(a.Name) + `"`
	nosniff := "nosniff"
	return gen.DownloadArtifact200ApplicationgzipResponse{
		Body: f, ContentLength: f.Size(),
		Headers: gen.DownloadArtifact200ResponseHeaders{ContentDisposition: &disposition, XContentTypeOptions: &nosniff},
	}, nil
}
