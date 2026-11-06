package e2e_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

type received struct {
	Name, Digest, Reason, Ref, Sha, RunUrl string
	SizeBytes                              int64
	Created, Triggered                     bool
	RunId                                  string
	RunNumber                              int64
}

func decode(t *testing.T, res apptest.Response) received {
	t.Helper()
	var r received
	res.Decode(t, &r)
	return r
}

func TestACorrectUploadMakesARunAndTheJobGetsTheSameDigest(t *testing.T) {
	w := newWorld(t)
	body := w.archive()
	digest := apptest.DigestHex(body)

	res := w.upload(body, nil)
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	assert.Equal(t, "ingest.artifact.stored", res.Message)
	got := decode(t, res)
	assert.Equal(t, digest, got.Digest, "the server computed the same digest")
	assert.EqualValues(t, len(body), got.SizeBytes)
	assert.True(t, got.Created)
	assert.True(t, got.Triggered)
	assert.NotEmpty(t, got.RunId)
	assert.True(t, w.env.BlobOnDisk(digest))
	assert.Equal(t, 1, w.runsOfKind(t, "artifact"))

	w.env.StartRuns()
	state, _ := w.env.RunState(uuid.MustParse(got.RunId))
	assert.Equal(t, "running", state)
	job := w.next(t)
	inputs := job.Grant.GetSpec().GetInputs()
	require.Len(t, inputs, 1)
	assert.Equal(t, "web", inputs[0].GetName())
	assert.Equal(t, digest, inputs[0].GetDigest(), "the job downloads the artifact with the digest that was uploaded")
	assert.EqualValues(t, len(body), inputs[0].GetSizeBytes())
	download := w.runner.GetBlob(job.Ref, inputs[0].GetDigest(), inputs[0].GetDownloadToken())
	require.Equal(t, http.StatusOK, download.Status, download.Code)
	assert.Equal(t, body, download.Body)
	w.succeed(t, job)
	state, _ = w.env.RunState(uuid.MustParse(got.RunId))
	assert.Equal(t, "succeeded", state)

	status := w.env.IngestStatus(w.token.Value, digest)
	require.Equal(t, http.StatusOK, status.Status, string(status.Raw))
	var list []received
	status.Decode(t, &list)
	require.Len(t, list, 1)
	assert.Equal(t, "refs/heads/main", list[0].Ref)
	assert.Equal(t, shaNext, list[0].Sha)
	assert.True(t, list[0].Triggered)
}

func TestTheRunReadsTheDefinitionFromTheDefaultBranchNotFromTheDeclaredCommit(t *testing.T) {
	w := newWorld(t)
	before := w.defs.Calls()
	res := w.upload(w.archive(), func(u *apptest.IngestUpload) { u.Sha = shaNext })
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	w.env.StartRuns()
	assert.Greater(t, w.defs.Calls(), before)
	var definitionSha, ref, baseRef string
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT "DefinitionSha", "Ref", "BaseRef" FROM "PipelineRuns" WHERE "Id" = $1`, uuid.MustParse(decode(t, res).RunId)).Scan(&definitionSha, &ref, &baseRef))
	assert.Equal(t, shaHead, definitionSha, "the head of the default branch, not the commit the uploader declared")
	assert.Equal(t, "refs/heads/main", baseRef)
	assert.Equal(t, "refs/heads/main", ref)
}

func TestARunFromAnUploadNeverPostsACommitStatus(t *testing.T) {
	w := newWorld(t)
	res := w.upload(w.archive(), nil)
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var n int
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM "CommitStatusReports" WHERE "RunId" = $1`, uuid.MustParse(decode(t, res).RunId)).Scan(&n))
	assert.Zero(t, n)
}

func TestARefOutsideThePatternsIsRefused(t *testing.T) {
	w := newWorld(t)
	body := w.archive()
	res := w.upload(body, func(u *apptest.IngestUpload) { u.Ref = "feature/x" })
	assert.Equal(t, http.StatusForbidden, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "REF_NOT_ALLOWED_FOR_TOKEN", res.Error.Code)
	w.assertNothingKept(t, body)

	res = w.upload(body, func(u *apptest.IngestUpload) { u.Ref, u.RefKind = "main", "tag" })
	assert.Equal(t, http.StatusForbidden, res.Status, "a tag named like an allowed branch is not allowed")

	res = w.upload(body, func(u *apptest.IngestUpload) { u.Ref, u.RefKind, u.Sha = "v1.2.0", "tag", shaTag })
	assert.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
}

