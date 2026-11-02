package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	"github.com/Khaymah/dhow/internal/logging"
)

type slowImages struct {
	delay  time.Duration
	calls  atomic.Int64
	active atomic.Int64
	peak   atomic.Int64
}

func (s *slowImages) Resolve(ctx context.Context, image string) (string, error) {
	s.calls.Add(1)
	now := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		peak := s.peak.Load()
		if now <= peak || s.peak.CompareAndSwap(peak, now) {
			break
		}
	}
	select {
	case <-time.After(s.delay):
		return image + "@sha256:" + strings.Repeat("f", 64), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func taggedPipeline(images ...string) string {
	var b strings.Builder
	b.WriteString("version: 1\non:\n  push: {}\njobs:\n")
	for i, image := range images {
		fmt.Fprintf(&b, "  j%d:\n    image: %s\n    steps:\n      - run: make\n", i, image)
	}
	return b.String()
}

func TestPinningRunsInParallelWithinABudgetAndAsksOncePerImage(t *testing.T) {
	images := &slowImages{delay: 300 * time.Millisecond}
	w := newWorldWith(t, apptest.Options{Images: images, PinBudget: 1500 * time.Millisecond})
	var tags []string
	for i := range 8 {
		tags = append(tags, fmt.Sprintf("tool%d:1", i))
	}
	w.defs.Put(shaA, taggedPipeline(append(tags, "tool0:1", "docker.io/library/tool0:1")...))
	run := w.push("refs/heads/main", shaA)
	start := time.Now()
	w.env.StartRuns()
	assert.Less(t, time.Since(start), 1500*time.Millisecond, "eight registry calls of 300 ms run four at a time")
	state, _ := w.env.RunState(run)
	assert.Equal(t, "running", state)
	assert.Equal(t, int64(8), images.calls.Load(), "the same image written three ways is asked once")
	assert.LessOrEqual(t, images.peak.Load(), int64(4))
}

func TestARegistryThatHangsFailsTheRunWhenTheBudgetEnds(t *testing.T) {
	images := &slowImages{delay: 10 * time.Second}
	w := newWorldWith(t, apptest.Options{Images: images, PinBudget: 300 * time.Millisecond})
	w.defs.Put(shaA, taggedPipeline("slow:1"))
	run := w.push("refs/heads/main", shaA)
	start := time.Now()
	w.env.StartRuns()
	assert.Less(t, time.Since(start), 3*time.Second)
	state, reason := w.env.RunState(run)
	assert.Equal(t, []string{"failed", "IMAGE_PIN_FAILED"}, []string{state, reason})
}

func TestTheReconcilerKeepsWorkingWhileARunStartHangs(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(simplePipeline))
	first := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	w.defs.Block(shaB)
	w.push("refs/heads/main", shaB)
	ctx, cancel := context.WithCancel(context.Background())
	stop, err := app.StartPipelines(ctx, w.env.Wired.Pipelines, logging.New(&apptest.SyncBuffer{}, slog.LevelInfo))
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		stop()
	})
	require.Eventually(t, func() bool { return w.defs.Calls() >= 2 }, 5*time.Second, 50*time.Millisecond, "the starter is stuck on the second run")
	w.env.Wired.Jobs.UseRunEvents(nil)
	assert.Equal(t, []string{"build"}, w.finishN(t, 1, succeeded))
	w.env.Wired.Jobs.UseRunEvents(w.env.Wired.Pipelines)
	w.env.Clock.Advance(time.Minute)
	require.Eventually(t, func() bool {
		state, _ := w.env.RunState(first)
		return state == "succeeded"
	}, 12*time.Second, 100*time.Millisecond, "the reconciler repairs the first run while the starter waits on the second")
}

func TestOutputsTooLargeForTheRowAreDroppedWithAReason(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(simplePipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	build := w.env.RunJobs(run)["build"]
	var origin uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "OriginId" FROM "Jobs" WHERE "Id" = $1`, build.Job).Scan(&origin))
	outputs := map[string]string{}
	for i := range 64 {
		outputs[fmt.Sprintf("o%02d", i)] = strings.Repeat(`"`, 1000)
	}
	require.NoError(t, dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		return w.env.Wired.Pipelines.JobFinished(context.Background(), q, runevents.Finished{
			OrganizationID: w.org, JobID: *build.Job, OriginID: origin, State: "succeeded", Outputs: outputs,
		})
	}), "outputs never break the completion")
	got := w.env.RunJobs(run)["build"]
	assert.Equal(t, []string{"succeeded", "OUTPUTS_TRUNCATED"}, []string{got.State, got.Reason})
	state, _ := w.env.RunState(run)
	assert.Equal(t, "succeeded", state)
}

