package e2e_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	jobsmodels "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var errRollBack = errors.New("the caller rolls back")

func pipelineRequest(w world, features ...string) jobqueue.Request {
	return jobqueue.Request{
		OrganizationID: w.org, CreatedByUserID: w.ownerID, OriginKind: jobqueue.OriginPipeline, OriginID: uuid.New(),
		Name: "build (pipeline)", RequiredFeatures: features,
		Spec: &runnerv1.JobSpec{Image: apptest.TestImage, Steps: []*runnerv1.Step{{Name: "build", Run: "make build"}}},
	}
}

func countJobs(t *testing.T, w world, origin uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Jobs" WHERE "OriginId" = $1`, origin).Scan(&n))
	return n
}

func TestEnqueueInsideARolledBackTransactionLeavesNoJob(t *testing.T) {
	w := newWorld(t)
	req := pipelineRequest(w)
	var queued jobqueue.Enqueued
	err := dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		var err error
		queued, err = w.env.Wired.JobQueue.Enqueue(context.Background(), q, req)
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, queued.JobID)
		return errRollBack
	})
	require.ErrorIs(t, err, errRollBack)
	assert.Zero(t, countJobs(t, w, req.OriginID), "the job must vanish with the caller's transaction")
	var byID int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Jobs" WHERE "Id" = $1`, queued.JobID).Scan(&byID))
	assert.Zero(t, byID)
}

func TestEnqueueCommitsWithTheCallerAndRecordsTheOrigin(t *testing.T) {
	w := newWorld(t)
	req := pipelineRequest(w, jobqueue.FeatureCheckout, jobqueue.FeatureCache, jobqueue.FeatureCheckout)
	require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		_, err := w.env.Wired.JobQueue.Enqueue(context.Background(), q, req)
		return err
	}))
	var kind string
	var features []string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "OriginKind", "RequiredFeatures" FROM "Jobs" WHERE "OriginId" = $1`, req.OriginID).Scan(&kind, &features))
	assert.Equal(t, jobqueue.OriginPipeline, kind)
	assert.Equal(t, []string{jobqueue.FeatureCache, jobqueue.FeatureCheckout}, features)

	manual := w.job(t, apptest.SimpleSpec(), nil)
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "OriginKind" FROM "Jobs" WHERE "Id" = $1`, manual).Scan(&kind))
	assert.Equal(t, jobqueue.OriginManual, kind, "a job from the API is manual")
}

func TestAJobThatNeedsAFeatureIsNeverLeasedToARunnerWithoutIt(t *testing.T) {
	w := newWorld(t)
	r := w.runner(apptest.SeedRunnerSpec{})
	require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		_, err := w.env.Wired.JobQueue.Enqueue(context.Background(), q, pipelineRequest(w, jobqueue.FeatureCheckout))
		return err
	}))
	noLease(t, r)
	plain := pipelineRequest(w)
	require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		_, err := w.env.Wired.JobQueue.Enqueue(context.Background(), q, plain)
		return err
	}))
	g := leaseNow(t, r)
	var origin uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "OriginId" FROM "Jobs" WHERE "Id" = $1`, g.Grant.GetJobId()).Scan(&origin))
	assert.Equal(t, plain.OriginID, origin)
}

func checkoutSpec() *runnerv1.JobSpec {
	return &runnerv1.JobSpec{
		Image: apptest.TestImage,
		Checkout: &runnerv1.Checkout{
			RepositoryUrl: "https://git.example.com/team/app.git", Ref: "refs/heads/main",
			Sha: "0123456789abcdef0123456789abcdef01234567", Transport: runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_HTTPS,
			CredentialSecret: jobspec.CheckoutSecret,
		},
		Secrets: []*runnerv1.SecretRef{{Name: jobspec.CheckoutSecret, Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.CheckoutSecretFile}}},
		Steps: []*runnerv1.Step{
			{Name: "build", Run: "make build"},
			{Name: "notify", Plugin: &runnerv1.Plugin{Image: apptest.TestImage, Inputs: map[string]string{"channel": "ops"}}},
		},
	}
}

func TestEnqueueTakesTheFeaturesFromTheSpecItself(t *testing.T) {
	w := newWorld(t)
	r := w.runner(apptest.SeedRunnerSpec{})
	req := pipelineRequest(w)
	req.Spec = checkoutSpec()
	require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		_, err := w.env.Wired.JobQueue.Enqueue(context.Background(), q, req)
		return err
	}), "the checkout credential is minted at Start, so it does not need the M5 secret store")
	var features []string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "RequiredFeatures" FROM "Jobs" WHERE "OriginId" = $1`, req.OriginID).Scan(&features))
	assert.Equal(t, []string{jobqueue.FeatureCheckout, jobqueue.FeaturePlugins}, features)
	noLease(t, r)
	partial := w.runner(apptest.SeedRunnerSpec{Features: []string{jobqueue.FeatureCheckout}})
	noLease(t, partial)
	full := w.runner(apptest.SeedRunnerSpec{Features: []string{jobqueue.FeatureCheckout, jobqueue.FeaturePlugins}})
	g := leaseNow(t, full)
	assert.Equal(t, "https://git.example.com/team/app.git", g.Grant.GetSpec().GetCheckout().GetRepositoryUrl())
}

