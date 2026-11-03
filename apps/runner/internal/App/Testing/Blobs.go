//go:build dhowtest

package runnertest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const blobPath = "/blobs/"

type BlobKnobs struct {
	MaxBytes        int64
	CorruptDownload bool
	ForkRun         bool
}

type CommittedArtifact struct {
	Name, Digest string
	Size         int64
}

type blobState struct {
	knobs     BlobKnobs
	blobs     map[string][]byte
	kinds     map[string]string
	artifacts []CommittedArtifact
	cache     map[string]string
	saved     []string
	puts      int
}

func prefix(digest string) string { return digest[:min(len(digest), 16)] }

func InputToken(digest string) string { return "in-" + prefix(digest) }

func cacheToken(digest string) string { return "ca-" + prefix(digest) }

func (s *Server) ConfigureBlobs(fn func(*BlobKnobs)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.blobs.knobs)
}

func (s *Server) PutBlobBytes(data []byte) string {
	sum := sha256.Sum256(data)
	d := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs.blobs[d] = append([]byte(nil), data...)
	return d
}

func (s *Server) Blob(digest string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.blobs.blobs[digest]...)
}

func (s *Server) Artifacts() []CommittedArtifact {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]CommittedArtifact(nil), s.blobs.artifacts...)
}

func (s *Server) SeedCache(key, digest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs.cache[key] = digest
}

func (s *Server) SavedCacheKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.blobs.saved...)
}

func (s *Server) BlobPuts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blobs.puts
}

func blobFail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"Code": code})
}

func (s *Server) blobLease(r *http.Request) (*Lease, int, string) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	fencing, _ := strconv.ParseInt(r.Header.Get("Dhow-Fencing-Token"), 10, 64)
	expiry, ok := s.sessions[token]
	if !ok || time.Now().After(expiry) {
		return nil, http.StatusUnauthorized, "UNAUTHENTICATED"
	}
	l, err := s.active(&runnerv1.LeaseRef{LeaseId: r.Header.Get("Dhow-Lease-Id"), FencingToken: fencing})
	if err != nil {
		return nil, http.StatusConflict, "LEASE_LOST"
	}
	return l, 0, ""
}

func (s *Server) serveBlob(w http.ResponseWriter, r *http.Request) {
	digest := strings.TrimPrefix(r.URL.Path, blobPath)
	s.mu.Lock()
	s.calls[r.Method+" "+blobPath]++
	_, status, code := s.blobLease(r)
	knobs := s.blobs.knobs
	s.mu.Unlock()
	if status != 0 {
		blobFail(w, status, code)
		return
	}
	if r.Method == http.MethodGet {
		s.getBlob(w, r, digest, knobs)
		return
	}
	limit := knobs.MaxBytes
	if limit <= 0 {
		limit = 64 << 20
	}
	if r.ContentLength > limit {
		blobFail(w, http.StatusRequestEntityTooLarge, "BLOB_TOO_LARGE")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	sum := sha256.Sum256(data)
	if err != nil || int64(len(data)) > limit || hex.EncodeToString(sum[:]) != digest {
		blobFail(w, http.StatusUnprocessableEntity, "DIGEST_MISMATCH")
		return
	}
	s.mu.Lock()
	s.blobs.blobs[digest] = data
	s.blobs.kinds[digest] = r.Header.Get("Dhow-Blob-Kind")
	s.blobs.puts++
	s.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) getBlob(w http.ResponseWriter, r *http.Request, digest string, knobs BlobKnobs) {
	token := r.Header.Get("Dhow-Download-Token")
	s.mu.Lock()
	data, ok := s.blobs.blobs[digest]
	s.mu.Unlock()
	if token != InputToken(digest) && token != cacheToken(digest) {
		blobFail(w, http.StatusForbidden, "DOWNLOAD_GRANT_REQUIRED")
		return
	}
	if !ok {
		blobFail(w, http.StatusNotFound, "BLOB_NOT_FOUND")
		return
	}
	if knobs.CorruptDownload {
		data = bytes.Repeat([]byte{'x'}, len(data))
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func (h handlers) Commit(_ context.Context, req *connect.Request[runnerv1.ArtifactServiceCommitRequest]) (*connect.Response[runnerv1.ArtifactServiceCommitResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.active(req.Msg.GetLease()); err != nil {
		return nil, err
	}
	data, ok := s.blobs.blobs[req.Msg.GetDigest()]
	if !ok || int64(len(data)) != req.Msg.GetSizeBytes() {
		return nil, rpcError(connect.CodeNotFound, "BLOB_NOT_FOUND")
	}
	s.blobs.artifacts = append(s.blobs.artifacts, CommittedArtifact{Name: req.Msg.GetName(), Digest: req.Msg.GetDigest(), Size: req.Msg.GetSizeBytes()})
	return connect.NewResponse(&runnerv1.ArtifactServiceCommitResponse{}), nil
}

type cacheHandlers struct{ s *Server }

func (h cacheHandlers) Lookup(_ context.Context, req *connect.Request[runnerv1.CacheServiceLookupRequest]) (*connect.Response[runnerv1.CacheServiceLookupResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.active(req.Msg.GetLease()); err != nil {
		return nil, err
	}
	d, ok := s.blobs.cache[req.Msg.GetKey()]
	if !ok {
		return connect.NewResponse(&runnerv1.CacheServiceLookupResponse{}), nil
	}
	return connect.NewResponse(&runnerv1.CacheServiceLookupResponse{
		Hit: true, Digest: d, SizeBytes: int64(len(s.blobs.blobs[d])), DownloadToken: cacheToken(d),
	}), nil
}

func (h cacheHandlers) Commit(_ context.Context, req *connect.Request[runnerv1.CacheServiceCommitRequest]) (*connect.Response[runnerv1.CacheServiceCommitResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.active(req.Msg.GetLease()); err != nil {
		return nil, err
	}
	if s.blobs.knobs.ForkRun {
		return nil, rpcError(connect.CodePermissionDenied, "CACHE_READ_ONLY")
	}
	if _, ok := s.blobs.cache[req.Msg.GetKey()]; ok {
		return connect.NewResponse(&runnerv1.CacheServiceCommitResponse{}), nil
	}
	s.blobs.cache[req.Msg.GetKey()] = req.Msg.GetDigest()
	s.blobs.saved = append(s.blobs.saved, req.Msg.GetKey())
	return connect.NewResponse(&runnerv1.CacheServiceCommitResponse{Stored: true}), nil
}
