package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	digestParam  = "digest"
	uploadFailed = "failed"
	uploadStored = "stored"
)

func extendDeadline(set func(time.Time) error, idle time.Duration) error {
	if err := set(time.Now().Add(idle)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("extend the transfer deadline: %w", err)
	}
	return nil
}

type idleReader struct {
	r    io.Reader
	rc   *http.ResponseController
	idle time.Duration
}

func (i idleReader) Read(p []byte) (int, error) {
	if err := extendDeadline(i.rc.SetReadDeadline, i.idle); err != nil {
		return 0, err
	}
	return i.r.Read(p)
}

type idleWriter struct {
	w    io.Writer
	rc   *http.ResponseController
	idle time.Duration
}

func (i idleWriter) Write(p []byte) (int, error) {
	if err := extendDeadline(i.rc.SetWriteDeadline, i.idle); err != nil {
		return 0, err
	}
	return i.w.Write(p)
}

func (s *Service) blobRequest(r *http.Request) (blob.Digest, leaseRef, error) {
	d, err := blob.ParseDigest(r.PathValue(digestParam))
	if err != nil {
		return d, leaseRef{}, errInvalid("Digest")
	}
	ref, err := parseRef(r.Header.Get(config.BlobLeaseHeader), r.Header.Get(config.BlobFencingHeader))
	return d, ref, err
}

func (s *Service) respond(w http.ResponseWriter, r *http.Request, err error) {
	var rej *rejection
	switch {
	case errors.As(err, &rej):
		writeRejection(w, rej.status, rej.code)
	case errors.Is(err, context.Canceled):
		writeRejection(w, http.StatusRequestTimeout, "CANCELED")
	default:
		s.log.ErrorContext(r.Context(), "blob request failed", "request_id", middleware.RequestIDFrom(r.Context()), "error", err)
		writeRejection(w, http.StatusInternalServerError, CodeInternal)
	}
}

type upload struct {
	limit int64
	name  string
	claim uuid.UUID
	org   uuid.UUID
}

func (s *Service) uploadSlot(l scope, kind, name string) (upload, error) {
	if !l.pipeline() {
		return upload{}, errNotPipeline()
	}
	switch kind {
	case config.BlobKindArtifact:
		decl, ok := declaredArtifact(l.spec, name)
		if !ok {
			return upload{}, errNotDeclared()
		}
		limit := s.cfg.MaxBytes
		if m := decl.GetMaxBytes(); m > 0 && m < limit {
			limit = m
		}
		return upload{limit: limit, name: name}, nil
	case config.BlobKindCache:
		if l.spec.GetCache() == nil {
			return upload{}, errNotDeclared()
		}
		if _, ok := l.writeScope(); !ok || l.spec.GetCache().GetReadOnly() {
			return upload{}, errReadOnly()
		}
		return upload{limit: s.cfg.CacheMaxBytes}, nil
	default:
		return upload{}, errInvalid("Kind")
	}
}

func (s *Service) claimUpload(ctx context.Context, r *http.Request, ref leaseRef) (upload, error) {
	kind := r.Header.Get(config.BlobKindHeader)
	var up upload
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		l, err := lockLease(ctx, q, ref)
		if err != nil {
			return err
		}
		if up, err = s.uploadSlot(l, kind, r.Header.Get(config.BlobNameHeader)); err != nil {
			return err
		}
		switch {
		case r.ContentLength < 0:
			return errLength()
		case r.ContentLength > up.limit:
			return errTooLarge()
		}
		if kind == config.BlobKindArtifact {
			if err := s.quotaFits(ctx, q, l, r.ContentLength); err != nil {
				return err
			}
		}
		up.org = l.row.OrganizationId
		up.claim, err = q.ClaimUpload(ctx, sqlc.ClaimUploadParams{
			OrganizationID: l.row.OrganizationId, LeaseID: l.row.LeaseId, Kind: kind, Name: up.name,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errUploadUsed()
		}
		return err
	})
	return up, err
}

func (s *Service) PutBlob(w http.ResponseWriter, r *http.Request) {
	if err := s.putBlob(w, r); err != nil {
		s.respond(w, r, err)
	}
}

