package e2e_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	artifactindex "github.com/Khaymah/dhow/apps/server/internal/Core/ArtifactIndex"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var archiveBytes = []byte("\x1f\x8b deterministic tar.gz of dist")

func TestAnArtifactPassesFromBuildToDeployCheckWithTheSameDigest(t *testing.T) {
	w := newWorld(t)
	run := w.push(t, "refs/heads/main", shaA)
	build := w.next(t)
	spec := build.Grant.GetSpec()
	require.Len(t, spec.GetArtifacts(), 1)
	assert.Equal(t, "web", spec.GetArtifacts()[0].GetName())
	assert.Equal(t, []string{"dist"}, spec.GetArtifacts()[0].GetPaths())
	assert.Positive(t, spec.GetArtifacts()[0].GetMaxBytes())
	require.NotNil(t, spec.GetCache())
	assert.False(t, spec.GetCache().GetReadOnly())

	put := w.runner.PutBlob(build.Ref, "artifact", archiveBytes)
	require.Equal(t, http.StatusCreated, put.Status, string(put.Body))
	require.NoError(t, w.runner.CommitArtifact(build.Ref, "web", archiveBytes))
	w.succeed(t, build)

	check := w.next(t)
	inputs := check.Grant.GetSpec().GetInputs()
	require.Len(t, inputs, 1)
	assert.Equal(t, "web", inputs[0].GetName())
	assert.Equal(t, apptest.DigestHex(archiveBytes), inputs[0].GetDigest(), "the next job gets the same digest")
	assert.EqualValues(t, len(archiveBytes), inputs[0].GetSizeBytes())
	got := w.runner.GetBlob(check.Ref, inputs[0].GetDigest(), inputs[0].GetDownloadToken())
	require.Equal(t, http.StatusOK, got.Status, got.Code)
	assert.Equal(t, archiveBytes, got.Body)
	w.succeed(t, check)
	state, _ := w.env.RunState(run)
	assert.Equal(t, "succeeded", state)
	assert.NotContains(t, w.env.Logs.String(), inputs[0].GetDownloadToken(), "a download token is never logged")

	res := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/runs/"+run.String()+"/artifacts"))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var list []struct {
		Id, Name, Digest, RunJobKey string
		SizeBytes                   int64
		Untrusted                   bool
	}
	res.Decode(t, &list)
	require.Len(t, list, 1)
	assert.Equal(t, "web", list[0].Name)
	assert.Equal(t, "build", list[0].RunJobKey)
	assert.False(t, list[0].Untrusted)
	dl := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/artifacts/"+list[0].Id+"/download"))
	require.Equal(t, http.StatusOK, dl.Status, string(dl.Raw))
	assert.Equal(t, archiveBytes, dl.Raw)
	assert.Equal(t, `attachment; filename="web.tar.gz"`, dl.Header.Get("Content-Disposition"))
	assert.Equal(t, "nosniff", dl.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "application/gzip", dl.Header.Get("Content-Type"))
}

func blobFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, p)
		}
		return nil
	}))
	return out
}

func TestAWrongDigestIsRefusedAndNeverStored(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	g := w.next(t)
	res := w.runner.PutBlobAs(g.Ref, "artifact", apptest.DigestHex([]byte("something else")), archiveBytes)
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
	assert.Equal(t, "DIGEST_MISMATCH", res.Code)
	assert.Empty(t, blobFiles(t, w.env.Config.Artifacts.BlobDir), "no file is left, not even a temporary one")
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Blobs"`).Scan(&n))
	assert.Zero(t, n)
	err := w.runner.CommitArtifact(g.Ref, "web", archiveBytes)
	assert.Equal(t, "BLOB_NOT_FOUND", reason(err), "a blob that was never stored cannot be recorded")
}

func TestUploadsAndDownloadsNeedALiveLeaseAndTheCurrentFencingToken(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	g := w.next(t)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "artifact", archiveBytes).Status)

	stale := &runnerv1.LeaseRef{LeaseId: g.Ref.GetLeaseId(), FencingToken: g.Ref.GetFencingToken() - 1}
	res := w.runner.PutBlob(stale, "artifact", archiveBytes)
	assert.Equal(t, http.StatusConflict, res.Status)
	assert.Equal(t, "LEASE_LOST", res.Code)
	assert.Equal(t, "LEASE_LOST", reason(w.runner.CommitArtifact(stale, "web", archiveBytes)))
	_, err := w.runner.LookupCache(stale, "deps-v1")
	assert.Equal(t, "LEASE_LOST", reason(err))

	require.NoError(t, w.runner.CommitArtifact(g.Ref, "web", archiveBytes))
	w.succeed(t, g)
	res = w.runner.PutBlob(g.Ref, "artifact", archiveBytes)
	assert.Equal(t, "LEASE_LOST", res.Code, "a finished lease uploads nothing")
	check := w.next(t)
	in := check.Grant.GetSpec().GetInputs()[0]
	_, err = w.env.Pool.Exec(context.Background(), `UPDATE "Leases" SET "ExpiresAt" = now() - interval '1 second' WHERE "Id" = $1`, check.Ref.GetLeaseId())
	require.NoError(t, err)
	got := w.runner.GetBlob(check.Ref, in.GetDigest(), in.GetDownloadToken())
	assert.Equal(t, http.StatusConflict, got.Status)
	assert.Equal(t, "LEASE_LOST", got.Code, "an expired lease downloads nothing")
}

