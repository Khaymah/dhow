package artifacts

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

type artifactHandler struct{ service *Service }

var _ runnerv1connect.ArtifactServiceHandler = artifactHandler{}

func (h artifactHandler) Commit(ctx context.Context, req *connect.Request[runnerv1.CommitArtifactRequest]) (*connect.Response[runnerv1.CommitArtifactResponse], error) {
	m := req.Msg
	if err := h.service.CommitArtifact(ctx, m.GetLease(), m.GetName(), m.GetDigest(), m.GetSizeBytes()); err != nil {
		return nil, rpcError(err)
	}
	return connect.NewResponse(&runnerv1.CommitArtifactResponse{}), nil
}

type cacheHandler struct{ service *Service }

var _ runnerv1connect.CacheServiceHandler = cacheHandler{}

func (h cacheHandler) Lookup(ctx context.Context, req *connect.Request[runnerv1.LookupCacheRequest]) (*connect.Response[runnerv1.LookupCacheResponse], error) {
	res, err := h.service.LookupCache(ctx, req.Msg.GetLease(), req.Msg.GetKey())
	if err != nil {
		return nil, rpcError(err)
	}
	return connect.NewResponse(res), nil
}

func (h cacheHandler) Commit(ctx context.Context, req *connect.Request[runnerv1.CommitCacheRequest]) (*connect.Response[runnerv1.CommitCacheResponse], error) {
	m := req.Msg
	stored, err := h.service.CommitCache(ctx, m.GetLease(), m.GetKey(), m.GetDigest(), m.GetSizeBytes())
	if err != nil {
		return nil, rpcError(err)
	}
	return connect.NewResponse(&runnerv1.CommitCacheResponse{Stored: stored}), nil
}

func touchBlob(ctx context.Context, q *sqlc.Queries, d blob.Digest, size int64) error {
	if err := q.LockBlobsShared(ctx, config.BlobSweepLockKey); err != nil {
		return err
	}
	got, err := q.TouchBlob(ctx, d[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return errBlobMissing()
	}
	if err != nil {
		return err
	}
	if got != size {
		return errInvalid("SizeBytes")
	}
	return nil
}

func (s *Service) CommitArtifact(ctx context.Context, lease *runnerv1.LeaseRef, name, digest string, size int64) error {
	ref, err := refOf(lease)
	if err != nil {
		return err
	}
	d, err := blob.ParseDigest(digest)
	if err != nil {
		return errInvalid("Digest")
	}
	return s.withLease(ctx, ref, func(q *sqlc.Queries, l scope) error {
		if !l.pipeline() {
			return errNotPipeline()
		}
		decl, ok := declaredArtifact(l.spec, name)
		if !ok {
			return errNotDeclared()
		}
		if size < 0 || size > s.cfg.MaxBytes {
			return errInvalid("SizeBytes")
		}
		if err := touchBlob(ctx, q, d, size); err != nil {
			return err
		}
		owner, err := q.ArtifactOwner(ctx, sqlc.ArtifactOwnerParams{RunID: *l.row.RunId, Name: name})
		switch {
		case err == nil && owner != l.row.RunJobKey.String:
			return errNameTaken()
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if err := q.RetireArtifact(ctx, sqlc.RetireArtifactParams{RunID: *l.row.RunId, Name: name}); err != nil {
			return err
		}
		retention := s.cfg.Retention().Seconds()
		if r := decl.GetRetentionSeconds(); r > 0 && float64(r) < retention {
			retention = float64(r)
		}
		_, err = q.InsertArtifact(ctx, sqlc.InsertArtifactParams{
			OrganizationID: l.row.OrganizationId, ProjectID: *l.row.ProjectId, RunID: *l.row.RunId, RunJobID: *l.row.RunJobId,
			RunJobKey: l.row.RunJobKey.String, JobID: l.row.JobId, Name: name, Digest: d[:], SizeBytes: size,
			Untrusted: l.fork(), RetentionSeconds: retention,
		})
		return err
	})
}
