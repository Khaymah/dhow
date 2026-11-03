package e2e_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func (w world) blobFile(digest string) string {
	return filepath.Join(w.env.Config.Artifacts.BlobDir, digest[:2], digest)
}

func (w world) age(t *testing.T, digest, interval string) {
	t.Helper()
	_, err := w.env.Pool.Exec(context.Background(),
		`UPDATE "Blobs" SET "LastUsedAt" = now() - $2::interval WHERE "Digest" = decode($1, 'hex')`, digest, interval)
	require.NoError(t, err)
}

func TestTheSweepRemovesUnusedBlobsAfterTheGraceAndKeepsTheRest(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	g := w.next(t)
	used := w.env.SeedArtifact(w.project, "web", []byte("still referenced"), false)
	usedHex := apptest.DigestHex(used.Content)
	orphan, fresh := []byte("nobody points here"), []byte("uploaded a moment ago")
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "artifact", "web", orphan).Status)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "cache", "", fresh).Status)
	w.age(t, apptest.DigestHex(orphan), "7 hours")
	w.age(t, usedHex, "7 hours")

	report, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.True(t, report.Ran)
	assert.Equal(t, 1, report.Removed)
	_, err = os.Stat(w.blobFile(apptest.DigestHex(orphan)))
	assert.ErrorIs(t, err, os.ErrNotExist, "the unused blob is gone")
	for _, d := range []string{usedHex, apptest.DigestHex(fresh)} {
		_, err = os.Stat(w.blobFile(d))
		assert.NoError(t, err, "a referenced or recent blob stays")
	}

	_, err = w.env.Pool.Exec(context.Background(), `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '1 minute' WHERE "Id" = $1`, used.ID)
	require.NoError(t, err)
	report, err = w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, report.Expired)
	assert.Equal(t, 1, report.Removed, "once its artifact expired, the old blob goes in the same pass")
	res := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/artifacts/"+used.ID.String()+"/download"))
	assert.Equal(t, http.StatusNotFound, res.Status, "an expired artifact is not served")
}

func TestASweepThatRunsAgainDoesNothingNew(t *testing.T) {
	w := newWorld(t)
	first, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	second, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.Equal(t, first.Removed, second.Removed)
	assert.Zero(t, second.Removed)
}

func TestAMissingStoredFileIsGoneNotAnError(t *testing.T) {
	w := newWorld(t)
	a := w.env.SeedArtifact(w.project, "web", []byte("archive"), false)
	require.NoError(t, os.Remove(w.blobFile(apptest.DigestHex(a.Content))))
	res := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/artifacts/"+a.ID.String()+"/download"))
	assert.Equal(t, http.StatusGone, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "ARTIFACT_GONE", res.Error.Code)
}

func TestAFileLeftByACrashBetweenTheRenameAndTheCommitIsRemoved(t *testing.T) {
	w := newWorld(t)
	old, recent := []byte("renamed, then the server died"), []byte("renamed a moment ago")
	for _, data := range [][]byte{old, recent} {
		require.NoError(t, w.env.Wired.Artifacts.StoreWithoutRecord(context.Background(), data))
	}
	past := time.Now().Add(-7 * time.Hour)
	require.NoError(t, os.Chtimes(w.blobFile(apptest.DigestHex(old)), past, past))

	report, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, report.Orphans)
	_, err = os.Stat(w.blobFile(apptest.DigestHex(old)))
	assert.ErrorIs(t, err, os.ErrNotExist, "a file without a row is removed after the grace period")
	_, err = os.Stat(w.blobFile(apptest.DigestHex(recent)))
	assert.NoError(t, err, "a recent file may still be waiting for its commit")
}
