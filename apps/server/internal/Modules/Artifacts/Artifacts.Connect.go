package artifacts

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

type artifactHandler struct{ service *Service }

var _ runnerv1connect.ArtifactServiceHandler = artifactHandler{}

func (h artifactHandler) Commit(ctx context.Context, req *connect.Request[runnerv1.ArtifactServiceCommitRequest]) (*connect.Response[runnerv1.ArtifactServiceCommitResponse], error) {
	m := req.Msg
	if err := h.service.CommitArtifact(ctx, m.GetLease(), m.GetName(), m.GetDigest(), m.GetSizeBytes()); err != nil {
		return nil, rpcError(err)
	}
	return connect.NewResponse(&runnerv1.ArtifactServiceCommitResponse{}), nil
}

type cacheHandler struct{ service *Service }

var _ runnerv1connect.CacheServiceHandler = cacheHandler{}

func (h cacheHandler) Lookup(ctx context.Context, req *connect.Request[runnerv1.CacheServiceLookupRequest]) (*connect.Response[runnerv1.CacheServiceLookupResponse], error) {
	res, err := h.service.LookupCache(ctx, req.Msg.GetLease(), req.Msg.GetKey())
	if err != nil {
		return nil, rpcError(err)
	}
	return connect.NewResponse(res), nil
}

func (h cacheHandler) Commit(ctx context.Context, req *connect.Request[runnerv1.CacheServiceCommitRequest]) (*connect.Response[runnerv1.CacheServiceCommitResponse], error) {
	m := req.Msg
	stored, err := h.service.CommitCache(ctx, m.GetLease(), m.GetKey(), m.GetDigest(), m.GetSizeBytes())
	if err != nil {
		return nil, rpcError(err)
	}
	return connect.NewResponse(&runnerv1.CacheServiceCommitResponse{Stored: stored}), nil
}

func touchBlob(ctx context.Context, q *sqlc.Queries, org uuid.UUID, d blob.Digest, size int64) error {
	if err := q.LockBlobsShared(ctx, config.BlobSweepLockKey); err != nil {
		return err
	}
	got, err := q.OwnedBlobSize(ctx, sqlc.OwnedBlobSizeParams{OrganizationID: org, Digest: d[:]})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && got != size) {
		return errBlobMissing()
	}
	if err != nil {
		return err
	}
	_, err = q.TouchBlob(ctx, d[:])
	return err
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
		if err := touchBlob(ctx, q, l.row.OrganizationId, d, size); err != nil {
			return err
		}
		if err := s.withinQuota(ctx, q, l, name, size); err != nil {
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

func (s *Service) withinQuota(ctx context.Context, q *sqlc.Queries, l scope, name string, size int64) error {
	org := l.row.OrganizationId
	if err := q.LockOrganizationArtifacts(ctx, org.String()); err != nil {
		return err
	}
	replaced, err := q.ArtifactSize(ctx, sqlc.ArtifactSizeParams{RunID: *l.row.RunId, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		replaced, err = 0, nil
	}
	if err != nil {
		return err
	}
	return s.quotaFits(ctx, q, l, size-replaced)
}

func (s *Service) quotaFits(ctx context.Context, q *sqlc.Queries, l scope, extra int64) error {
	used, err := q.OrganizationArtifactBytes(ctx, l.row.OrganizationId)
	if err != nil {
		return err
	}
	if used+extra > s.cfg.OrgMaxBytes {
		return errQuota()
	}
	if !l.fork() {
		return nil
	}
	forks, err := q.ProjectUntrustedArtifactBytes(ctx, *l.row.ProjectId)
	if err != nil {
		return err
	}
	if forks+extra > s.cfg.ForkMaxBytes() {
		return errQuota()
	}
	return nil
}
