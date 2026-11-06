package plugin

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
)

func shippedComposeManifest(t *testing.T) Manifest {
	t.Helper()
	raw, err := os.ReadFile("../../../../plugins/deploy-compose/Dockerfile")
	require.NoError(t, err)
	m := manifestLabel.FindSubmatch(raw)
	require.Len(t, m, 2, "the Dockerfile of dhow-deploy-compose has one manifest label")
	parsed, err := Parse(string(m[1]))
	require.NoError(t, err)
	return parsed
}

func TestTheComposePluginDeclaresAReasonForEveryExitCodeItHas(t *testing.T) {
	m := shippedComposeManifest(t)
	want := map[int32]string{
		1: "DEPLOY_FAILED", 2: "PLUGIN_INPUT_INVALID", 10: "DEPLOY_ROLLED_BACK", 11: "DEPLOY_HEALTH_FAILED", 12: "DEPLOY_ROLLBACK_FAILED",
		13: "HOST_KEY_MISMATCH", 14: "ARTIFACT_UNSAFE", 15: "TARGET_LOCKED", 16: "CURRENT_NOT_A_SYMLINK", 17: "TARGET_HOST_UNREACHABLE",
		18: "TARGET_STATE_CORRUPT", 30: "ARTIFACT_UNSAFE",
	}
	for code, name := range want {
		got, ok := m.ExitReason(code)
		assert.True(t, ok, code)
		assert.Equal(t, name, got, code)
	}
	_, ok := m.ExitReason(137)
	assert.False(t, ok, "a code the plugin does not declare stays a failed step")
	assert.Len(t, m.ExitReasons, len(want), "every exit code of the plugin is declared and nothing else")
}

func TestTheManifestOfTheComposePluginIsOneTheRunnerAcceptsAndTheServerCanRelyOn(t *testing.T) {
	m := shippedComposeManifest(t)
	assert.Equal(t, []string{"/dhow-deploy-compose"}, m.Entrypoint)
	assert.True(t, m.Mutating, "a deploy step changes a server, so it is never retried by itself")

	assert.True(t, m.Inputs[jobspec.TargetInput].Required, "the server pins the target by this input name")
	artifact := m.Inputs[jobspec.TargetArtifactInput]
	assert.True(t, artifact.Required)
	assert.True(t, artifact.IsArtifact(), "the runner hands the archive over as a file for this input")
	assert.Equal(t, map[string]string{jobspec.TargetArtifactInput: "stack"}, m.ArtifactNames(map[string]string{jobspec.TargetArtifactInput: "stack", jobspec.TargetInput: "prod"}))

	env, err := m.Env(map[string]string{"target": "prod", "artifact": "stack", "images": "web=registry.example.com/web@sha256:abc"})
	require.NoError(t, err)
	assert.Equal(t, "120s", env["DHOW_INPUT_WAIT_TIMEOUT"])
	assert.Equal(t, "10m", env["DHOW_INPUT_PULL_TIMEOUT"])
	assert.Equal(t, "web=registry.example.com/web@sha256:abc", env["DHOW_INPUT_IMAGES"])
	assert.Equal(t, "200-399", env["DHOW_INPUT_HEALTH_CHECK_STATUS"])
	assert.NotContains(t, env, "DHOW_INPUT_STRATEGY")
}