func (s *Service) putBlob(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	d, ref, err := s.blobRequest(r)
	if err != nil {
		return err
	}
	up, err := s.claimUpload(ctx, r, ref)
	if err != nil {
		return err
	}
	stored, err := s.storeBlob(ctx, w, r, d, ref, up)
	if err != nil {
		if ferr := s.queries().FinishUpload(context.WithoutCancel(ctx), sqlc.FinishUploadParams{ID: up.claim, State: uploadFailed}); ferr != nil {
			return errors.Join(err, ferr)
		}
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	body := `{"Digest":"` + stored.Digest.String() + `","SizeBytes":` + strconv.FormatInt(stored.Size, 10) + "}"
	if _, err := io.WriteString(w, body); err != nil {
		s.log.WarnContext(ctx, "the blob was stored but the answer did not reach the runner", "request_id", middleware.RequestIDFrom(ctx), "error", err)
	}
	return nil
}

func (s *Service) storeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, d blob.Digest, ref leaseRef, up upload) (blob.Stored, error) {
	rc := http.NewResponseController(w)
	body := idleReader{r: http.MaxBytesReader(w, r.Body, up.limit), rc: rc, idle: config.BlobIdleTimeout}
	pending, err := s.store.Stage(ctx, blob.Put{Digest: d, Size: r.ContentLength, MaxBytes: up.limit, Body: body})
	if err != nil {
		return blob.Stored{}, stageError(err)
	}
	defer pending.Abort()
	var out blob.Stored
	commitCtx := context.WithoutCancel(ctx)
	err = dbtx.Run(commitCtx, s.db, func(q *sqlc.Queries) error {
		if _, err := lockLease(commitCtx, q, ref); err != nil {
			return err
		}
		if err := q.LockBlobsShared(commitCtx, config.BlobSweepLockKey); err != nil {
			return err
		}
		if _, err := q.UpsertBlob(commitCtx, sqlc.UpsertBlobParams{Digest: d[:], SizeBytes: r.ContentLength, StorageKey: d.Key()}); err != nil {
			return err
		}
		if err := q.UpsertBlobOwner(commitCtx, sqlc.UpsertBlobOwnerParams{OrganizationID: up.org, Digest: d[:]}); err != nil {
			return err
		}
		if err := q.FinishUpload(commitCtx, sqlc.FinishUploadParams{ID: up.claim, State: uploadStored, Digest: d[:]}); err != nil {
			return err
		}
		var err error
		out, err = pending.Commit()
		return err
	})
	return out, err
}

func stageError(err error) error {
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, blob.ErrTooLarge), errors.As(err, &tooBig):
		return errTooLarge()
	case errors.Is(err, blob.ErrDigestMismatch), errors.Is(err, blob.ErrSizeMismatch):
		return errDigest()
	case errors.Is(err, blob.ErrNoSpace):
		return errFull()
	default:
		return err
	}
}

func (s *Service) GetBlob(w http.ResponseWriter, r *http.Request) {
	if err := s.getBlob(w, r); err != nil {
		s.respond(w, r, err)
	}
}

func (s *Service) getBlob(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	d, ref, err := s.blobRequest(r)
	if err != nil {
		return err
	}
	token := r.Header.Get(config.BlobDownloadTokenHeader)
	err = s.withLease(ctx, ref, func(q *sqlc.Queries, l scope) error {
		if !s.grants.verify(token, d, l.runJob(), l.row.LeaseId, s.now()) {
			return errGrant()
		}
		_, err := q.TouchBlob(ctx, d[:])
		if errors.Is(err, pgx.ErrNoRows) {
			return errBlobMissing()
		}
		return err
	})
	if err != nil {
		return err
	}
	f, err := s.store.Open(ctx, d)
	if errors.Is(err, blob.ErrNotFound) {
		return errBlobMissing()
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(f.Size(), 10))
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	if _, err := io.Copy(idleWriter{w: w, rc: rc, idle: config.BlobIdleTimeout}, f); err != nil {
		s.log.WarnContext(ctx, "a blob download stopped before its end", "request_id", middleware.RequestIDFrom(ctx), "error", err)
	}
	return nil
}
