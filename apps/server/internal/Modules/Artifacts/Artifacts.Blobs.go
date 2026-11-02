package artifacts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const digestParam = "digest"

type idleReader struct {
	r    io.Reader
	rc   *http.ResponseController
	idle time.Duration
}

func (i idleReader) Read(p []byte) (int, error) {
	_ = i.rc.SetReadDeadline(time.Now().Add(i.idle))
	return i.r.Read(p)
}

type idleWriter struct {
	w    io.Writer
	rc   *http.ResponseController
	idle time.Duration
}

func (i idleWriter) Write(p []byte) (int, error) {
	_ = i.rc.SetWriteDeadline(time.Now().Add(i.idle))
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

func (s *Service) uploadLimit(l scope, kind string) (int64, error) {
	if !l.pipeline() {
		return 0, errNotPipeline()
	}
	switch kind {
	case config.BlobKindArtifact:
		if len(l.spec.GetArtifacts()) == 0 {
			return 0, errNotDeclared()
		}
		return s.cfg.MaxBytes, nil
	case config.BlobKindCache:
		if l.spec.GetCache() == nil {
			return 0, errNotDeclared()
		}
		if _, ok := l.writeScope(); !ok || l.spec.GetCache().GetReadOnly() {
			return 0, errReadOnly()
		}
		return s.cfg.CacheMaxBytes, nil
	default:
		return 0, errInvalid("Kind")
	}
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
	var limit int64
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		l, err := lockLease(ctx, q, ref)
		if err != nil {
			return err
		}
		limit, err = s.uploadLimit(l, r.Header.Get(config.BlobKindHeader))
		return err
	})
	if err != nil {
		return err
	}
	switch {
	case r.ContentLength < 0:
		return errLength()
	case r.ContentLength > limit:
		return errTooLarge()
	}
	rc := http.NewResponseController(w)
	body := idleReader{r: http.MaxBytesReader(w, r.Body, limit), rc: rc, idle: config.BlobIdleTimeout}
	pending, err := s.store.Stage(ctx, blob.Put{Digest: d, Size: r.ContentLength, MaxBytes: limit, Body: body})
	if err != nil {
		return stageError(err)
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
		var err error
		if out, err = pending.Commit(); err != nil {
			return err
		}
		_, err = q.UpsertBlob(commitCtx, sqlc.UpsertBlobParams{Digest: d[:], SizeBytes: out.Size, StorageKey: out.Key})
		return err
	})
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"Digest":"` + out.Digest.String() + `","SizeBytes":` + strconv.FormatInt(out.Size, 10) + "}"))
	return nil
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
		if _, err := q.TouchBlob(ctx, d[:]); err != nil {
			return errBlobMissing()
		}
		return nil
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
	_, _ = io.Copy(idleWriter{w: w, rc: rc, idle: config.BlobIdleTimeout}, f)
	return nil
}
