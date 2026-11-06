package e2e_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const earlier = "11-aaaaaaaa-0"

type shipped struct {
	deployWorld
	rollRunner *apptest.TestRunner
	current    string
	artifact   apptest.SeededArtifact
}

func (d deployWorld) rollbackPath(suffix string) string {
	return apptest.OrgPath(d.org, "/projects/"+d.project.String()+"/environments/production/targets/web"+suffix)
}

func (d deployWorld) askForRollback(c *apptest.Client, release string, factor map[string]any) apptest.Response {
	body := map[string]any{"ReleaseId": release}
	if factor != nil {
		body["CurrentFactor"] = factor
	}
	return c.Do(apptest.Request{Method: http.MethodPost, Path: d.rollbackPath("/rollback"), Body: body})
}

func (d deployWorld) shipped(t *testing.T) shipped {
	t.Helper()
	j := d.releasing(t)
	art := d.env.SeedArtifact(d.project, "web", []byte("the archive of the earlier release"), false)
	d.sql(t, `INSERT INTO "PipelineRunCounters" ("ProjectId", "Next")
		SELECT "ProjectId", max("Number") + 1 FROM "PipelineRuns" WHERE "ProjectId" = $1 GROUP BY "ProjectId"
		ON CONFLICT ("ProjectId") DO UPDATE SET "Next" = GREATEST("PipelineRunCounters"."Next", EXCLUDED."Next")`, d.project)
	j.start(t)
	for i, host := range j.hosts() {
		sw := ev(kindSwitched, host)
		sw.Batch = int32(i + 1)
		j.report(t, ev(kindUploaded, host), sw, ev(kindVerified, host))
	}
	j.report(t, ev(kindVerified, uuid.Nil))
	d.sql(t, `UPDATE "DeploymentReleases" SET "ArtifactDigest" = $1, "StartedAt" = now() - interval '2 days',
		"HealthInputs" = '{"health_check_url": "https://{host}/healthz"}' WHERE "ReleaseId" = $2`, art.Digest[:], earlier)
	require.NoError(t, d.runner.Complete(j.grant, succeeded, 0, runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED))
	rr := d.env.SeedConnectedRunner(d.org, d.ownerID, apptest.SeedRunnerSpec{
		Capacity: 4, Features: []string{"checkout", "plugins", "artifacts", "deploy_targets"},
	})
	return shipped{deployWorld: d, rollRunner: rr, current: j.release, artifact: art}
}

type rollbackJob struct {
	shipped
	run   uuid.UUID
	grant *apptest.Grant
}

func (s shipped) rollingBack(t *testing.T, release string) rollbackJob {
	t.Helper()
	res := s.askForRollback(s.owner, release, apptest.PasswordFactor(s.pass))
	require.Equal(t, http.StatusAccepted, res.Status, string(res.Raw))
	var started struct{ RunId uuid.UUID }
	res.Decode(t, &started)
	s.env.StartRuns()
	g, err := s.rollRunner.Lease(context.Background(), 1, 0)
	require.NoError(t, err)
	require.NotNil(t, g, "the rollback run queued a job for the deploy runner: %+v", s.env.RunJobs(started.RunId))
	require.Equal(t, "rollback", s.env.RunJobKey(uuid.MustParse(g.Grant.GetJobId())))
	_, err = s.rollRunner.Start(g)
	require.NoError(t, err)
	require.NoError(t, s.rollRunner.BeginStep(g.Ref, 0, true))
	return rollbackJob{shipped: s, run: started.RunId, grant: g}
}

func (r rollbackJob) hosts() []uuid.UUID {
	out := make([]uuid.UUID, 0, 3)
	for _, h := range r.target.Hosts {
		out = append(out, h.ID)
	}
	return out
}

func (r rollbackJob) report(t *testing.T, events ...*runnerv1.DeployEvent) {
	t.Helper()
	require.NoError(t, r.rollRunner.ReportRelease(r.grant.Ref, 0, events...))
}

func (r rollbackJob) release() string {
	return r.grant.Grant.GetSpec().GetTargets()[0].GetReleaseId()
}

