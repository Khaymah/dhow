package e2e_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const reviewerPassword = "a long password for the reviewer"

type reviewer struct {
	id    uuid.UUID
	c     *apptest.Client
	codes []string
}

func (r *reviewer) factor() map[string]any {
	code := r.codes[0]
	r.codes = r.codes[1:]
	return apptest.CodeFactor("recovery_code", code)
}

func (w world) reviewer(t *testing.T, env uuid.UUID, email, role string) *reviewer {
	t.Helper()
	id := w.env.SeedUser(w.org, email, reviewerPassword, role)
	c := w.env.SeedSession(id)
	codes := c.EnableTotp(apptest.PasswordFactor(reviewerPassword)).RecoveryCodes
	if env != uuid.Nil {
		w.exec(t, `INSERT INTO "EnvironmentReviewers" ("OrganizationId", "EnvironmentId", "UserId") VALUES ($1, $2, $3)`, w.org, env, id)
	}
	return &reviewer{id: id, c: c, codes: codes}
}

type approvalView struct {
	Id, State, Environment, FactorKind string
	Round                              int
}

func (w world) approvals(t *testing.T, run uuid.UUID) []approvalView {
	t.Helper()
	res := w.owner.Get(w.runPath(run, "/approvals"))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var out []approvalView
	res.Decode(t, &out)
	return out
}

func (w world) decide(c *apptest.Client, run uuid.UUID, approval, action string, body map[string]any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPost, Path: w.runPath(run, "/approvals/"+approval+"/"+action), Body: body})
}

func (w world) gatedDeploy(t *testing.T, sha string) (uuid.UUID, approvalView) {
	t.Helper()
	run := w.toDeploy(t, sha)
	list := w.approvals(t, run)
	require.Len(t, list, 1)
	require.Equal(t, "pending", list[0].State)
	return run, list[0]
}

