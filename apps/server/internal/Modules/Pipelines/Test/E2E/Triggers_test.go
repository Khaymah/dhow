package e2e_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
)

const simplePipeline = `version: 1
on:
  push: {}
  tag: {}
  manual:
    inputs:
      target:
        type: choice
        options: [staging, production]
        default: staging
      dry:
        type: boolean
  api: {}
jobs:
  build:
    image: IMAGE
    env:
      TARGET: ${{ inputs.target }}
    steps:
      - run: make build
`

func TestTheSameCommitTriggersOnceAcrossDeliveries(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(simplePipeline))
	first := w.env.Deliver(w.trigger(triggers.EventPush, "refs/heads/main", shaA))
	require.True(t, first.Created)
	again := w.env.Deliver(w.trigger(triggers.EventPush, "refs/heads/main", shaA))
	assert.False(t, again.Created, "another delivery of the same ref and sha makes no run")
	assert.Equal(t, first.RunID, again.RunID)
	other := w.env.Deliver(w.trigger(triggers.EventPush, "refs/heads/release", shaA))
	assert.True(t, other.Created, "the same sha on another ref is its own run")
	assert.Equal(t, 2, w.env.CountRows(`SELECT count(*) FROM "PipelineRuns"`))
}

func signGitea(secret string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func TestAWebhookReplayedAfterItsDeliveriesArePurgedMakesNoRun(t *testing.T) {
	w := newWorld(t)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Repositories" SET "IsDeleted" = true, "DeployKeySealed" = NULL, "WebhookSecretSealed" = NULL WHERE "Id" = $1`, w.repo)
	require.NoError(t, err)
	linked := w.owner.Do(apptest.Request{
		Method: http.MethodPut, Path: apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/repository"),
		Body: map[string]any{"CloneUrl": "https://git.example.com/team/app.git"},
	})
	require.Equal(t, http.StatusCreated, linked.Status, string(linked.Raw))
	var l struct {
		Repository    struct{ WebhookUrl string }
		WebhookSecret string
	}
	linked.Decode(t, &l)
	_, hook, _ := strings.Cut(l.Repository.WebhookUrl, apptest.PublicOrigin)
	body := []byte(`{"ref":"refs/heads/main","before":"` + strings.Repeat("1", 40) + `","after":"` + shaB +
		`","pusher":{"login":"sara"}}`)
	send := func(delivery string) apptest.Response {
		return w.env.Webhook(http.MethodPost, hook, map[string]string{
			"X-Gitea-Signature": signGitea(l.WebhookSecret, body), "X-Gitea-Delivery": delivery, "X-Gitea-Event": "push",
		}, body)
	}
	require.Equal(t, http.StatusAccepted, send("delivery-1").Status)
	runs := `SELECT count(*) FROM "PipelineRuns"`
	require.Equal(t, 1, w.env.CountRows(runs))

	_, err = w.env.Pool.Exec(context.Background(), `TRUNCATE "WebhookDeliveries"`)
	require.NoError(t, err)
	replayed := send("delivery-2")
	require.Equal(t, http.StatusAccepted, replayed.Status, string(replayed.Raw))
	assert.Equal(t, "sources.webhook.accepted", replayed.Message, "with the deliveries gone the replay passes the webhook checks")
	assert.Equal(t, 1, w.env.CountRows(runs), "the run table, not the purged deliveries, refuses a sha that already ran on this ref")
}

func TestARunWithAMissingOrWrongFileEndsClearly(t *testing.T) {
	w := newWorld(t)
	missing := w.push("refs/heads/main", shaA)
	w.defs.Put(shaB, "version: 1\non:\n  push: {}\njobs:\n  build:\n    image: alpine@sha256:nope\n    bogus: true\n")
	invalid := w.push("refs/heads/main", shaB)
	w.defs.Put(shaC, pipeline("version: 1\non:\n  tag: {}\njobs:\n  build:\n    image: IMAGE\n    steps:\n      - run: make\n"))
	notEnabled := w.push("refs/heads/main", shaC)
	w.env.StartRuns()

	state, reason := w.env.RunState(missing)
	assert.Equal(t, []string{"skipped", "DEFINITION_NOT_FOUND"}, []string{state, reason})
	state, reason = w.env.RunState(notEnabled)
	assert.Equal(t, []string{"skipped", "TRIGGER_NOT_ENABLED"}, []string{state, reason})
	v := w.view(t, invalid)
	assert.Equal(t, "failed", v.Run.State)
	assert.Equal(t, "PIPELINE_INVALID", v.Run.FailureReason)
	require.NotEmpty(t, v.Issues)
	codes := map[string]bool{}
	for _, i := range v.Issues {
		codes[i.Code] = true
		assert.Positive(t, i.Line, "every problem has its line")
	}
	assert.True(t, codes["UNKNOWN_KEY"], "%+v", v.Issues)
	assert.Empty(t, w.env.RunJobs(invalid))
}

func TestAServerWithoutACheckoutImageFailsGenericRunsClosed(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org, ownerID := env.OrganizationID(d.OrgSlug), uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	repo := env.SeedRepository(org, project, ownerID)
	got := env.Deliver(triggers.Trigger{
		OrganizationID: org, ProjectID: project, RepositoryID: repo, DeliveryID: uuid.New(), Provider: "generic",
		Event: triggers.EventPush, Ref: "refs/heads/main", Sha: shaA,
	})
	env.StartRuns()
	state, reason := env.RunState(got.RunID)
	assert.Equal(t, "failed", state)
	assert.Equal(t, "CHECKOUT_UNAVAILABLE", reason, "a server built without the checkout image cannot read the file of a generic repository")
	assert.Zero(t, env.CountRows(`SELECT count(*) FROM "Jobs"`))
}

func TestPeopleAndTokensStartRunsWithInputs(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(simplePipeline))
	w.push("refs/heads/main", shaA)
	tag := w.env.Deliver(w.trigger(triggers.EventTag, "refs/tags/v1", shaA))
	require.True(t, tag.Created)
	path := apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/runs")
	res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{
		"Ref": "main", "Sha": shaA, "Inputs": map[string]any{"target": "production", "dry": true},
	}})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var created struct{ Id, Trigger, Ref, State string }
	res.Decode(t, &created)
	assert.Equal(t, "manual", created.Trigger)
	assert.Equal(t, "refs/heads/main", created.Ref)
	assert.Equal(t, "pending", created.State)
	w.env.StartRuns()
	run := uuid.MustParse(created.Id)
	jobs := w.env.RunJobs(run)
	require.Equal(t, "active", jobs["build"].State)
	var env map[string]string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Spec"->'env' FROM "Jobs" WHERE "Id" = $1`, jobs["build"].Job).Scan(&env))
	assert.Equal(t, "production", env["TARGET"], "an input reaches the job as data in its environment")

	bad := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{
		"Ref": "main", "Inputs": map[string]any{"target": "moon"},
	}})
	require.Equal(t, http.StatusCreated, bad.Status, string(bad.Raw))
	var badRun struct{ Id string }
	bad.Decode(t, &badRun)
	w.env.StartRuns()
	state, reason := w.env.RunState(uuid.MustParse(badRun.Id))
	assert.Equal(t, []string{"failed", "INPUTS_INVALID"}, []string{state, reason})
	assert.Equal(t, shaA, w.view(t, uuid.MustParse(badRun.Id)).Run.DefinitionSha, "without a sha the run takes the latest pushed head")

	for _, c := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"Ref": "main", "Sha": shaB}, "SHA_NOT_ON_REF"},
		{map[string]any{"Ref": "refs/tags/v1", "Sha": shaB}, "SHA_NOT_ON_REF"},
		{map[string]any{"Ref": "never-pushed"}, "REF_HEAD_UNKNOWN"},
		{map[string]any{"Ref": "never-pushed", "Sha": shaA}, "SHA_NOT_ON_REF"},
	} {
		res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: c.body})
		require.Equal(t, http.StatusUnprocessableEntity, res.Status, string(res.Raw))
		assert.Equal(t, c.code, res.Error.Code, "%v", c.body)
	}

	token := w.env.SeedAPIToken(w.ownerID, w.org, "run.trigger", "run.read")
	viaToken := w.env.Bearer(token.Value, apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{"Ref": "refs/tags/v1", "Sha": shaA}})
	require.Equal(t, http.StatusCreated, viaToken.Status, string(viaToken.Raw))
	var api struct{ Trigger string }
	viaToken.Decode(t, &api)
	assert.Equal(t, "api", api.Trigger)

	for _, ref := range []string{"-x", "refs/pull/1/head", "main..x"} {
		res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{"Ref": ref}})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, ref)
	}
	audit := w.env.CountRows(`SELECT count(*) FROM "AuditEvents" WHERE "Action" IN ('project.run_created', 'project.run_started')`)
	assert.GreaterOrEqual(t, audit, 2, "every run that people start and every run that starts leaves an audit event")
}