func (r rollbackJob) rows(t *testing.T) (kind, state, restored string) {
	t.Helper()
	var rest *string
	require.NoError(t, r.env.Pool.QueryRow(context.Background(),
		`SELECT "Kind", "State", "RestoredReleaseId" FROM "DeploymentReleases" WHERE "ReleaseId" = $1`, r.release()).Scan(&kind, &state, &rest))
	if rest != nil {
		restored = *rest
	}
	return kind, state, restored
}

func (r rollbackJob) switchAll(t *testing.T) {
	t.Helper()
	r.report(t, &runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: r.release(), PreviousReleaseId: r.current, Mode: "rollback"})
	for i, host := range r.hosts() {
		sw := ev(kindSwitched, host)
		sw.Batch = int32(i + 1)
		r.report(t, ev(kindUploaded, host), sw, ev(kindVerified, host))
	}
}

func TestAManualRollbackStartsASyntheticRunWithTheEarlierReleaseTheStoredHealthCheckAndThePinnedArtifact(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	r := s.rollingBack(t, earlier)

	spec := r.grant.Grant.GetSpec()
	require.Len(t, spec.GetSteps(), 1)
	step := spec.GetSteps()[0]
	assert.Equal(t, sshPlugin, step.GetPlugin().GetImage())
	inputs := step.GetPlugin().GetInputs()
	assert.Equal(t, "rollback", inputs["mode"])
	assert.Equal(t, earlier, inputs["release"])
	assert.Equal(t, "https://{host}/healthz", inputs["health_check_url"], "the health check of the last deploy is kept")
	assert.Equal(t, "web", inputs["artifact"])
	require.Len(t, spec.GetInputs(), 1)
	assert.Equal(t, hex.EncodeToString(s.artifact.Digest[:]), spec.GetInputs()[0].GetDigest())
	assert.NotEmpty(t, spec.GetInputs()[0].GetDownloadToken())
	assert.True(t, step.GetMutating())
	require.Len(t, spec.GetTargets(), 1)
	assert.Len(t, spec.GetTargets()[0].GetHosts(), 3, "all the hosts of the target")
	assert.Nil(t, spec.GetCheckout())
	assert.Equal(t, "rollback", r.view(t, r.run).Run.Trigger)
	assert.Equal(t, 1, r.env.AuditCount("deployment.rollback_requested"))
	var deployments int
	require.NoError(t, r.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Deployments" WHERE "RunId" = $1`, r.run).Scan(&deployments))
	assert.Equal(t, 1, deployments, "the run went through the gate like any deployment")
}

func TestAManualRollbackThatSucceedsIsRecordedAsARollbackOfTheEarlierReleaseAndIsTheNewCurrentOne(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	r := s.rollingBack(t, earlier)
	r.switchAll(t)
	r.report(t, ev(kindVerified, uuid.Nil))

	kind, state, restored := r.rows(t)
	assert.Equal(t, "rollback", kind)
	assert.Equal(t, "succeeded", state)
	assert.Equal(t, earlier, restored)
	var digest []byte
	require.NoError(t, r.env.Pool.QueryRow(context.Background(),
		`SELECT "ArtifactDigest" FROM "DeploymentReleases" WHERE "ReleaseId" = $1`, r.release()).Scan(&digest))
	assert.Equal(t, s.artifact.Digest[:], digest)

	res := r.owner.Get(r.rollbackPath("/releases"))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var data struct {
		CurrentReleaseId string
		Partial          bool
		Releases         []struct {
			ReleaseId, Kind, State, RestoredReleaseId string
			RollbackTarget, ArtifactKept              bool
			Hosts                                     []struct{ State string }
		}
	}
	res.Decode(t, &data)
	assert.Equal(t, earlier, data.CurrentReleaseId, "the group is back on the earlier release")
	assert.False(t, data.Partial)
	require.Len(t, data.Releases, 3)
	assert.Equal(t, "rollback", data.Releases[0].Kind)
	assert.False(t, data.Releases[0].RollbackTarget, "a rollback is not a release to roll back to")
	assert.Equal(t, earlier, data.Releases[0].RestoredReleaseId)
	assert.Len(t, data.Releases[0].Hosts, 3)
	byID := map[string]int{}
	for i, rel := range data.Releases {
		byID[rel.ReleaseId] = i
	}
	assert.True(t, data.Releases[byID[earlier]].RollbackTarget)
	assert.True(t, data.Releases[byID[earlier]].ArtifactKept)
	assert.Equal(t, 1, r.env.AuditCount("deployment.release_succeeded")-1)
}

func TestAManualRollbackThatFailsOnAServerLeavesTheGroupRollbackFailedAndCannotBeRunAgain(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	r := s.rollingBack(t, earlier)
	h := r.hosts()
	failedHost := ev(kindRbFailed, h[1])
	failedHost.Reason = "health_check_failed"
	r.report(t, &runnerv1.DeployEvent{Kind: kindStarted, ReleaseId: r.release(), PreviousReleaseId: s.current, Mode: "rollback"},
		ev(kindUploaded, h[0]), ev(kindSwitched, h[0]), ev(kindUploaded, h[1]), ev(kindSwitched, h[1]), failedHost,
		ev(kindUploaded, h[2]), ev(kindSkipped, h[2]))
	group := ev(kindRbFailed, uuid.Nil)
	group.Reason = "health_check_failed"
	r.report(t, group)
	require.NoError(t, r.rollRunner.EndStep(r.grant.Ref, 0, 12))
	require.NoError(t, r.rollRunner.Complete(r.grant, failed, 12, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_ROLLBACK_FAILED))

	kind, state, _ := r.rows(t)
	assert.Equal(t, "rollback", kind)
	assert.Equal(t, "rollback_failed", state)
	states := hostStatesOf(t, r.deployWorld, r.release())
	assert.Equal(t, "unknown", states[h[0]], "a server that switched and was not confirmed is not said to be fine")
	assert.Equal(t, "rollback_failed", states[h[1]])
	assert.Equal(t, "skipped", states[h[2]])
	assert.Equal(t, 1, r.env.AuditCount("deployment.release_rollback_failed"))

	res := r.post(t, r.owner, r.run, "rerun-failed")
	assert.Equal(t, http.StatusConflict, res.Status, string(res.Raw))
	assert.Contains(t, string(res.Raw), "ROLLBACK_RUN_NOT_RERUNNABLE")

	listed := r.owner.Get(r.rollbackPath("/releases"))
	var data struct {
		CurrentReleaseId string
		Partial          bool
	}
	listed.Decode(t, &data)
	assert.True(t, data.Partial, "the servers are on different releases")
	assert.Equal(t, s.current, data.CurrentReleaseId, "the last good release is still the one of the deploy")
}

func TestAManualRollbackSkipsTheReviewersAndTheWaitOfTheEnvironment(t *testing.T) {
	d := newDeployWorld(t)
	s := d.shipped(t)
	d.reviewer(t, d.prod, "reviewer@example.com", "developer")
	d.sql(t, `UPDATE "Environments" SET "WaitMinutes" = 30, "RuleVersion" = "RuleVersion" + 1 WHERE "Id" = $1`, d.prod)

	r := s.rollingBack(t, earlier)
	assert.NotNil(t, r.grant, "the job was leased at once")
	assert.Zero(t, len(d.approvals(t, r.run)), "no approval was asked for")
	gate, _ := d.gate(t, r.run, "rollback")
	assert.Empty(t, gate)
}

func TestAManualRollbackIsRefusedForAReleaseThatNeverSucceededOnTheTarget(t *testing.T) {
	d := newDeployWorld(t)
	s := d.shipped(t)
	d.sql(t, `INSERT INTO "DeploymentReleases"
		("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "ReleaseId", "State", "FinishedAt")
		SELECT r."OrganizationId", r."DeploymentId", r."RunJobId", r."JobId", gen_random_uuid(), r."TargetId", 2, '12-bbbbbbbb-0', 'failed', now()
		FROM "DeploymentReleases" r WHERE r."ReleaseId" = $1`, earlier)
	d.sql(t, `INSERT INTO "DeploymentReleases"
		("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "ReleaseId", "State", "Kind", "RestoredReleaseId", "FinishedAt")
		SELECT r."OrganizationId", r."DeploymentId", r."RunJobId", r."JobId", gen_random_uuid(), r."TargetId", 3, '13-cccccccc-0', 'succeeded', 'rollback', $2, now()
		FROM "DeploymentReleases" r WHERE r."ReleaseId" = $1`, earlier, earlier)
	for _, release := range []string{"12-bbbbbbbb-0", "13-cccccccc-0", "99-nothing-0"} {
		res := d.askForRollback(s.owner, release, apptest.PasswordFactor(s.pass))
		assert.Equal(t, http.StatusConflict, res.Status, "%s: %s", release, string(res.Raw))
		assert.Contains(t, string(res.Raw), "ROLLBACK_RELEASE_NOT_ELIGIBLE", release)
	}
	assert.Equal(t, http.StatusNotFound, d.askForRollback(s.owner, "../etc", apptest.PasswordFactor(s.pass)).Status)
	var runs int
	require.NoError(t, d.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "PipelineRuns" WHERE "Trigger" = 'rollback'`).Scan(&runs))
	assert.Zero(t, runs, "a refused rollback creates no run")
	assert.Zero(t, d.env.AuditCount("deployment.rollback_requested"))
}

func TestAManualRollbackIsRefusedWhileTheEnvironmentIsPaused(t *testing.T) {
	d := newDeployWorld(t)
	s := d.shipped(t)
	d.sql(t, `UPDATE "Environments" SET "PausedAt" = now(), "PausedByUserId" = $2, "PausedReason" = 'freeze' WHERE "Id" = $1`, d.prod, d.ownerID)
	res := d.askForRollback(s.owner, earlier, apptest.PasswordFactor(s.pass))
	assert.Equal(t, http.StatusConflict, res.Status, string(res.Raw))
	assert.Contains(t, string(res.Raw), "ENVIRONMENT_PAUSED")
	d.sql(t, `UPDATE "Environments" SET "PausedAt" = NULL, "PausedByUserId" = NULL, "PausedReason" = NULL WHERE "Id" = $1`, d.prod)
	assert.Equal(t, http.StatusAccepted, d.askForRollback(s.owner, earlier, apptest.PasswordFactor(s.pass)).Status)
}

func TestAManualRollbackNeedsAMaintainerAndASecondFactor(t *testing.T) {
	d := newDeployWorld(t)
	s := d.shipped(t)
	for _, role := range []string{"developer", "viewer"} {
		c, _ := d.env.SignedIn(d.org, role+"@example.com", role)
		res := d.askForRollback(c, earlier, apptest.PasswordFactor("whatever"))
		assert.Equal(t, http.StatusForbidden, res.Status, role)
	}
	assert.Equal(t, http.StatusForbidden, d.askForRollback(s.owner, earlier, nil).Status, "no factor at all")
	assert.Equal(t, http.StatusForbidden, d.askForRollback(s.owner, earlier, apptest.PasswordFactor("not the password at all")).Status)

	enrolled := s.owner.EnableTotp(apptest.PasswordFactor(d.pass))
	res := d.askForRollback(s.owner, earlier, apptest.PasswordFactor(d.pass))
	assert.Equal(t, http.StatusForbidden, res.Status, "a password is no longer a second factor once one is enrolled: %s", string(res.Raw))
	ok := d.askForRollback(s.owner, earlier, apptest.CodeFactor("totp", d.env.TotpCode(enrolled.Secret, 1)))
	assert.Equal(t, http.StatusAccepted, ok.Status, string(ok.Raw))
	var runs int
	require.NoError(t, d.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "PipelineRuns" WHERE "Trigger" = 'rollback'`).Scan(&runs))
	assert.Equal(t, 1, runs, "only the request with the factor created a run")
}

