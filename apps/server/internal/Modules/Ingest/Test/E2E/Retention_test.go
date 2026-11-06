package e2e_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func (w world) ingestedRow(t *testing.T, digest string) (deleted bool, pinned *time.Time) {
	t.Helper()
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "IsDeleted", "PinnedUntil" FROM "IngestedArtifacts" WHERE "ProjectId" = $1 AND "Digest" = decode($2, 'hex')
		 ORDER BY "CreatedAt" DESC LIMIT 1`, w.project, digest).Scan(&deleted, &pinned))
	return deleted, pinned
}

func (w world) pastRetention(t *testing.T, digest string) {
	t.Helper()
	_, err := w.env.Pool.Exec(context.Background(),
		`UPDATE "IngestedArtifacts" SET "ExpiresAt" = now() - interval '1 hour' WHERE "ProjectId" = $1 AND "Digest" = decode($2, 'hex')`,
		w.project, digest)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(context.Background(),
		`UPDATE "Blobs" SET "LastUsedAt" = now() - interval '7 hours' WHERE "Digest" = decode($1, 'hex')`, digest)
	require.NoError(t, err)
}

func (w world) uploaded(t *testing.T, content map[string]string) string {
	t.Helper()
	body := apptest.TarGz(content)
	res := w.upload(body, func(u *apptest.IngestUpload) { u.NoTrigger = true; u.Sha = shaTag })
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	return apptest.DigestHex(body)
}

func TestAnIngestedArtifactPastRetentionAndWithoutAReleaseIsRemovedWithItsBlob(t *testing.T) {
	w := newWorld(t)
	digest := w.uploaded(t, map[string]string{"a.txt": "nobody deploys this"})
	require.True(t, w.env.BlobOnDisk(digest))
	w.pastRetention(t, digest)

	report, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, report.Expired)
	deleted, pinned := w.ingestedRow(t, digest)
	assert.True(t, deleted)
	assert.Nil(t, pinned)
	assert.False(t, w.env.BlobOnDisk(digest), "the blob goes in the same pass")
}

func TestAnIngestedArtifactPastRetentionButPinnedToAReleaseIsStillThere(t *testing.T) {
	w := newWorld(t)
	digest := w.uploaded(t, map[string]string{"a.txt": "deployed to production"})
	target, _ := w.env.SeedDeployTarget(w.project, "production", "prod")
	w.env.SeedSucceededRelease(w.project, target, digest, "1-aaaaaaaa-1", time.Hour)
	w.pastRetention(t, digest)

	report, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, report.Pinned)
	assert.Zero(t, report.Expired, "retention does not take an artifact a kept release uses")
	deleted, pinned := w.ingestedRow(t, digest)
	assert.False(t, deleted)
	require.NotNil(t, pinned)
	assert.WithinDuration(t, time.Now().Add(config.IngestPinMax), *pinned, time.Minute)
	assert.True(t, w.env.BlobOnDisk(digest), "the blob stays, so a rollback needs no rebuild")

	again, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.Zero(t, again.Expired)
	assert.True(t, w.env.BlobOnDisk(digest))
}

func TestThePinEndsWhenTheReleaseLeavesKeepReleases(t *testing.T) {
	w := newWorld(t)
	digest := w.uploaded(t, map[string]string{"a.txt": "an old release"})
	target, _ := w.env.SeedDeployTarget(w.project, "production", "prod")
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "DeployTargets" SET "KeepReleases" = 2 WHERE "Id" = $1`, target)
	require.NoError(t, err)
	w.env.SeedSucceededRelease(w.project, target, digest, "1-aaaaaaaa-1", 3*time.Hour)
	w.pastRetention(t, digest)
	_, err = w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	deleted, pinned := w.ingestedRow(t, digest)
	require.False(t, deleted)
	require.NotNil(t, pinned)

	w.env.SeedSucceededRelease(w.project, target, apptest.DigestHex([]byte("newer one")), "2-bbbbbbbb-1", 2*time.Hour)
	w.env.SeedSucceededRelease(w.project, target, apptest.DigestHex([]byte("newest")), "3-cccccccc-1", time.Hour)
	report, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, report.Expired, "two newer releases fill keep_releases")
	deleted, _ = w.ingestedRow(t, digest)
	assert.True(t, deleted)
}

func TestAPinOnlyCoversTheProjectOfTheTarget(t *testing.T) {
	w := newWorld(t)
	digest := w.uploaded(t, map[string]string{"a.txt": "same bytes, another project"})
	other := w.env.SeedProject(w.org, w.ownerID)
	target, _ := w.env.SeedDeployTarget(other, "production", "prod-other")
	w.env.SeedSucceededRelease(other, target, digest, "1-aaaaaaaa-1", time.Hour)
	w.pastRetention(t, digest)

	report, err := w.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.Zero(t, report.Pinned, "a release of another project keeps nothing of this one")
	assert.EqualValues(t, 1, report.Expired)
}
