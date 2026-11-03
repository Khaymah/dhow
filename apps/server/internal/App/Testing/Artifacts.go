package apptest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"connectrpc.com/connect"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

type SeededArtifact struct {
	ID      uuid.UUID
	RunID   uuid.UUID
	Content []byte
	Digest  [32]byte
}

func (e *Env) SeedArtifact(project uuid.UUID, name string, content []byte, untrusted bool) SeededArtifact {
	e.T.Helper()
	ctx := context.Background()
	digest := sha256.Sum256(content)
	require.NoError(e.T, e.Wired.Artifacts.SeedBlob(ctx, digest, bytes.NewReader(content), int64(len(content))))
	run := e.SeedRun(project, "pending")
	var org, runJob uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position")
		 SELECT "OrganizationId", "Id", 'build', 'build', 'build', 0 FROM "PipelineRuns" WHERE "Id" = $1
		 ON CONFLICT ("RunId", "Key") DO UPDATE SET "UpdatedAt" = now()
		 RETURNING "OrganizationId", "Id"`, run).Scan(&org, &runJob))
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "Artifacts" ("OrganizationId", "ProjectId", "RunId", "RunJobId", "RunJobKey", "JobId", "Name", "Digest",
		 "SizeBytes", "Untrusted", "ExpiresAt")
		 VALUES ($1, $2, $3, $4, 'build', $5, $6, $7, $8, $9, now() + interval '30 days') RETURNING "Id"`,
		org, project, run, runJob, uuid.New(), name, digest[:], len(content), untrusted).Scan(&id))
	return SeededArtifact{ID: id, RunID: run, Content: content, Digest: digest}
}

type BlobResponse struct {
	Status int
	Code   string
	Body   []byte
}

func DigestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (r *TestRunner) blob(method, digest string, ref *runnerv1.LeaseRef, body []byte, headers map[string]string) BlobResponse {
	r.T.Helper()
	var reader io.Reader = http.NoBody
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, RunnerBaseURL+"/blobs/"+digest, reader)
	require.NoError(r.T, err)
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	if ref != nil {
		req.Header.Set("Dhow-Lease-Id", ref.GetLeaseId())
		req.Header.Set("Dhow-Fencing-Token", strconv.FormatInt(ref.GetFencingToken(), 10))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := r.Client.HTTP.Do(req)
	require.NoError(r.T, err)
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	require.NoError(r.T, err)
	out := BlobResponse{Status: res.StatusCode, Body: raw}
	if res.StatusCode >= 300 {
		var e struct{ Code string }
		_ = json.Unmarshal(raw, &e)
		out.Code = e.Code
	}
	return out
}

func (r *TestRunner) PutBlob(ref *runnerv1.LeaseRef, kind, name string, data []byte) BlobResponse {
	r.T.Helper()
	return r.PutBlobAs(ref, kind, name, DigestHex(data), data)
}

func (r *TestRunner) PutBlobAs(ref *runnerv1.LeaseRef, kind, name, digest string, data []byte) BlobResponse {
	r.T.Helper()
	return r.blob(http.MethodPut, digest, ref, data, map[string]string{"Dhow-Blob-Kind": kind, "Dhow-Blob-Name": name})
}

func (r *TestRunner) GetBlob(ref *runnerv1.LeaseRef, digest, token string) BlobResponse {
	r.T.Helper()
	return r.blob(http.MethodGet, digest, ref, nil, map[string]string{"Dhow-Download-Token": token})
}

func (r *TestRunner) Artifacts() runnerv1connect.ArtifactServiceClient {
	return runnerv1connect.NewArtifactServiceClient(r.Client.HTTP, RunnerBaseURL)
}

func (r *TestRunner) Cache() runnerv1connect.CacheServiceClient {
	return runnerv1connect.NewCacheServiceClient(r.Client.HTTP, RunnerBaseURL)
}

func (r *TestRunner) CommitArtifact(ref *runnerv1.LeaseRef, name string, data []byte) error {
	_, err := r.Artifacts().Commit(context.Background(), connect.NewRequest(&runnerv1.ArtifactServiceCommitRequest{
		Lease: ref, Name: name, Digest: DigestHex(data), SizeBytes: int64(len(data)),
	}))
	return err
}

func (r *TestRunner) LookupCache(ref *runnerv1.LeaseRef, key string) (*runnerv1.CacheServiceLookupResponse, error) {
	res, err := r.Cache().Lookup(context.Background(), connect.NewRequest(&runnerv1.CacheServiceLookupRequest{Lease: ref, Key: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (r *TestRunner) CommitCache(ref *runnerv1.LeaseRef, key string, data []byte) (bool, error) {
	res, err := r.Cache().Commit(context.Background(), connect.NewRequest(&runnerv1.CacheServiceCommitRequest{
		Lease: ref, Key: key, Digest: DigestHex(data), SizeBytes: int64(len(data)),
	}))
	if err != nil {
		return false, err
	}
	return res.Msg.GetStored(), nil
}