func TestEnqueueValidatesTheEvaluatedSpecAgain(t *testing.T) {
	w := newWorld(t)
	tests := []struct {
		name   string
		mutate func(*runnerv1.JobSpec)
	}{
		{"an evaluated env value with NUL", func(s *runnerv1.JobSpec) { s.Env = map[string]string{"BRANCH": "main\x00"} }},
		{"an evaluated env value that is too long", func(s *runnerv1.JobSpec) {
			s.Env = map[string]string{"MESSAGE": strings.Repeat("x", jobspec.MaxEnvValueBytes+1)}
		}},
		{"an evaluated plugin input with NUL", func(s *runnerv1.JobSpec) { s.Steps[1].Plugin.Inputs["channel"] = "ops\x00" }},
		{"a plugin that is not pinned", func(s *runnerv1.JobSpec) { s.Steps[1].Plugin.Image = "docker.io/library/alpine:3" }},
		{"a checkout from file://", func(s *runnerv1.JobSpec) { s.Checkout.RepositoryUrl = "file:///etc" }},
		{"a checkout ref that is an option", func(s *runnerv1.JobSpec) { s.Checkout.Ref = "--upload-pack=touch" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := pipelineRequest(w)
			req.Spec = checkoutSpec()
			tt.mutate(req.Spec)
			err := dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
				_, err := w.env.Wired.JobQueue.Enqueue(context.Background(), q, req)
				return err
			})
			var ae *apperrors.AppError
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, apperrors.CodeValidationFailed, ae.Code)
			assert.Zero(t, countJobs(t, w, req.OriginID))
		})
	}
}

func TestEnqueueRefusesWhatOnlyTheAPIMayQueue(t *testing.T) {
	w := newWorld(t)
	other := w.env.SeedOrganization("Other", "other")
	outsider := w.env.SeedUser(other, "outsider@example.com", "a long password for the outsider", "owner")
	tests := []struct {
		name   string
		mutate func(*jobqueue.Request)
		code   string
	}{
		{"a manual origin", func(r *jobqueue.Request) { r.OriginKind = jobqueue.OriginManual }, ""},
		{"no origin", func(r *jobqueue.Request) { r.OriginID = uuid.Nil }, ""},
		{"no creator", func(r *jobqueue.Request) { r.CreatedByUserID = uuid.Nil }, ""},
		{"an unknown feature", func(r *jobqueue.Request) { r.RequiredFeatures = []string{"docker"} }, apperrors.CodeValidationFailed},
		{"an empty name", func(r *jobqueue.Request) { r.Name = "" }, apperrors.CodeValidationFailed},
		{"an unpinned image", func(r *jobqueue.Request) { r.Spec.Image = "alpine" }, apperrors.CodeValidationFailed},
		{"an invalid label", func(r *jobqueue.Request) { r.Spec.Labels = []string{"no spaces allowed"} }, apperrors.CodeValidationFailed},
		{"a creator of another organization", func(r *jobqueue.Request) { r.CreatedByUserID = outsider }, ""},
		{"a secret", func(r *jobqueue.Request) {
			r.Spec.Secrets = []*runnerv1.SecretRef{{Name: "TOKEN", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "TOKEN"}}}
		}, apperrors.CodeSecretsUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := pipelineRequest(w)
			tt.mutate(&req)
			err := dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
				_, err := w.env.Wired.JobQueue.Enqueue(context.Background(), q, req)
				return err
			})
			require.Error(t, err)
			if tt.code == "" {
				assert.ErrorIs(t, err, jobs.ErrInvalidEnqueue)
				return
			}
			var ae *apperrors.AppError
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, tt.code, ae.Code)
		})
	}
}