func TestADownloadNeedsAGrantForThatExactDigest(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	build := w.next(t)
	other := []byte("another archive")
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(build.Ref, "artifact", archiveBytes).Status)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(build.Ref, "cache", other).Status)
	require.NoError(t, w.runner.CommitArtifact(build.Ref, "web", archiveBytes))
	w.succeed(t, build)
	check := w.next(t)
	in := check.Grant.GetSpec().GetInputs()[0]
	for name, c := range map[string]struct{ digest, token string }{
		"no token":                       {in.GetDigest(), ""},
		"a token for another digest":     {apptest.DigestHex(other), in.GetDownloadToken()},
		"a made up token":                {in.GetDigest(), "AQAAAAAAAAAAAAAA"},
		"a token with one character off": {in.GetDigest(), flip(in.GetDownloadToken())},
	} {
		res := w.runner.GetBlob(check.Ref, c.digest, c.token)
		assert.Equal(t, http.StatusForbidden, res.Status, name)
		assert.Equal(t, "DOWNLOAD_GRANT_REQUIRED", res.Code, name)
	}
	w.push(t, "refs/heads/main", shaB)
	b2 := w.next(t)
	res := w.runner.GetBlob(b2.Ref, in.GetDigest(), in.GetDownloadToken())
	assert.Equal(t, "DOWNLOAD_GRANT_REQUIRED", res.Code, "a grant of one run job does not open the blob for another")
}

func flip(token string) string {
	b := []byte(token)
	if b[len(b)-1] == 'A' {
		b[len(b)-1] = 'B'
	} else {
		b[len(b)-1] = 'A'
	}
	return string(b)
}

func TestAnArtifactLargerThanTheLimitIsRefusedBeforeItFillsTheDisk(t *testing.T) {
	w := newWorldWith(t, func(c *config.Config) { c.Artifacts.MaxBytes = 1024 })
	w.push(t, "refs/heads/main", shaA)
	g := w.next(t)
	big := make([]byte, 4096)
	res := w.runner.PutBlob(g.Ref, "artifact", big)
	assert.Equal(t, http.StatusRequestEntityTooLarge, res.Status)
	assert.Equal(t, "BLOB_TOO_LARGE", res.Code)
	assert.Empty(t, blobFiles(t, w.env.Config.Artifacts.BlobDir))
	assert.Equal(t, "VALIDATION_FAILED: SizeBytes", reason(w.runner.CommitArtifact(g.Ref, "web", big)))
}

func TestAnUndeclaredUploadIsRefused(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	build := w.next(t)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(build.Ref, "artifact", archiveBytes).Status)
	assert.Equal(t, "NOT_DECLARED", reason(w.runner.CommitArtifact(build.Ref, "secrets", archiveBytes)))
	require.NoError(t, w.runner.CommitArtifact(build.Ref, "web", archiveBytes))
	w.succeed(t, build)
	check := w.next(t)
	res := w.runner.PutBlob(check.Ref, "artifact", archiveBytes)
	assert.Equal(t, "NOT_DECLARED", res.Code, "deploy-check declares no artifacts")
	res = w.runner.PutBlob(check.Ref, "cache", archiveBytes)
	assert.Equal(t, "NOT_DECLARED", res.Code, "deploy-check has no cache")
}

func TestArtifactsOfAForkRunAreUntrustedAndNeverFeedADeploy(t *testing.T) {
	w := newWorld(t)
	run := w.pullRequest(t, 12, shaB, triggers.PullOriginFork)
	build := w.next(t)
	assert.True(t, build.Grant.GetSpec().GetCache().GetReadOnly(), "a fork reads the cache only")
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(build.Ref, "artifact", archiveBytes).Status)
	require.NoError(t, w.runner.CommitArtifact(build.Ref, "web", archiveBytes))
	var untrusted bool
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Untrusted" FROM "Artifacts" WHERE "RunId" = $1`, run).Scan(&untrusted))
	assert.True(t, untrusted)

	var runJob uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Id" FROM "RunJobs" WHERE "RunId" = $1 AND "Key" = 'deploy-check'`, run).Scan(&runJob))
	for _, deploy := range []bool{false, true} {
		var out artifactindex.Prepared
		require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
			var err error
			out, err = w.env.Wired.Artifacts.Prepare(context.Background(), q, artifactindex.Request{
				OrganizationID: w.org, RunID: run, RunJobID: runJob, Needs: []string{"build"}, FeedsDeploy: deploy,
			})
			return err
		}))
		if deploy {
			assert.Equal(t, artifactindex.ReasonUntrustedForDeploy, out.Refused)
			assert.Empty(t, out.Inputs, "nothing of an untrusted artifact reaches a deploy")
		} else {
			assert.Empty(t, out.Refused)
			require.Len(t, out.Inputs, 1)
			assert.True(t, out.Inputs[0].Untrusted)
		}
	}
}