func TestStartingRunsByHandIsRateLimited(t *testing.T) {
	w := newWorld(t)
	w.push("refs/heads/main", shaA)
	path := apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/runs")
	limited := 0
	for range 61 {
		res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{"Ref": "main"}})
		if res.Status == http.StatusTooManyRequests {
			limited++
			continue
		}
		require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	}
	assert.Equal(t, 1, limited, "a user starts at most 60 runs in ten minutes")
}

func TestWebhookRunsAreLimitedPerProject(t *testing.T) {
	w := newWorld(t)
	sha := func(i int) string { return fmt.Sprintf("%040x", i+1) }
	for i := range config.PipelineWebhookRunsPerProject {
		w.defs.Put(sha(i), pipeline(simplePipeline))
		got, err := w.env.TryDeliver(w.trigger(triggers.EventPush, "refs/heads/main", sha(i)))
		require.NoError(t, err, "push %d of %d", i+1, config.PipelineWebhookRunsPerProject)
		require.True(t, got.Created)
	}
	_, err := w.env.TryDeliver(w.trigger(triggers.EventPush, "refs/heads/main", "f"+strings.Repeat("0", 39)))
	var limited *apperrors.AppError
	require.ErrorAs(t, err, &limited, "one push more than %d an hour makes no run", config.PipelineWebhookRunsPerProject)
	assert.Equal(t, http.StatusTooManyRequests, limited.Status)
	assert.Positive(t, limited.RetryAfter, "the provider is told when to deliver again")
	again, err := w.env.TryDeliver(w.trigger(triggers.EventPush, "refs/heads/main", sha(0)))
	require.NoError(t, err, "a delivery of a commit that already has a run is not charged")
	assert.False(t, again.Created)
	assert.Equal(t, config.PipelineWebhookRunsPerProject, w.env.CountRows(`SELECT count(*) FROM "PipelineRuns"`))
}
