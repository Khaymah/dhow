//go:build dhowtest

package app_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	archive "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Archive"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	"github.com/Khaymah/dhow/internal/cachekey"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func transferExecs(events []fake.Event) []fake.Event {
	var out []fake.Event
	for _, e := range execs(events) {
		if e.Step == sandbox.TransferStepIndex {
			out = append(out, e)
		}
	}
	return out
}

func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		paths = append(paths, strings.Split(name, "/")[0])
	}
	var buf bytes.Buffer
	require.NoError(t, archive.Pack(dir, paths, &buf))
	return buf.Bytes()
}

func evilTarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "../../etc/passwd", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4}))
	_, err := tw.Write([]byte("root"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func transferSpec() *runnerv1.JobSpec {
	return runnertest.Spec(&runnerv1.Step{Name: "build", Run: "make build"})
}

func TestAJobUploadsItsArtifactsAfterItSucceeds(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	spec := transferSpec()
	spec.Artifacts = []*runnerv1.ArtifactUpload{{Name: "web", Paths: []string{"dist"}, MaxBytes: 1 << 20}}
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{
		Steps: []fake.Step{{Writes: map[string]string{"dist/app.js": "console.log(1)\n", "other.txt": "left"}}},
	}, runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

	arts := srv.Artifacts()
	require.Len(t, arts, 1)
	assert.Equal(t, "web", arts[0].Name)
	data := srv.Blob(arts[0].Digest)
	sum := sha256.Sum256(data)
	assert.Equal(t, arts[0].Digest, hex.EncodeToString(sum[:]), "the artifact is named by its sha256")
	dst := t.TempDir()
	require.NoError(t, archive.Unpack(bytes.NewReader(data), dst, archive.Limits{}))
	got, err := os.ReadFile(filepath.Join(dst, "dist/app.js"))
	require.NoError(t, err)
	assert.Equal(t, "console.log(1)\n", string(got))
	_, err = os.Stat(filepath.Join(dst, "other.txt"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	transfers := transferExecs(running.Driver.Events())
	require.Len(t, transfers, 1)
	assert.Equal(t, []string{sandbox.AgentPath, sandbox.PackCommand, "--", "dist"}, transfers[0].Argv)
}

func TestAFailedJobUploadsNothing(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	spec := transferSpec()
	spec.Artifacts = []*runnerv1.ArtifactUpload{{Name: "web", Paths: []string{"dist"}, MaxBytes: 1 << 20}}
	spec.Cache = &runnerv1.Cache{KeyExpression: "deps", Paths: []string{".npm"}, MaxBytes: 1 << 20}
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{ExitCode: 2, Writes: map[string]string{"dist/a": "x", ".npm/b": "y"}}}},
		runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
	assert.Empty(t, srv.Artifacts())
	assert.Empty(t, srv.SavedCacheKeys())
	assert.Zero(t, srv.BlobPuts())
}

func TestTheArtifactsOfNeedsAreUnpackedBeforeTheFirstStep(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	digest := srv.PutBlobBytes(tarball(t, map[string]string{"dist/app.js": "built"}))
	spec := transferSpec()
	spec.Inputs = []*runnerv1.ArtifactInput{{Name: "web", Digest: digest, SizeBytes: int64(len(srv.Blob(digest))), DownloadToken: runnertest.InputToken(digest)}}
	srv.Enqueue(spec, nil)
	root := t.TempDir()
	running := r.Start(fake.Recording{Steps: []fake.Step{{}}}, runnertest.StartOptions{Workspace: root})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

	all := execs(running.Driver.Events())
	require.Len(t, all, 2)
	assert.Equal(t, sandbox.TransferStepIndex, all[0].Step, "the unpack runs first")
	assert.Equal(t, []string{sandbox.AgentPath, sandbox.UnpackCommand}, all[0].Argv)
	assert.Equal(t, int32(0), all[1].Step)
	lease := srv.WaitLease(t, finishWithin)
	got, err := os.ReadFile(filepath.Join(running.Driver.WorkspaceDir(lease.AttemptID), "dist/app.js"))
	require.NoError(t, err)
	assert.Equal(t, "built", string(got))
}

func TestADownloadThatDoesNotMatchItsDigestIsNeverUnpacked(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	digest := srv.PutBlobBytes(tarball(t, map[string]string{"dist/app.js": "built"}))
	srv.ConfigureBlobs(func(k *runnertest.BlobKnobs) { k.CorruptDownload = true })
	spec := transferSpec()
	spec.Inputs = []*runnerv1.ArtifactInput{{Name: "web", Digest: digest, SizeBytes: int64(len(srv.Blob(digest))), DownloadToken: runnertest.InputToken(digest)}}
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{}}}, runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_DOWNLOAD_FAILED, out.Reason)
	assert.Empty(t, execs(running.Driver.Events()), "neither the unpack nor a step ran")
}

