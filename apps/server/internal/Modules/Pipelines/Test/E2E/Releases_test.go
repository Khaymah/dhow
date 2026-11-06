package e2e_test

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	kindStarted  = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_RELEASE_STARTED
	kindUploaded = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_UPLOADED
	kindSwitched = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_SWITCHED
	kindHealth   = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_HEALTH
	kindVerified = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_VERIFIED
	kindRolled   = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLED_BACK
	kindRbFailed = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLBACK_FAILED
	kindPruned   = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_PRUNED
	kindBatch    = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_BATCH_PASSED
	kindFailed   = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_FAILED
	kindSkipped  = runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_SKIPPED
)

type releaseJob struct {
	deployWorld
	run     uuid.UUID
	grant   *apptest.Grant
	release string
}

func (d deployWorld) releasing(t *testing.T) releaseJob {
	t.Helper()
	run := d.built(t, deployPipeline)
	g := d.leaseDeploy(t)
	_, err := d.runner.Start(g)
	require.NoError(t, err)
	require.NoError(t, d.runner.BeginStep(g.Ref, 1, true))
	return releaseJob{deployWorld: d, run: run, grant: g, release: g.Grant.GetSpec().GetTargets()[0].GetReleaseId()}
}

func ev(kind runnerv1.DeployEventKind, host uuid.UUID) *runnerv1.DeployEvent {
	e := &runnerv1.DeployEvent{Kind: kind}
	if host != uuid.Nil {
		e.HostId = host.String()
	}
	return e
}

func (j releaseJob) hosts() []uuid.UUID {
	out := make([]uuid.UUID, 0, 3)
	for _, h := range j.target.Hosts {
		out = append(out, h.ID)
	}
	return out
}

func (j releaseJob) report(t *testing.T, events ...*runnerv1.DeployEvent) {
	t.Helper()
	require.NoError(t, j.runner.ReportRelease(j.grant.Ref, 1, events...))
}

func (j releaseJob) start(t *testing.T) {
	t.Helper()
	j.seedPrevious(t)
	j.report(t, &runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: j.release, PreviousReleaseId: "11-aaaaaaaa-0"})
}

func rejected(t *testing.T, err error) (connect.Code, string) {
	t.Helper()
	require.Error(t, err)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	return ce.Code(), ce.Message()
}

func (j releaseJob) refuse(t *testing.T, code connect.Code, message string, step int32, events ...*runnerv1.DeployEvent) {
	t.Helper()
	gotCode, gotMessage := rejected(t, j.runner.ReportRelease(j.grant.Ref, step, events...))
	assert.Equal(t, code, gotCode)
	assert.True(t, strings.HasPrefix(gotMessage, message), "%s should start with %s", gotMessage, message)
}

func (j releaseJob) releaseState(t *testing.T) string {
	t.Helper()
	var s string
	require.NoError(t, j.env.Pool.QueryRow(context.Background(), `SELECT "State" FROM "DeploymentReleases" WHERE "ReleaseId" = $1`, j.release).Scan(&s))
	return s
}

func (j releaseJob) hostStates(t *testing.T) map[uuid.UUID]string {
	t.Helper()
	rows, err := j.env.Pool.Query(context.Background(), `SELECT "HostId", "State" FROM "DeploymentReleaseHosts" WHERE "ReleaseId" = $1`, j.release)
	require.NoError(t, err)
	defer rows.Close()
	out := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var s string
		require.NoError(t, rows.Scan(&id, &s))
		out[id] = s
	}
	require.NoError(t, rows.Err())
	return out
}

