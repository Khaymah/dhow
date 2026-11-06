package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

func (s shipped) artifactKept(t *testing.T) bool {
	t.Helper()
	var gone bool
	require.NoError(t, s.env.Pool.QueryRow(context.Background(),
		`SELECT "IsDeleted" FROM "Artifacts" WHERE "Digest" = $1`, s.artifact.Digest[:]).Scan(&gone))
	return !gone
}

func TestTheArtifactOfAReleaseThatCanStillBeRolledBackToOutlivesItsExpiryUntilItLeavesTheKeptReleases(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	s.sql(t, `UPDATE "DeployTargets" SET "KeepReleases" = 2 WHERE "Id" = $1`, s.target.ID)
	s.sql(t, `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '1 day' WHERE "Digest" = $1`, s.artifact.Digest[:])

	_, err := s.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.True(t, s.artifactKept(t), "the earlier release is one of the two kept, so its artifact stays although it expired")

	s.sql(t, `INSERT INTO "DeploymentReleases"
		("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "ReleaseId", "State", "FinishedAt", "StartedAt")
		SELECT r."OrganizationId", r."DeploymentId", r."RunJobId", r."JobId", gen_random_uuid(), r."TargetId", 4, '30-eeeeeeee-0', 'succeeded', now(), now() + interval '1 hour'
		FROM "DeploymentReleases" r WHERE r."ReleaseId" = $1`, s.current)
	_, err = s.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.False(t, s.artifactKept(t), "a newer release pushed it out of the kept ones, so the normal expiry applies")
}

func TestAReleaseThatNeverSucceededDoesNotKeepItsArtifactPastItsExpiry(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	s.sql(t, `UPDATE "DeploymentReleases" SET "State" = 'rolled_back' WHERE "ReleaseId" = $1`, earlier)
	s.sql(t, `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '1 day' WHERE "Digest" = $1`, s.artifact.Digest[:])
	_, err := s.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.False(t, s.artifactKept(t))
}

func TestTheArtifactOfAnotherProjectWithTheSameDigestIsNotKeptForThisTarget(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	other := s.env.SeedProject(s.org, s.ownerID)
	foreign := s.env.SeedArtifact(other, "web", s.artifact.Content, false)
	s.sql(t, `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '1 day' WHERE "Digest" = $1`, s.artifact.Digest[:])

	_, err := s.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)

	var mine, theirs bool
	require.NoError(t, s.env.Pool.QueryRow(context.Background(), `SELECT "IsDeleted" FROM "Artifacts" WHERE "Id" = $1`, s.artifact.ID).Scan(&mine))
	require.NoError(t, s.env.Pool.QueryRow(context.Background(), `SELECT "IsDeleted" FROM "Artifacts" WHERE "Id" = $1`, foreign.ID).Scan(&theirs))
	assert.False(t, mine, "the artifact of the project of the target is kept")
	assert.True(t, theirs, "the artifact of another project with the same digest is not")
}

func TestAnArtifactIsKeptForARollbackAtMostNinetyDaysPastItsExpiry(t *testing.T) {
	require.Equal(t, 90*24*time.Hour, config.RollbackArtifactMaxKeep)
	s := newDeployWorld(t).shipped(t)
	s.sql(t, `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '89 days' WHERE "Digest" = $1`, s.artifact.Digest[:])
	_, err := s.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.True(t, s.artifactKept(t), "89 days past its expiry it is still kept")

	s.sql(t, `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '91 days' WHERE "Digest" = $1`, s.artifact.Digest[:])
	_, err = s.env.Wired.Artifacts.Sweep(context.Background())
	require.NoError(t, err)
	assert.False(t, s.artifactKept(t), "91 days past its expiry it goes although the release is still one of the kept ones")
}

func TestAnArtifactKeptForARollbackCountsAgainstTheQuotaOfTheOrganization(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	bytesOf := func() int64 {
		n, err := sqlc.New(s.env.Pool).OrganizationArtifactBytes(context.Background(), sqlc.OrganizationArtifactBytesParams{
			OrganizationID: s.org, RollbackGraceSeconds: config.RollbackArtifactMaxKeep.Seconds(),
		})
		require.NoError(t, err)
		return n
	}
	size := int64(len(s.artifact.Content))
	assert.Equal(t, size, bytesOf())
	s.sql(t, `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '1 day' WHERE "Digest" = $1`, s.artifact.Digest[:])
	assert.Equal(t, size, bytesOf(), "expired but kept for a rollback, so it still counts")
	s.sql(t, `UPDATE "Artifacts" SET "ExpiresAt" = now() - interval '91 days' WHERE "Digest" = $1`, s.artifact.Digest[:])
	assert.Zero(t, bytesOf(), "past the cap it no longer counts")
}