func TestADownloadWithoutAGrantFails(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	digest := srv.PutBlobBytes(tarball(t, map[string]string{"a": "b"}))
	spec := transferSpec()
	spec.Inputs = []*runnerv1.ArtifactInput{{Name: "web", Digest: digest, SizeBytes: int64(len(srv.Blob(digest))), DownloadToken: "forged-token"}}
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{}}}, runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_DOWNLOAD_FAILED, out.Reason)
}

func TestAnArtifactThatLeavesTheWorkspaceFailsTheJob(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	digest := srv.PutBlobBytes(evilTarball(t))
	spec := transferSpec()
	spec.Inputs = []*runnerv1.ArtifactInput{{Name: "evil", Digest: digest, SizeBytes: int64(len(srv.Blob(digest))), DownloadToken: runnertest.InputToken(digest)}}
	srv.Enqueue(spec, nil)
	root := t.TempDir()
	running := r.Start(fake.Recording{Steps: []fake.Step{{}}}, runnertest.StartOptions{Workspace: root})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UNSAFE, out.Reason)
	for _, e := range execs(running.Driver.Events()) {
		assert.Equal(t, sandbox.TransferStepIndex, e.Step, "no step ran after the unsafe artifact")
	}
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the attempt's workspace is under the root")
}

func TestAnArtifactLargerThanItsLimitStopsTheJob(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	spec := transferSpec()
	spec.Artifacts = []*runnerv1.ArtifactUpload{{Name: "big", Paths: []string{"big.bin"}, MaxBytes: 256}}
	srv.Enqueue(spec, nil)
	noise := make([]byte, 64<<10)
	for i := range noise {
		noise[i] = byte(i*7919 + i/13)
	}
	running := r.Start(fake.Recording{Steps: []fake.Step{{Writes: map[string]string{"big.bin": string(noise)}}}},
		runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_TOO_LARGE, out.Reason)
	assert.Zero(t, srv.BlobPuts(), "nothing too large reached the server")
}

func cacheSpec(readOnly bool) *runnerv1.JobSpec {
	spec := transferSpec()
	var b cachekey.Builder
	b.Text("npm-")
	b.Hash(0)
	spec.Cache = &runnerv1.Cache{
		KeyExpression: b.String(), Hashes: []*runnerv1.CacheHash{{Patterns: []string{"package-lock.json"}}},
		Paths: []string{".npm"}, ReadOnly: readOnly, MaxBytes: 1 << 20,
	}
	return spec
}

func TestTheCacheIsSavedOnAMissAndRestoredOnTheNextRun(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	srv.Enqueue(cacheSpec(false), nil)
	rec := fake.Recording{
		Workspace: map[string]string{"package-lock.json": `{"lockfileVersion":3}`},
		Steps:     []fake.Step{{Writes: map[string]string{".npm/pkg.tgz": "cached bytes"}}},
	}
	root := t.TempDir()
	running := r.Start(rec, runnertest.StartOptions{Workspace: root})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())
	keys := srv.SavedCacheKeys()
	require.Len(t, keys, 1)
	assert.True(t, strings.HasPrefix(keys[0], "npm-"), keys[0])
	assert.Len(t, keys[0], len("npm-")+64)
	require.NoError(t, running.Stop())

	srv.Enqueue(cacheSpec(false), nil)
	second := r.Start(fake.Recording{Workspace: rec.Workspace, Steps: []fake.Step{{}}}, runnertest.StartOptions{Workspace: root})
	out = second.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())
	assert.Len(t, srv.SavedCacheKeys(), 1, "a hit is not saved again")
	leases := srv.Leases()
	got, err := os.ReadFile(filepath.Join(second.Driver.WorkspaceDir(leases[len(leases)-1].AttemptID), ".npm/pkg.tgz"))
	require.NoError(t, err)
	assert.Equal(t, "cached bytes", string(got))
}

