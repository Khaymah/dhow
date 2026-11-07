package e2e_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type auditedRelease struct {
	Actor   *uuid.UUID
	Payload map[string]string
}

func (j releaseJob) auditOf(t *testing.T, action string) auditedRelease {
	t.Helper()
	var out auditedRelease
	var raw []byte
	require.NoError(t, j.env.Pool.QueryRow(context.Background(),
		`SELECT "ActorUserId", "Data" FROM "AuditEvents" WHERE "Action" = $1 ORDER BY "CreatedAt" DESC LIMIT 1`, action).Scan(&out.Actor, &raw))
	require.NoError(t, json.Unmarshal(raw, &out.Payload))
	return out
}

func TestTheReleaseAuditEventsCarryTheDigestTheRunnerAndWhoStartedTheRun(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	digest := make([]byte, 32)
	for i := range digest {
		digest[i] = byte(i + 1)
	}
	j.sql(t, `UPDATE "DeploymentReleases" SET "ArtifactDigest" = $1 WHERE "ReleaseId" = $2`, digest, j.release)
	var wantActor *uuid.UUID
	var wantGit *string
	var wantRunner uuid.UUID
	require.NoError(t, j.env.Pool.QueryRow(context.Background(),
		`SELECT r."TriggeredByUserId", r."Actor", l."RunnerId" FROM "PipelineRuns" r, "Leases" l WHERE r."Id" = $1 AND l."Id" = $2`,
		j.run, j.grant.Ref.GetLeaseId()).Scan(&wantActor, &wantGit, &wantRunner))
	for i, h := range j.hosts() {
		sw := ev(kindSwitched, h)
		sw.Batch = int32(i + 1)
		j.report(t, ev(kindUploaded, h), sw, ev(kindVerified, h))
	}
	j.report(t, ev(kindVerified, uuid.Nil))

	for _, action := range []string{"deployment.release_switched", "deployment.release_succeeded"} {
		got := j.auditOf(t, action)
		assert.Equal(t, hex.EncodeToString(digest), got.Payload["Digest"], action)
		assert.Equal(t, wantRunner.String(), got.Payload["RunnerId"], action)
		assert.Equal(t, wantActor, got.Actor, action)
		if wantGit != nil {
			assert.Equal(t, *wantGit, got.Payload["Actor"], action)
		}
		assert.Equal(t, j.release, got.Payload["ReleaseId"], action)
		assert.NotEmpty(t, got.Payload["TargetId"])
		assert.NotEmpty(t, got.Payload["DeploymentId"])
	}
	assert.True(t, wantActor != nil || wantGit != nil, "the run records who started it, so the event can name them")
}