func TestAReleaseGoesFromStartedToSucceededOnTheReportsOfTheDeployStep(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	hosts := j.hosts()
	j.start(t)
	j.start(t)
	assert.Equal(t, "running", j.releaseState(t))
	for _, h := range hosts {
		assert.Equal(t, "pending", j.hostStates(t)[h])
	}
	var deployment string
	require.NoError(t, j.env.Pool.QueryRow(context.Background(), `SELECT "State" FROM "Deployments" WHERE "RunId" = $1`, j.run).Scan(&deployment))
	assert.Equal(t, "running", deployment, "the first report moves the deployment to running")

	for i, h := range hosts {
		up := ev(kindUploaded, h)
		sw := ev(kindSwitched, h)
		sw.Batch, sw.PreviousReleaseId = int32(i+1), "11-aaaaaaaa-0"
		hl := ev(kindHealth, h)
		hl.Health = &runnerv1.DeployHealth{Attempts: 3, Passed: true, LastStatus: 200, DurationMs: 1500, StableChecks: 3}
		hl.Message = "ok\x00 with a control\x07 character"
		j.report(t, up, sw, hl, ev(kindVerified, h), &runnerv1.DeployEvent{Kind: kindBatch, Batch: int32(i + 1)})
	}
	j.report(t, ev(kindVerified, uuid.Nil), ev(kindPruned, uuid.Nil))
	assert.Equal(t, "succeeded", j.releaseState(t))
	for _, h := range hosts {
		assert.Equal(t, "verified", j.hostStates(t)[h])
	}
	var digestLen int
	var health, message string
	require.NoError(t, j.env.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(octet_length(r."ArtifactDigest"), 0), h."Health"::text, h."Message" FROM "DeploymentReleases" r
		 JOIN "DeploymentReleaseHosts" h ON h."ReleaseRowId" = r."Id" WHERE h."HostId" = $1`, hosts[0]).Scan(&digestLen, &health, &message))
	assert.Zero(t, digestLen, "the build job committed no artifact in this test, so no digest is known")
	assert.JSONEq(t, `{"Attempts":3,"Passed":true,"LastStatus":200,"LastError":"","DurationMs":1500,"StableChecks":3}`, health)
	assert.Equal(t, "ok with a control character", message)
	assert.Equal(t, 1, j.env.AuditCount("deployment.release_started"))
	assert.Equal(t, 3, j.env.AuditCount("deployment.release_switched"))
	assert.Equal(t, 1, j.env.AuditCount("deployment.release_succeeded"))
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_CONFLICT", 1, &runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: j.release})
}

func TestAFailureAfterTheFirstSwitchRollsTheWholeGroupBack(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	j.report(t, ev(kindUploaded, h[0]), ev(kindSwitched, h[0]), ev(kindVerified, h[0]), ev(kindUploaded, h[1]), ev(kindSwitched, h[1]))
	failed := ev(kindRolled, h[1])
	failed.Reason = "health_check_failed"
	j.report(t, ev(kindRolled, h[0]), failed)
	rest := ev(kindSkipped, h[2])
	j.report(t, rest)
	end := ev(kindRolled, uuid.Nil)
	end.Reason = "health_check_failed"
	j.report(t, end)
	assert.Equal(t, "rolled_back", j.releaseState(t))
	assert.Equal(t, map[uuid.UUID]string{h[0]: "rolled_back", h[1]: "rolled_back", h[2]: "skipped"}, j.hostStates(t))
	assert.Equal(t, 1, j.env.AuditCount("deployment.release_rolled_back"))
}

func TestARollbackThatFailsOnOneHostEndsAsRollbackFailedAndLeavesNothingUnsaid(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	j.report(t, ev(kindUploaded, h[0]), ev(kindSwitched, h[0]), ev(kindUploaded, h[1]), ev(kindSwitched, h[1]))
	j.report(t, ev(kindRolled, h[0]), ev(kindRbFailed, h[1]), ev(kindFailed, h[2]))
	j.report(t, ev(kindRbFailed, uuid.Nil))
	assert.Equal(t, "rollback_failed", j.releaseState(t))
	assert.Equal(t, 1, j.env.AuditCount("deployment.release_rollback_failed"))
	assert.Equal(t, "rollback_failed", j.hostStates(t)[h[1]])
}

func TestAReportOfAnotherLeaseAnOldFencingTokenOrAnotherStepIsRejected(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	h := j.hosts()
	stale := &runnerv1.LeaseRef{LeaseId: j.grant.Ref.GetLeaseId(), FencingToken: j.grant.Ref.GetFencingToken() - 1}
	code, msg := rejected(t, j.runner.ReportRelease(stale, 1, ev(kindUploaded, h[0])))
	assert.Equal(t, []string{connect.CodeFailedPrecondition.String(), "LEASE_LOST"}, []string{code.String(), msg})
	other := &runnerv1.LeaseRef{LeaseId: uuid.NewString(), FencingToken: j.grant.Ref.GetFencingToken()}
	code, msg = rejected(t, j.runner.ReportRelease(other, 1, ev(kindUploaded, h[0])))
	assert.Equal(t, []string{connect.CodeFailedPrecondition.String(), "LEASE_LOST"}, []string{code.String(), msg})
	j.refuse(t, connect.CodePermissionDenied, "NOT_A_TARGET_STEP", 0, ev(kindUploaded, h[0]))
	j.refuse(t, connect.CodePermissionDenied, "NOT_A_TARGET_STEP", 7, ev(kindUploaded, h[0]))

	_, err := j.env.Pool.Exec(context.Background(), `UPDATE "DeploymentReleases" SET "LeaseId" = $1 WHERE "ReleaseId" = $2`, uuid.New(), j.release)
	require.NoError(t, err)
	j.refuse(t, connect.CodePermissionDenied, "RELEASE_LEASE_MISMATCH", 1, ev(kindUploaded, h[0]))
	assert.Equal(t, "pending", j.hostStates(t)[h[0]], "a forged report changes nothing")
}

func TestAReportOfAFinishedLeaseIsRejected(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	j.env.ExpireLease(j.grant.Ref.GetLeaseId())
	j.refuse(t, connect.CodeFailedPrecondition, "LEASE_LOST", 1, ev(kindUploaded, j.hosts()[0]))
}

func TestOversizedOrMalformedFieldsAreRejectedAndNothingOfTheRequestIsKept(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	h := j.hosts()[0]
	long := ev(kindUploaded, h)
	long.Message = strings.Repeat("m", 5000)
	badHealth := ev(kindHealth, h)
	badHealth.Health = &runnerv1.DeployHealth{Attempts: 5000}
	badError := ev(kindHealth, h)
	badError.Health = &runnerv1.DeployHealth{LastError: "a free text error that leaks a secret"}
	badReason := ev(kindRolled, h)
	badReason.Reason = "because"
	badRelease := ev(kindUploaded, h)
	badRelease.ReleaseId = "../etc"
	otherRelease := ev(kindUploaded, h)
	otherRelease.ReleaseId = "1-deadbeef-0"
	badBatch := ev(kindSwitched, h)
	badBatch.Batch = 11
	badMode := &runnerv1.DeployEvent{Kind: kindStarted, Mode: "format-disk"}
	for name, e := range map[string]*runnerv1.DeployEvent{
		"message": long, "health": badHealth, "health error": badError, "reason": badReason, "release id": badRelease,
		"another release": otherRelease, "batch": badBatch, "mode": badMode, "kind": {Kind: runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_UNSPECIFIED},
	} {
		code, _ := rejected(t, j.runner.ReportRelease(j.grant.Ref, 1, ev(kindUploaded, h), e))
		assert.Equal(t, connect.CodeInvalidArgument, code, name)
	}
	many := make([]*runnerv1.DeployEvent, 101)
	for i := range many {
		many[i] = ev(kindPruned, uuid.Nil)
	}
	j.refuse(t, connect.CodeInvalidArgument, "VALIDATION_FAILED", 1, many...)
	j.refuse(t, connect.CodeInvalidArgument, "VALIDATION_FAILED", 1)
	assert.Equal(t, "pending", j.hostStates(t)[h], "the valid first event of a refused request is not kept either")
}

func TestAHostThatIsNotInTheTargetIsRejected(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	j.refuse(t, connect.CodePermissionDenied, "RELEASE_UNKNOWN_HOST", 1, ev(kindUploaded, uuid.New()))
	j.refuse(t, connect.CodePermissionDenied, "RELEASE_UNKNOWN_HOST", 1, &runnerv1.DeployEvent{Kind: kindUploaded, HostId: "not-an-id"})
	j.refuse(t, connect.CodePermissionDenied, "RELEASE_UNKNOWN_HOST", 1, ev(kindPruned, uuid.New()))
}

func TestIllegalTransitionsAreRejected(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_NOT_STARTED", 1, ev(kindUploaded, h[0]))
	j.start(t)
	for name, e := range map[string]*runnerv1.DeployEvent{
		"verified before it was switched": ev(kindVerified, h[0]), "switched before it was uploaded": ev(kindSwitched, h[0]),
		"rolled back before a switch": ev(kindRolled, h[0]), "health before a switch": ev(kindHealth, h[0]),
		"the group verified with hosts pending": ev(kindVerified, uuid.Nil), "the group rolled back with nothing switched": ev(kindRolled, uuid.Nil),
		"the group rollback failed with no failed host": ev(kindRbFailed, uuid.Nil),
	} {
		code, msg := rejected(t, j.runner.ReportRelease(j.grant.Ref, 1, e))
		assert.Equal(t, []string{"failed_precondition", "RELEASE_ILLEGAL_TRANSITION"}, []string{code.String(), msg}, name)
	}
	j.report(t, ev(kindUploaded, h[0]), ev(kindUploaded, h[0]), ev(kindSwitched, h[0]), ev(kindVerified, h[0]))
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_ILLEGAL_TRANSITION", 1, ev(kindUploaded, h[0]))
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_ILLEGAL_TRANSITION", 1, ev(kindFailed, uuid.Nil))
	j.report(t, ev(kindFailed, h[1]), ev(kindSkipped, h[2]))
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_ILLEGAL_TRANSITION", 1, ev(kindSwitched, h[1]))
	j.refuse(t, connect.CodeInvalidArgument, "VALIDATION_FAILED", 1, &runnerv1.DeployEvent{Kind: kindBatch, Batch: 4})
}

func TestNothingChangesAfterAReleaseFinishedExceptPruning(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	j.report(t, ev(kindFailed, h[0]), ev(kindSkipped, h[1]), ev(kindSkipped, h[2]))
	j.report(t, ev(kindFailed, uuid.Nil))
	assert.Equal(t, "failed", j.releaseState(t))
	j.refuse(t, connect.CodeFailedPrecondition, "RELEASE_FINISHED", 1, ev(kindUploaded, h[0]))
	j.report(t, ev(kindPruned, uuid.Nil))
}

func TestAKilledRunnerLeavesTheRunningReleaseUnknown(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	j.report(t, ev(kindUploaded, h[0]), ev(kindSwitched, h[0]), ev(kindUploaded, h[1]))
	assert.Equal(t, "running", j.releaseState(t))

	j.env.ExpireLease(j.grant.Ref.GetLeaseId())
	j.env.Reconcile()
	j.env.StartRuns()
	j.sweepReleases(t)

	assert.Equal(t, "unknown", j.releaseState(t))
	assert.Equal(t, map[uuid.UUID]string{h[0]: "unknown", h[1]: "unknown", h[2]: "pending"}, j.hostStates(t))
	assert.Equal(t, 1, j.env.AuditCount("deployment.release_unknown"))
	assert.Zero(t, j.runningReleases(t), "no release of the target is left running")
	for _, job := range j.view(t, j.run).Jobs {
		if job.Key == "deploy" {
			require.NotNil(t, job.FailureReason)
			assert.Equal(t, "DEPLOY_INTERRUPTED", *job.FailureReason, "a deploy cut off after its mutating step began is not a plain lost job")
		}
	}
	j.refuse(t, connect.CodeFailedPrecondition, "LEASE_LOST", 1, ev(kindVerified, h[0]))
}

func TestAJobThatEndsWithoutAFinalReportLeavesItsReleaseUnknown(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	h := j.hosts()
	j.start(t)
	j.report(t, ev(kindUploaded, h[0]))
	require.NoError(t, j.runner.EndStep(j.grant.Ref, 1, 0))
	require.NoError(t, j.runner.Complete(j.grant, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED))
	j.sweepReleases(t)
	assert.Equal(t, "unknown", j.releaseState(t), "nobody confirmed the end, so the log does not claim one")
	assert.Equal(t, "unknown", j.hostStates(t)[h[0]])
}

func TestANewReleaseCanStartOnlyAfterTheOldRunningOneIsGone(t *testing.T) {
	j := newDeployWorld(t).releasing(t)
	j.start(t)
	var target uuid.UUID
	require.NoError(t, j.env.Pool.QueryRow(context.Background(), `SELECT "TargetId" FROM "DeploymentReleases" WHERE "ReleaseId" = $1`, j.release).Scan(&target))
	_, err := j.env.Pool.Exec(context.Background(), `INSERT INTO "DeploymentReleases" ("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "ReleaseId", "State")
		SELECT "OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", 'dup-1', 'running' FROM "DeploymentReleases" WHERE "ReleaseId" = $1`, j.release)
	require.Error(t, err, "the database allows one running release per target")
}