func TestAReviewerApprovesAndTheJobGetsTheEnvironmentSecrets(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "reviewer@example.com", "developer")
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)
	gate, _ := w.gate(t, run, "deploy")
	assert.Equal(t, "approval", gate)

	res := w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": rev.factor(), "Comment": "looks good"})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, "active", w.env.RunJobs(run)["deploy"].State, "the decision reopens the run at once")
	assert.Equal(t, []string{"approved", "recovery_code"}, []string{w.approvals(t, run)[0].State, w.approvals(t, run)[0].FactorKind})
	var approval *uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "ApprovalId" FROM "Deployments" WHERE "RunId" = $1`, run).Scan(&approval))
	require.NotNil(t, approval)
	assert.Equal(t, a.Id, approval.String())
	assert.Equal(t, 1, w.env.AuditCount("deployment.approval_requested"))
	assert.Equal(t, 1, w.env.AuditCount("deployment.approved"))
	assert.Equal(t, 1, w.env.AuditCount("deployment.started"))

	r, g := w.leaseKey(t, "deploy")
	start, err := r.Start(g)
	require.NoError(t, err)
	assert.Equal(t, envValue, openStart(t, r, g, start)["PROD_KEY"])
}

func TestAnApprovalNeedsASecondFactorAndAnEligibleReviewer(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "reviewer@example.com", "developer")
	other := w.reviewer(t, uuid.Nil, "not-reviewer@example.com", "developer")
	viewer := w.reviewer(t, env, "viewer@example.com", "viewer")
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)

	requireCode(t, w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": apptest.PasswordFactor(reviewerPassword)}),
		http.StatusForbidden, "REVIEWER_NEEDS_SECOND_FACTOR")
	requireCode(t, w.decide(other.c, run, a.Id, "approve", map[string]any{"CurrentFactor": other.factor()}), http.StatusUnprocessableEntity, "REVIEWER_NOT_ELIGIBLE")
	assert.Equal(t, http.StatusForbidden, w.decide(viewer.c, run, a.Id, "approve", map[string]any{"CurrentFactor": viewer.factor()}).Status)
	assert.Equal(t, "waiting", w.env.RunJobs(run)["deploy"].State)
	assert.False(t, w.secretUsed(t))
}

func TestTheUserWhoPushedOrStartedTheRunCannotApproveIt(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "sara@example.com", "developer")
	w.exec(t, `INSERT INTO "UserIdentities" ("UserId", "Provider", "Issuer", "Subject", "Login") VALUES ($1, 'git', 'https://git.example.com', '42', 'Sara')`, rev.id)
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)
	requireCode(t, w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": rev.factor()}), http.StatusForbidden, "SELF_REVIEW_NOT_ALLOWED")
	requireCode(t, w.decide(rev.c, run, a.Id, "reject", map[string]any{"CurrentFactor": rev.factor()}), http.StatusForbidden, "SELF_REVIEW_NOT_ALLOWED")

	w.exec(t, `UPDATE "PipelineRuns" SET "Actor" = 'someone-else', "TriggeredByUserId" = $2 WHERE "Id" = $1`, run, rev.id)
	requireCode(t, w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": rev.factor()}), http.StatusForbidden, "SELF_REVIEW_NOT_ALLOWED")

	w.exec(t, `UPDATE "Environments" SET "AllowSelfApproval" = true WHERE "Id" = $1`, env)
	res := w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": rev.factor()})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
}

func TestARejectionFailsTheDeploy(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "reviewer@example.com", "developer")
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)
	res := w.decide(rev.c, run, a.Id, "reject", map[string]any{"CurrentFactor": rev.factor()})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	deploy := w.env.RunJobs(run)["deploy"]
	assert.Equal(t, []string{"failed", "DEPLOYMENT_REJECTED"}, []string{deploy.State, deploy.Reason})
	requireCode(t, w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": rev.factor()}), http.StatusConflict, "APPROVAL_NOT_PENDING")
	assert.False(t, w.secretUsed(t))
}

func TestAnApprovalRequestExpires(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "reviewer@example.com", "developer")
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)
	w.exec(t, `UPDATE "DeploymentApprovals" SET "ExpiresAt" = $2 WHERE "Id" = $1`, a.Id, w.env.Clock.Now().Add(-time.Minute))
	requireCode(t, w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": rev.factor()}), http.StatusConflict, "APPROVAL_NOT_PENDING")
	w.reopen(t)
	deploy := w.env.RunJobs(run)["deploy"]
	assert.Equal(t, []string{"failed", "APPROVAL_EXPIRED"}, []string{deploy.State, deploy.Reason})
	assert.Equal(t, 1, w.env.AuditCount("deployment.approval_expired"))
}

func TestANewerPushOnTheSameRefSupersedesTheOlderRequest(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.reviewer(t, env, "reviewer@example.com", "developer")
	w.defs.Put(shaA, pipeline(envPipeline))
	w.defs.Put(shaB, pipeline(envPipeline))
	older, _ := w.gatedDeploy(t, shaA)
	newer, _ := w.gatedDeploy(t, shaB)
	assert.Equal(t, "superseded", w.approvals(t, older)[0].State)
	w.reopen(t)
	deploy := w.env.RunJobs(older)["deploy"]
	assert.Equal(t, []string{"canceled", "SUPERSEDED"}, []string{deploy.State, deploy.Reason})
	assert.Equal(t, "pending", w.approvals(t, newer)[0].State)
}

func TestARuleChangeBeforeTheDecisionNeedsANewRequest(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "reviewer@example.com", "developer")
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)
	w.exec(t, `UPDATE "Environments" SET "RuleVersion" = "RuleVersion" + 1 WHERE "Id" = $1`, env)
	requireCode(t, w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": rev.factor()}), http.StatusConflict, "APPROVAL_NOT_PENDING")
	w.reopen(t)
	list := w.approvals(t, run)
	require.Len(t, list, 2)
	assert.Equal(t, []string{"superseded", "pending"}, []string{list[0].State, list[1].State})
	assert.Equal(t, "waiting", w.env.RunJobs(run)["deploy"].State)
}

func TestAnAdminBypassSkipsTheReviewersAndTheTimerOnly(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	w.reviewer(t, env, "reviewer@example.com", "developer")
	owner := &reviewer{c: w.owner, codes: w.owner.EnableTotp(apptest.PasswordFactor(apptest.DefaultSetup().Password)).RecoveryCodes}
	w.exec(t, `UPDATE "Environments" SET "WaitMinutes" = 30 WHERE "Id" = $1`, env)
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)
	requireCode(t, w.decide(owner.c, run, a.Id, "bypass", map[string]any{"CurrentFactor": owner.factor(), "Reason": "hotfix"}),
		http.StatusForbidden, "BYPASS_NOT_ALLOWED")
	w.exec(t, `UPDATE "Environments" SET "AllowAdminBypass" = true WHERE "Id" = $1`, env)
	run2, a2 := run, a
	res := w.decide(owner.c, run2, a2.Id, "bypass", map[string]any{"CurrentFactor": owner.factor()})
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "a bypass needs a reason")
	w.reopen(t)
	list := w.approvals(t, run)
	current := list[len(list)-1]
	res = w.decide(owner.c, run, current.Id, "bypass", map[string]any{"CurrentFactor": owner.factor(), "Reason": "hotfix"})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, "active", w.env.RunJobs(run)["deploy"].State, "the bypass skipped the 30 minute timer")
	assert.Equal(t, 1, w.env.AuditCount("environment.bypassed"))
}

func TestConcurrentApprovalsDecideOnceAndEnqueueOnce(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "reviewer@example.com", "developer")
	w.defs.Put(shaA, pipeline(envPipeline))
	run, a := w.gatedDeploy(t, shaA)
	codes := rev.codes
	var wg sync.WaitGroup
	statuses := make([]int, len(codes))
	for i, code := range codes {
		wg.Add(1)
		go func(i int, code string) {
			defer wg.Done()
			statuses[i] = w.decide(rev.c, run, a.Id, "approve", map[string]any{"CurrentFactor": apptest.CodeFactor("recovery_code", code)}).Status
		}(i, code)
	}
	wg.Wait()
	ok := 0
	for _, s := range statuses {
		if s == http.StatusOK {
			ok++
		}
	}
	assert.Equal(t, 1, ok, "one approval wins: %v", statuses)
	var jobs int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "Jobs" j JOIN "RunJobs" rj ON rj."Id" = j."OriginId" WHERE rj."RunId" = $1 AND rj."Key" = 'deploy'`, run).Scan(&jobs))
	assert.Equal(t, 1, jobs, "the deploy is enqueued once")
}

func TestThePendingListShowsWhatIMayDecide(t *testing.T) {
	w := newWorld(t)
	env := w.production(t)
	rev := w.reviewer(t, env, "reviewer@example.com", "developer")
	other := w.reviewer(t, uuid.Nil, "other@example.com", "developer")
	w.defs.Put(shaA, pipeline(envPipeline))
	w.gatedDeploy(t, shaA)
	path := apptest.OrgPath(w.org, "/deployment-approvals")
	for _, c := range []struct {
		client *apptest.Client
		query  string
		want   int
	}{{rev.c, "?Mine=true", 1}, {other.c, "?Mine=true", 0}, {other.c, "", 1}} {
		res := c.client.Get(path + c.query)
		require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
		var list []approvalView
		res.Decode(t, &list)
		assert.Len(t, list, c.want, c.query)
	}
}
