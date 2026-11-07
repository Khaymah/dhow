//go:build dhowtest

package app_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestTheTargetKeyIsNeverInTheArgvTheEnvironmentOrTheSpecOfAnyProcess(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	running, out := runTarget(t, r, srv, job, fake.Recording{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

	fragments := []string{targetKeyLine1, targetKeyLine2, strings.TrimSpace(targetKeyText)}
	var checked int
	for _, e := range execs(running.Driver.Events()) {
		surface := strings.Join(e.Argv, "\x00") + "\x00" + strings.Join(e.EnvNames, "\x00")
		for name, v := range e.Env {
			surface += "\x00" + name + "=" + v
		}
		for _, f := range fragments {
			assert.NotContains(t, surface, f, "step %d: the key is in the argv or the environment", e.Step)
		}
		checked++
	}
	require.GreaterOrEqual(t, checked, 4, "every step of the job was looked at")

	for _, spec := range running.Driver.Specs() {
		dump := fmt.Sprintf("%+v", spec)
		for _, f := range fragments {
			assert.NotContains(t, dump, f, "sandbox %d: the key is in the sandbox spec", spec.Seq)
		}
	}

	deploy := stepExecs(running.Driver.Events())[1]
	assert.Equal(t, []string{"/deploy"}, deploy.Argv)
	assert.Equal(t, targetKeyText, string(deploy.Saw["target-key"]), "the key arrives only as the content of a mounted file")
}
