package e2e_test

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (j releaseJob) seedPrevious(t *testing.T) {
	t.Helper()
	_, err := j.env.Pool.Exec(context.Background(), `INSERT INTO "DeploymentReleases"
		("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "ReleaseId", "State", "FinishedAt")
		SELECT d."OrganizationId", d."Id", (SELECT rj."Id" FROM "RunJobs" rj WHERE rj."RunId" = $2 AND rj."Key" = 'build'), d."JobId", gen_random_uuid(), $1, 1, '11-aaaaaaaa-0', 'succeeded', now()
		FROM "Deployments" d WHERE d."RunId" = $2 ON CONFLICT DO NOTHING`, j.target.ID, j.run)
	require.NoError(t, err)
}

func (j releaseJob) orphan(t *testing.T) {
	t.Helper()
	_, err := j.env.Pool.Exec(context.Background(), `UPDATE "DeploymentReleases" SET "LeaseId" = $1 WHERE "ReleaseId" = $2`, uuid.New(), j.release)
	require.NoError(t, err)
}

func TestARetriedJobAdoptsTheUnknownReleaseOfItsLostAttemptAndDeploys(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	j.report(t, ev(kindUploaded, h[0]), ev(kindSwitched, h[0]))
	j.orphan(t)
	j.sweepReleases(t)
	require.Equal(t, "unknown", j.releaseState(t))
	require.Equal(t, "unknown", j.hostStates(t)[h[0]])

	j.start(t)
	assert.Equal(t, "running", j.releaseState(t), "the new attempt took the row over")
	for _, id := range h {
		assert.Equal(t, "pending", j.hostStates(t)[id], "the hosts start again from pending")
	}
	var lease uuid.UUID
	require.NoError(t, j.env.Pool.QueryRow(context.Background(), `SELECT "LeaseId" FROM "DeploymentReleases" WHERE "ReleaseId" = $1`, j.release).Scan(&lease))
	assert.Equal(t, j.grant.Ref.GetLeaseId(), lease.String())
	for _, id := range h {
		j.report(t, ev(kindUploaded, id), ev(kindSwitched, id), ev(kindVerified, id))
	}
	j.report(t, ev(kindVerified, uuid.Nil))
	assert.Equal(t, "succeeded", j.releaseState(t))
}

func TestARetryCannotTakeOverTheReleaseOfAnotherJobOrOneThatEndedForGood(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	j.orphan(t)
	j.sweepReleases(t)
	_, err := j.env.Pool.Exec(context.Background(), `UPDATE "DeploymentReleases" SET "JobId" = $1 WHERE "ReleaseId" = $2`, uuid.New(), j.release)
	require.NoError(t, err)
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_CONFLICT", 1, &runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: j.release})

	k := newDeployWorld(t).releasing(t)
	k.start(t)
	k.report(t, ev(kindFailed, uuid.Nil))
	k.orphan(t)
	k.refuse(t, connect.CodeFailedPrecondition, "RELEASE_CONFLICT", 1, &runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: k.release})
}

func TestAPreviousReleaseMustBeAKnownReleaseOfTheSameTarget(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.refuse(t, connect.CodeInvalidArgument, "VALIDATION_FAILED: PreviousReleaseId", 1,
		&runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: j.release, PreviousReleaseId: "QyNTUxOQAAACD3d5eD0W6rQmJXm5nQ0m5rWQ7s5oX6lWQ0v2q2QJ0m3"})
	j.start(t)
	smuggle := ev(kindUploaded, h[0])
	smuggle.PreviousReleaseId = "b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW"
	j.refuse(t, connect.CodeInvalidArgument, "VALIDATION_FAILED: PreviousReleaseId", 1, smuggle)
	self := ev(kindUploaded, h[0])
	self.PreviousReleaseId = j.release
	j.refuse(t, connect.CodeInvalidArgument, "VALIDATION_FAILED: PreviousReleaseId", 1, self)
	known := ev(kindUploaded, h[0])
	known.PreviousReleaseId = "11-aaaaaaaa-0"
	j.report(t, known)
}

func TestAKeyChunkInAMessageIsStoredMasked(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	chunk := "b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW"
	m := ev(kindFailed, h[0])
	m.Message = "failed with " + chunk + " and " + "-----" + "BEGIN OPENSSH PRIVATE KEY" + "-----"
	j.report(t, m)
	end := ev(kindFailed, uuid.Nil)
	end.Message = chunk
	j.report(t, end)
	var host, group string
	require.NoError(t, j.env.Pool.QueryRow(context.Background(),
		`SELECT h."Message", r."Message" FROM "DeploymentReleaseHosts" h JOIN "DeploymentReleases" r ON r."Id" = h."ReleaseRowId" WHERE h."HostId" = $1`, h[0]).Scan(&host, &group))
	for _, stored := range []string{host, group} {
		assert.NotContains(t, stored, chunk[:20])
		assert.NotContains(t, stored, "BEGIN OPENSSH")
		assert.Contains(t, stored, "***")
	}
	assert.True(t, strings.HasPrefix(host, "failed with "))
}

func TestASucceededReleaseOfAJobThatFailedBecomesUnknown(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	for _, id := range h {
		j.report(t, ev(kindUploaded, id), ev(kindSwitched, id), ev(kindVerified, id))
	}
	j.report(t, ev(kindVerified, uuid.Nil))
	require.Equal(t, "succeeded", j.releaseState(t))
	require.NoError(t, j.runner.EndStep(j.grant.Ref, 1, 1))
	require.NoError(t, j.runner.Complete(j.grant, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, 1, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED))
	j.sweepReleases(t)
	assert.Equal(t, "unknown", j.releaseState(t), "the step exited with an error, so the report is not trusted as a success")
	assert.Equal(t, 1, j.env.AuditCount("deployment.release_unknown"))
	for _, id := range h {
		assert.Equal(t, "verified", j.hostStates(t)[id], "host states stay as the plugin reported them")
	}
}
