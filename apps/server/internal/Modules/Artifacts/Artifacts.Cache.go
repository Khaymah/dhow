package artifacts

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/cachekey"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const evictPage = 100

func (s *Service) LookupCache(ctx context.Context, lease *runnerv1.LeaseRef, key string) (*runnerv1.CacheServiceLookupResponse, error) {
	ref, err := refOf(lease)
	if err != nil {
		return nil, err
	}
	if !cachekey.ValidKey(key) {
		return nil, errInvalid("Key")
	}
	out := &runnerv1.CacheServiceLookupResponse{}
	err = s.withLease(ctx, ref, func(q *sqlc.Queries, l scope) error {
		if !l.pipeline() {
			return errNotPipeline()
		}
		if l.spec.GetCache() == nil {
			return errNotDeclared()
		}
		scopes := l.restoreScopes()
		if len(scopes) == 0 {
			return nil
		}
		entry, err := q.FindCacheEntry(ctx, sqlc.FindCacheEntryParams{ProjectID: *l.row.ProjectId, Key: key, Scopes: scopes})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		d, ok := blob.DigestOf(entry.Digest)
		if !ok {
			return nil
		}
		if _, err := q.TouchBlob(ctx, d[:]); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		out.Hit, out.Digest, out.SizeBytes = true, d.String(), entry.SizeBytes
		out.DownloadToken = s.grants.issue(grantCache, l.row.LeaseId, d, s.now().Add(config.CacheGrantTTL))
		return nil
	})
	return out, err
}

func (s *Service) CommitCache(ctx context.Context, lease *runnerv1.LeaseRef, key, digest string, size int64) (bool, error) {
	ref, err := refOf(lease)
	if err != nil {
		return false, err
	}
	if !cachekey.ValidKey(key) {
		return false, errInvalid("Key")
	}
	d, err := blob.ParseDigest(digest)
	if err != nil {
		return false, errInvalid("Digest")
	}
	stored := false
	err = s.withLease(ctx, ref, func(q *sqlc.Queries, l scope) error {
		if !l.pipeline() {
			return errNotPipeline()
		}
		if l.spec.GetCache() == nil {
			return errNotDeclared()
		}
		where, ok := l.writeScope()
		if !ok || l.spec.GetCache().GetReadOnly() {
			return errReadOnly()
		}
		if size < 0 || size > s.cfg.CacheMaxBytes {
			return errInvalid("SizeBytes")
		}
		if err := q.LockProjectCache(ctx, l.row.ProjectId.String()); err != nil {
			return err
		}
		if err := touchBlob(ctx, q, l.row.OrganizationId, d, size); err != nil {
			return err
		}
		id, err := q.InsertCacheEntry(ctx, sqlc.InsertCacheEntryParams{
			OrganizationID: l.row.OrganizationId, ProjectID: *l.row.ProjectId, Scope: where, Key: key, Digest: d[:], SizeBytes: size,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		stored, err = s.evict(ctx, q, *l.row.ProjectId, id, where, l.defaultScope())
		return err
	})
	return stored, err
}

func (s *Service) evict(ctx context.Context, q *sqlc.Queries, project, added uuid.UUID, where, protected string) (bool, error) {
	total, err := q.ProjectCacheBytes(ctx, project)
	if err != nil {
		return false, err
	}
	includeProtected := protected == "" || where == protected
	for total > s.cfg.CacheMaxBytes {
		rows, err := q.OldestCacheEntries(ctx, sqlc.OldestCacheEntriesParams{
			ProjectID: project, IncludeProtected: includeProtected, ProtectedScope: protected, MaxRows: evictPage,
		})
		if err != nil {
			return false, err
		}
		var ids []uuid.UUID
		for _, r := range rows {
			if total <= s.cfg.CacheMaxBytes {
				break
			}
			if r.Id == added {
				continue
			}
			ids = append(ids, r.Id)
			total -= r.SizeBytes
		}
		if len(ids) == 0 {
			break
		}
		if _, err := q.EvictCacheEntries(ctx, ids); err != nil {
			return false, err
		}
	}
	if total <= s.cfg.CacheMaxBytes {
		return true, nil
	}
	_, err = q.EvictCacheEntries(ctx, []uuid.UUID{added})
	return false, err
}