func TestCancelThroughThePortFollowsTheCallerTransaction(t *testing.T) {
	w := newWorld(t)
	req := pipelineRequest(w)
	var queued jobqueue.Enqueued
	require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		var err error
		queued, err = w.env.Wired.JobQueue.Enqueue(context.Background(), q, req)
		return err
	}))
	cancel := jobqueue.Cancellation{OrganizationID: w.org, JobID: queued.JobID, RequestedBy: w.ownerID}
	err := dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		changed, err := w.env.Wired.JobQueue.Cancel(context.Background(), q, cancel)
		require.NoError(t, err)
		assert.True(t, changed)
		return errRollBack
	})
	require.ErrorIs(t, err, errRollBack)
	assert.Equal(t, "queued", w.get(t, queued.JobID).Job.State, "a rolled-back cancel leaves the job queued")

	for range 2 {
		require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
			_, err := w.env.Wired.JobQueue.Cancel(context.Background(), q, cancel)
			return err
		}))
	}
	assert.Equal(t, "canceled", w.get(t, queued.JobID).Job.State, "cancel is idempotent")
	other := w.env.SeedOrganization("Other", "other")
	err = dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		_, err := w.env.Wired.JobQueue.Cancel(context.Background(), q, jobqueue.Cancellation{OrganizationID: other, JobID: queued.JobID, RequestedBy: w.ownerID})
		return err
	})
	var ae *apperrors.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeNotFound, ae.Code, "a job of another organization is not found")
}

func enqueue(t *testing.T, w world, r jobqueue.Request) error {
	t.Helper()
	return dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		_, err := w.env.Wired.JobQueue.Enqueue(context.Background(), q, r)
		return err
	})
}

func TestAnOriginNeverCrossesOrganizations(t *testing.T) {
	w := newWorld(t)
	req := pipelineRequest(w)
	require.NoError(t, enqueue(t, w, req))
	require.NoError(t, enqueue(t, w, req), "a retry of the same origin stays in its organization")

	other := w.env.SeedOrganization("Other", "other")
	otherOwner := w.env.SeedUser(other, "other-owner@example.com", "a long password for the other owner", "owner")
	foreign := req
	foreign.OrganizationID, foreign.CreatedByUserID = other, otherOwner
	assert.ErrorIs(t, enqueue(t, w, foreign), jobs.ErrInvalidEnqueue)
	assert.Equal(t, 2, countJobs(t, w, req.OriginID))
}

func TestJobsThatNeedFeaturesHaveTheirOwnSmallCap(t *testing.T) {
	w := newWorld(t)
	seed := pipelineRequest(w, jobqueue.FeatureCheckout)
	require.NoError(t, enqueue(t, w, seed))
	_, err := w.env.Pool.Exec(context.Background(),
		`INSERT INTO "Jobs" ("OrganizationId", "CreatedByUserId", "Name", "Spec", "SpecDigest", "Image", "Isolation", "QueueDeadline",
		                     "OriginKind", "OriginId", "RequiredFeatures")
		 SELECT "OrganizationId", "CreatedByUserId", "Name", "Spec", "SpecDigest", "Image", "Isolation", "QueueDeadline",
		        "OriginKind", "OriginId", "RequiredFeatures"
		 FROM "Jobs", generate_series(2, $2) WHERE "OriginId" = $1`, seed.OriginID, config.JobMaxQueuedWithFeatures)
	require.NoError(t, err)

	var ae *apperrors.AppError
	require.ErrorAs(t, enqueue(t, w, pipelineRequest(w, jobqueue.FeatureCache)), &ae)
	assert.Equal(t, apperrors.CodeJobQueueFull, ae.Code)
	require.NoError(t, enqueue(t, w, pipelineRequest(w)), "jobs a runner can take today are not held back")
	w.job(t, apptest.SimpleSpec(), nil)
}

func TestAManualJobAlsoNeedsTheFeaturesOfItsSpec(t *testing.T) {
	w := newWorld(t)
	r := w.runner(apptest.SeedRunnerSpec{})
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Subject: policy.Subject{UserID: w.ownerID}})
	spec := &runnerv1.JobSpec{Steps: []*runnerv1.Step{{Name: "notify", Plugin: &runnerv1.Plugin{Image: apptest.TestImage}}}}
	created, err := w.env.Wired.Jobs.Create(ctx, w.org, jobsmodels.CreateInput{Name: "plugin only", Spec: spec})
	require.NoError(t, err)
	var features []string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "RequiredFeatures" FROM "Jobs" WHERE "Id" = $1`, created.Job.ID).Scan(&features))
	assert.Equal(t, []string{jobqueue.FeaturePlugins}, features)
	noLease(t, r)
	g := leaseNow(t, w.runner(apptest.SeedRunnerSpec{Features: []string{jobqueue.FeaturePlugins}}))
	assert.Equal(t, created.Job.ID.String(), g.Grant.GetJobId())
}
