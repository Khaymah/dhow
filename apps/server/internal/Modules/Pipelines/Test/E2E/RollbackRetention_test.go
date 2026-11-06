package e2e_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
