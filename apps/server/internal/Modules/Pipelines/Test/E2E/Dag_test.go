package e2e_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dagPipeline = `version: 1
on:
  push:
    branches: [main]
jobs:
  lint:
    image: IMAGE
    steps:
      - run: make lint
  unit:
    image: IMAGE
    steps:
      - run: make test
  build:
    needs: [lint, unit]
    image: IMAGE
    strategy:
      matrix:
        os: [debian, alpine]
        node: [20, 22]
    steps:
      - run: make build
  publish:
    needs: [build]
    image: IMAGE
    steps:
      - run: make publish
`

func builds() []string {
	return []string{
		"build (node=20, os=alpine)", "build (node=20, os=debian)", "build (node=22, os=alpine)", "build (node=22, os=debian)",
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestADagRunsInOrderWithItsMatrix(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(dagPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	state, _ := w.env.RunState(run)
	require.Equal(t, "running", state)

	jobs := w.env.RunJobs(run)
	require.Len(t, jobs, 7)
	assert.Equal(t, "active", jobs["lint"].State)
	assert.Equal(t, "active", jobs["unit"].State, "lint and unit run side by side")
	for _, b := range builds() {
		assert.Equal(t, "waiting", jobs[b].State)
	}

	assert.Equal(t, []string{"lint"}, w.finishN(t, 1, succeeded), "lint was queued first")
	for _, b := range builds() {
		assert.Equal(t, "waiting", w.env.RunJobs(run)[b].State, "a build waits for unit too")
	}
	assert.Equal(t, []string{"unit"}, w.finishN(t, 1, succeeded))
	jobs = w.env.RunJobs(run)
	for _, b := range builds() {
		assert.Equal(t, "active", jobs[b].State, "all four matrix jobs start once both needs succeed")
	}
	assert.Equal(t, "waiting", jobs["publish"].State)
	assert.Equal(t, builds(), sorted(w.finishN(t, 4, succeeded)))
	assert.Equal(t, []string{"publish"}, w.drain(t, succeeded))

	state, reason := w.env.RunState(run)
	assert.Equal(t, "succeeded", state)
	assert.Empty(t, reason)
	v := w.view(t, run)
	require.Len(t, v.Jobs, 7)
	assert.Equal(t, "lint", v.Jobs[0].Key, "the API lists jobs in pipeline order")
	assert.Equal(t, "publish", v.Jobs[6].Key)
	assert.Equal(t, map[string]string{"node": "20", "os": "debian"}, v.Jobs[2].Matrix)
	assert.Equal(t, shaA, v.Run.DefinitionSha)
	assert.Len(t, v.Run.DefinitionDigest, 64)
}

func TestAFailedNeedSkipsEverythingAfterIt(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(dagPipeline))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()

	key, ok := w.finish(failed)
	require.True(t, ok)
	other, ok := w.finish(succeeded)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"lint", "unit"}, []string{key, other})
	_, more := w.finish(succeeded)
	assert.False(t, more, "nothing after a failed need is queued")

	jobs := w.env.RunJobs(run)
	assert.Equal(t, "failed", jobs[key].State)
	assert.Equal(t, "STEP_FAILED", jobs[key].Reason)
	for _, b := range append(builds(), "publish") {
		assert.Equal(t, "skipped", jobs[b].State, b)
		assert.Equal(t, "NEEDS_FAILED", jobs[b].Reason, b)
	}
	state, reason := w.env.RunState(run)
	assert.Equal(t, "failed", state)
	assert.Equal(t, "JOBS_FAILED", reason)
}

func TestAConditionThatIsFalseSkipsAndCountsAsDone(t *testing.T) {
	w := newWorld(t)
	w.defs.Put(shaA, pipeline(`version: 1
on:
  push: {}
jobs:
  release:
    if: git.is_tag
    image: IMAGE
    steps:
      - run: make release
  notify:
    needs: [release]
    image: IMAGE
    steps:
      - run: make notify
`))
	run := w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	jobs := w.env.RunJobs(run)
	assert.Equal(t, "skipped", jobs["release"].State)
	assert.Equal(t, "CONDITION_FALSE", jobs["release"].Reason)
	assert.Equal(t, "active", jobs["notify"].State, "a need skipped by its condition counts as done")
	assert.Equal(t, []string{"notify"}, w.drain(t, succeeded))
	state, _ := w.env.RunState(run)
	assert.Equal(t, "succeeded", state)
}