func (w world) assertNothingKept(t *testing.T, body []byte) {
	t.Helper()
	assert.Zero(t, w.env.IngestedCount(w.project), "no artifact row")
	assert.False(t, w.env.BlobStored(apptest.DigestHex(body)), "no blob row")
	assert.False(t, w.env.BlobOnDisk(apptest.DigestHex(body)), "no blob file")
	assert.Zero(t, w.runsOfKind(t, "artifact"), "no run")
}

func TestEveryRefusalHasItsCodeAndLeavesNothingBehind(t *testing.T) {
	w := newWorldWith(t, func(c *config.Config) { c.Artifacts.IngestMaxBytes = 2 << 20 })
	good := w.archive()
	traversal := apptest.TarGz(map[string]string{"../evil.sh": "rm -rf /", "ok.txt": "fine"})
	bomb := func() []byte {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}))
		require.NoError(t, tw.Close())
		require.NoError(t, gz.Close())
		return buf.Bytes()
	}()
	notGzip := []byte("this is not an archive at all")
	noise := make([]byte, 3<<20)
	_, err := rand.Read(noise)
	require.NoError(t, err)
	huge := apptest.TarGz(map[string]string{"big.bin": string(noise)})
	revoked := w.env.SeedIngestToken(w.project, w.ownerID, apptest.IngestSeed{Revoked: true})
	expired := w.env.SeedIngestToken(w.project, w.ownerID, apptest.IngestSeed{})
	w.env.ExpireIngestToken(expired.ID)

	tests := []struct {
		name   string
		body   []byte
		edit   func(*apptest.IngestUpload)
		status int
		code   string
	}{
		{"a wrong digest", good, func(u *apptest.IngestUpload) { u.Digest = apptest.DigestHex([]byte("other")) }, http.StatusUnprocessableEntity, "DIGEST_MISMATCH"},
		{"a path with ../", traversal, nil, http.StatusUnprocessableEntity, "ARCHIVE_UNSAFE"},
		{"a link that leaves the folder", bomb, nil, http.StatusUnprocessableEntity, "ARCHIVE_UNSAFE"},
		{"bytes that are not a tar.gz", notGzip, nil, http.StatusUnprocessableEntity, "ARCHIVE_UNSAFE"},
		{"a file over the limit", huge, nil, http.StatusRequestEntityTooLarge, "ARTIFACT_TOO_LARGE"},
		{"a revoked token", good, func(u *apptest.IngestUpload) { u.Token = revoked.Value }, http.StatusUnauthorized, "INGEST_TOKEN_REVOKED"},
		{"an expired token", good, func(u *apptest.IngestUpload) { u.Token = expired.Value }, http.StatusUnauthorized, "INGEST_TOKEN_EXPIRED"},
		{"an unknown token", good, func(u *apptest.IngestUpload) { u.Token = apptest.IngestTokenPrefix + strings.Repeat("a", 43) }, http.StatusUnauthorized, "INGEST_TOKEN_INVALID"},
		{"a ref outside the patterns", good, func(u *apptest.IngestUpload) { u.Ref = "dev" }, http.StatusForbidden, "REF_NOT_ALLOWED_FOR_TOKEN"},
		{"a name that is a path", good, func(u *apptest.IngestUpload) { u.Name = "../web" }, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{"a commit that is not a sha", good, func(u *apptest.IngestUpload) { u.Sha = "main" }, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{"no length", good, func(u *apptest.IngestUpload) { u.Chunked = true }, http.StatusLengthRequired, "LENGTH_REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := w.upload(tt.body, tt.edit)
			assert.Equal(t, tt.status, res.Status, string(res.Raw))
			require.NotNil(t, res.Error, string(res.Raw))
			assert.Equal(t, tt.code, res.Error.Code)
			assert.Zero(t, w.env.IngestedCount(w.project), "no artifact row")
			assert.Zero(t, w.runsOfKind(t, "artifact"), "no run")
			digest := apptest.DigestHex(tt.body)
			assert.False(t, w.env.BlobStored(digest), "no blob row")
			assert.False(t, w.env.BlobOnDisk(digest), "no blob file")
		})
	}
}