func TestAPullRequestIsAForkUnlessTheProviderSaysOtherwise(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(forkBase))
	pr := func(origin triggers.PullOrigin) triggers.Trigger {
		tr := w.trigger(triggers.EventPullRequest, "refs/pull/9/head", shaB)
		tr.BaseRef, tr.BaseSha, tr.PrNumber, tr.Origin = "refs/heads/main", shaA, 9, origin
		return tr
	}
	unknown := w.env.Deliver(pr(triggers.PullOriginUnknown))
	require.True(t, unknown.Created)
	v := w.view(t, unknown.RunID)
	assert.Equal(t, "awaiting_approval", v.Run.State, "a pull request without proof of its origin waits like a fork")
	same := w.env.Deliver(pr(triggers.PullOriginSameRepository))
	assert.True(t, same.Created, "the same commit from the same repository is another run, not the fork's")
	state, _ := w.env.RunState(same.RunID)
	assert.Equal(t, "pending", state)
	assert.False(t, w.env.Deliver(pr(triggers.PullOriginFork)).Created, "a fork's copy of the same commit is the unknown one")

	for _, c := range []struct {
		event, ref string
	}{
		{triggers.EventPush, "refs/pull/9/head"},
		{triggers.EventPush, "refs/tags/v1"},
		{triggers.EventTag, "refs/heads/main"},
		{triggers.EventPullRequest, "refs/heads/main"},
	} {
		tr := w.trigger(c.event, c.ref, shaC)
		tr.BaseSha, tr.PrNumber = shaA, 3
		err := dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
			_, err := w.env.Wired.Pipelines.Deliver(context.Background(), q, tr)
			return err
		})
		assert.Error(t, err, "%s on %s", c.event, c.ref)
	}
}

func TestCancelInProgressNeverReachesAnotherBranch(t *testing.T) {
	w := newWorld(t)
	file := pipeline(strings.Replace(groupFile("true"), `${{ "deploy-" + git.branch }}`, "deploy", 1))
	w.defs.Put(shaA, file)
	w.defs.Put(shaB, file)
	w.defs.Put(shaC, file)
	main := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	other := w.push("refs/heads/feature", shaB)
	w.env.StartRuns()
	state, _ := w.env.RunState(main)
	assert.Equal(t, "running", state, "a push on another branch never cancels this one")
	state, _ = w.env.RunState(other)
	assert.Equal(t, "queued", state, "it waits for the group instead")
	again := w.push("refs/heads/main", shaC)
	w.env.StartRuns()
	state, reason := w.env.RunState(main)
	assert.Equal(t, []string{"canceled", "CANCELED"}, []string{state, reason}, "a newer push on the same branch cancels it")
	_ = again
}

func TestAStoredValueThatCannotBeReadFailsTheRunClosed(t *testing.T) {
	w := newWorld(t)
	w.images.move("docker.io/library/alpine:3", "docker.io/library/alpine:3@sha256:"+repeat("a", 64))
	w.defs.Put(shaA, "version: 1\non:\n  push: {}\njobs:\n  build:\n    image: alpine:3\n    steps:\n      - run: make\n"+
		"  ship:\n    needs: [build]\n    image: alpine:3\n    steps:\n      - run: make ship\n")
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "PipelineRuns" SET "ImagePins" = '{"alpine:3": 1}' WHERE "Id" = $1`, run)
	require.NoError(t, err)
	assert.Equal(t, []string{"build"}, w.finishN(t, 1, succeeded))
	state, reason := w.env.RunState(run)
	assert.Equal(t, []string{"failed", "RUN_STATE_INVALID"}, []string{state, reason})
	assert.Equal(t, "canceled", w.env.RunJobs(run)["ship"].State, "nothing runs with pins it could not read")
}