func TestAManualRollbackIsOnlyForSshTargets(t *testing.T) {
	d := newDeployWorld(t)
	s := d.shipped(t)
	d.sql(t, `UPDATE "DeployTargets" SET "Kind" = 'compose' WHERE "Id" = $1`, d.target.ID)
	res := d.askForRollback(s.owner, earlier, apptest.PasswordFactor(s.pass))
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, string(res.Raw))
	assert.Contains(t, string(res.Raw), "ROLLBACK_NOT_SUPPORTED")
}

func TestAPipelineCannotAskForTheRollbackModeItself(t *testing.T) {
	d := newDeployWorld(t)
	yaml := strings.Replace(deployPipeline, "          target: web\n", "          target: web\n          mode: rollback\n          release: "+earlier+"\n", 1)
	run := d.built(t, yaml)
	assert.Equal(t, "failed", d.env.RunJobs(run)["deploy"].State)
	assert.Equal(t, "ROLLBACK_MODE_NOT_ALLOWED", d.env.RunJobs(run)["deploy"].Reason)
}

func TestARollbackWithoutAKeptArtifactStillStartsAndTheServersThatKeptTheReleaseSwitch(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	s.sql(t, `UPDATE "Artifacts" SET "IsDeleted" = true WHERE "Digest" = $1`, s.artifact.Digest[:])
	r := s.rollingBack(t, earlier)
	inputs := r.grant.Grant.GetSpec().GetSteps()[0].GetPlugin().GetInputs()
	assert.NotContains(t, inputs, "artifact")
	assert.Empty(t, r.grant.Grant.GetSpec().GetInputs())

	res := r.owner.Get(r.rollbackPath("/releases"))
	var data struct {
		Releases []struct {
			ReleaseId    string
			ArtifactKept bool
		}
	}
	res.Decode(t, &data)
	for _, rel := range data.Releases {
		if rel.ReleaseId == earlier {
			assert.False(t, rel.ArtifactKept)
		}
	}
}