func TestAReadOnlyCacheIsNeverSaved(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	srv.Enqueue(cacheSpec(true), nil)
	running := r.Start(fake.Recording{
		Workspace: map[string]string{"package-lock.json": "{}"},
		Steps:     []fake.Step{{Writes: map[string]string{".npm/pkg.tgz": "poison"}}},
	}, runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())
	assert.Empty(t, srv.SavedCacheKeys())
	assert.Zero(t, srv.BlobPuts())
	for _, e := range transferExecs(running.Driver.Events()) {
		assert.NotEqual(t, sandbox.PackCommand, e.Argv[1], "a read-only cache is never packed")
	}
}

func TestAMalformedDigestFromTheServerIsNeverUsed(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	spec := transferSpec()
	spec.Cache = &runnerv1.Cache{KeyExpression: "deps", Paths: []string{".npm"}, MaxBytes: 1 << 20, ReadOnly: true}
	srv.SeedCache("deps", "../../short")
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{Steps: []fake.Step{{}}}, runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, "a bad cache answer is only a warning: %s", out.Reason)
	for _, e := range transferExecs(running.Driver.Events()) {
		assert.NotEqual(t, sandbox.UnpackCommand, e.Argv[1], "nothing is unpacked from a malformed digest")
	}
	assert.Zero(t, srv.Calls("GET /blobs/"), "nothing is downloaded either")
}

const leakedSecret = "a-secret-value-that-must-not-leave"

func withSecret(spec *runnerv1.JobSpec) *runnerv1.JobSpec {
	spec.Secrets = []*runnerv1.SecretRef{{Name: "TOKEN", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "TOKEN"}, StepIndexes: []int32{0}}}
	return spec
}

func TestAnArtifactHoldingASecretFailsTheJobAndIsNotUploaded(t *testing.T) {
	for name, content := range map[string]string{
		"raw":    "token=" + leakedSecret + "\n",
		"base64": "dG9rZW49" + "YS1zZWNyZXQtdmFsdWUtdGhhdC1tdXN0LW5vdC1sZWF2ZQ==",
	} {
		t.Run(name, func(t *testing.T) {
			srv := runnertest.NewServer(t)
			r := checkoutRunner(t, srv)
			spec := withSecret(transferSpec())
			spec.Artifacts = []*runnerv1.ArtifactUpload{{Name: "web", Paths: []string{"dist"}, MaxBytes: 1 << 20}}
			srv.Enqueue(spec, map[string][]byte{"TOKEN": []byte(leakedSecret)})
			running := r.Start(fake.Recording{
				Steps: []fake.Step{{Writes: map[string]string{"dist/config.txt": content}}},
			}, runnertest.StartOptions{Workspace: t.TempDir()})
			out := running.Next(t, finishWithin)
			require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
			assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_CONTAINS_SECRET, out.Reason)
			assert.Empty(t, srv.Artifacts(), "nothing was uploaded")
		})
	}
}

func TestACacheHoldingASecretIsNotSaved(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	srv.Enqueue(withSecret(cacheSpec(false)), map[string][]byte{"TOKEN": []byte(leakedSecret)})
	rec := fake.Recording{
		Workspace: map[string]string{"package-lock.json": `{"lockfileVersion":3}`},
		Steps:     []fake.Step{{Writes: map[string]string{".npm/.npmrc": "//registry/:_authToken=" + leakedSecret}}},
	}
	running := r.Start(rec, runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, "a cache is optional, so the job still succeeds")
	assert.Empty(t, srv.SavedCacheKeys())
}

func TestAnArtifactWithoutSecretsIsUploadedWhenTheJobHasSecrets(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	spec := withSecret(transferSpec())
	spec.Artifacts = []*runnerv1.ArtifactUpload{{Name: "web", Paths: []string{"dist"}, MaxBytes: 1 << 20}}
	srv.Enqueue(spec, map[string][]byte{"TOKEN": []byte(leakedSecret)})
	running := r.Start(fake.Recording{
		Steps: []fake.Step{{Writes: map[string]string{"dist/app.js": "console.log(1)\n"}}},
	}, runnertest.StartOptions{Workspace: t.TempDir()})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())
	assert.Len(t, srv.Artifacts(), 1)
}