func TestTheSameUploadTwiceIsOneArtifactAndOneRun(t *testing.T) {
	w := newWorld(t)
	body := w.archive()
	first := w.upload(body, nil)
	require.Equal(t, http.StatusCreated, first.Status, string(first.Raw))
	second := w.upload(body, nil)
	require.Equal(t, http.StatusOK, second.Status, string(second.Raw))
	assert.Equal(t, "ingest.artifact.duplicate", second.Message)
	a, b := decode(t, first), decode(t, second)
	assert.False(t, b.Created)
	assert.Equal(t, a.RunId, b.RunId, "the answer is the same, and no second run")
	assert.Equal(t, 1, w.env.IngestedCount(w.project))
	assert.Equal(t, 1, w.runsOfKind(t, "artifact"))

	other := w.upload(body, func(u *apptest.IngestUpload) { u.Sha = shaTag })
	require.Equal(t, http.StatusCreated, other.Status, "the same bytes for another commit is a new upload")
	assert.Equal(t, 2, w.env.IngestedCount(w.project))
}

func TestAUploadCanSkipTheTriggerOrFindNoMatchingPipeline(t *testing.T) {
	w := newWorld(t)
	res := w.upload(w.archive(), func(u *apptest.IngestUpload) { u.NoTrigger = true })
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	got := decode(t, res)
	assert.False(t, got.Triggered)
	assert.Equal(t, "TRIGGER_DISABLED", got.Reason)
	assert.Zero(t, w.runsOfKind(t, "artifact"))
	assert.Equal(t, 1, w.env.IngestedCount(w.project), "the file is kept")

	w.push(t, "refs/heads/main", shaNext)
	other := apptest.TarGz(map[string]string{"app.txt": "second build"})
	res = w.upload(other, nil)
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	got = decode(t, res)
	assert.False(t, got.Triggered)
	assert.Equal(t, "NO_ARTIFACT_TRIGGER", got.Reason, "the default branch has no on.artifact")
	assert.Zero(t, w.runsOfKind(t, "artifact"))
}

func TestAProjectWithoutAKnownDefaultBranchKeepsTheFileAndStartsNothing(t *testing.T) {
	w := newWorld(t)
	_, err := w.env.Pool.Exec(t.Context(), `UPDATE "PipelineRuns" SET "Trigger" = 'manual' WHERE "ProjectId" = $1`, w.project)
	require.NoError(t, err)
	res := w.upload(w.archive(), nil)
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	got := decode(t, res)
	assert.False(t, got.Triggered)
	assert.Equal(t, "DEFAULT_BRANCH_UNKNOWN", got.Reason)
}

func TestAJobWithoutItsIngestedArtifactIsSkippedNotFailed(t *testing.T) {
	w := newWorld(t)
	var state, why string
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT j."State", j."FailureReason" FROM "RunJobs" j JOIN "PipelineRuns" r ON r."Id" = j."RunId"
		 WHERE r."ProjectId" = $1 AND j."Key" = 'ship'`, w.project).Scan(&state, &why))
	assert.Equal(t, "skipped", state, "the push run has no uploaded web")
	assert.Equal(t, "INGESTED_ARTIFACT_NOT_IN_RUN", why)
	runState, _ := w.env.RunState(w.runIDOf(t, "push"))
	assert.Equal(t, "succeeded", runState)
}

func (w world) runIDOf(t *testing.T, trigger string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT "Id" FROM "PipelineRuns" WHERE "ProjectId" = $1 AND "Trigger" = $2 ORDER BY "Number" DESC LIMIT 1`, w.project, trigger).Scan(&id))
	return id
}

func TestTheLimitOfUploadsPerTokenAnswersTooManyRequests(t *testing.T) {
	w := newWorld(t)
	for i := range config.IngestPerTokenMax {
		body := apptest.TarGz(map[string]string{"n.txt": strings.Repeat("a", i+1)})
		res := w.upload(body, func(u *apptest.IngestUpload) { u.NoTrigger = true })
		require.Equal(t, http.StatusCreated, res.Status, "upload %d: %s", i, res.Raw)
	}
	res := w.upload(w.archive(), nil)
	assert.Equal(t, http.StatusTooManyRequests, res.Status, string(res.Raw))
	assert.NotEmpty(t, res.Header.Get("Retry-After"))
}