func (s shipped) rollbackInputs(t *testing.T) map[string]string {
	t.Helper()
	r := s.rollingBack(t, earlier)
	return r.grant.Grant.GetSpec().GetSteps()[0].GetPlugin().GetInputs()
}

func TestARollbackUsesTheHealthCheckStoredOnTheChosenReleaseAndNotTheLatestDeploy(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	s.sql(t, `UPDATE "DeploymentReleases" SET "HealthInputs" = '{"health_check_url": "https://{host}/old-health"}' WHERE "ReleaseId" = $1`, earlier)
	s.sql(t, `UPDATE "DeploymentReleases" SET "HealthInputs" = '{"health_check_url": "https://{host}/new-health"}' WHERE "ReleaseId" = $1`, s.current)
	assert.Equal(t, "https://{host}/old-health", s.rollbackInputs(t)["health_check_url"])
}

func TestARollbackToAReleaseWithoutStoredHealthSettingsRunsWithoutACheck(t *testing.T) {
	s := newDeployWorld(t).shipped(t)
	s.sql(t, `UPDATE "DeploymentReleases" SET "HealthInputs" = '{}' WHERE "ReleaseId" = $1`, earlier)
	assert.NotContains(t, s.rollbackInputs(t), "health_check_url")
}

func TestARollbackToAReleaseWithDamagedHealthSettingsIsRefusedAndStartsNoRun(t *testing.T) {
	d := newDeployWorld(t)
	s := d.shipped(t)
	d.sql(t, `UPDATE "DeploymentReleases" SET "HealthInputs" = '{"health_check_url": 5}' WHERE "ReleaseId" = $1`, earlier)
	res := d.askForRollback(s.owner, earlier, apptest.PasswordFactor(s.pass))
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, string(res.Raw))
	assert.Contains(t, string(res.Raw), "ROLLBACK_SETTINGS_INVALID")
	var runs int
	require.NoError(t, d.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "PipelineRuns" WHERE "Trigger" = 'rollback'`).Scan(&runs))
	assert.Zero(t, runs)
}

func TestARollbackReleaseNeedsTheRestoredReleaseOnTheRow(t *testing.T) {
	d := newDeployWorld(t)
	d.shipped(t)
	_, err := d.env.Pool.Exec(context.Background(), `INSERT INTO "DeploymentReleases"
		("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "ReleaseId", "Kind")
		SELECT r."OrganizationId", r."DeploymentId", r."RunJobId", r."JobId", gen_random_uuid(), r."TargetId", 9, '40-ffffffff-0', 'rollback'
		FROM "DeploymentReleases" r WHERE r."ReleaseId" = $1`, earlier)
	require.Error(t, err, "a rollback row without the release it restored is refused by the database")
}
