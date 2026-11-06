package plugin

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
)

var manifestLabel = regexp.MustCompile(`(?m)^LABEL io\.dhow\.plugin\.manifest='(.+)'$`)

func shippedManifest(t *testing.T) Manifest {
	t.Helper()
	raw, err := os.ReadFile("../../../../plugins/deploy-ssh/Dockerfile")
	require.NoError(t, err)
	m := manifestLabel.FindSubmatch(raw)
	require.Len(t, m, 2, "the Dockerfile of dhow-deploy-ssh has one manifest label")
	parsed, err := Parse(string(m[1]))
	require.NoError(t, err)
	return parsed
}

func TestTheManifestOfTheSshPluginIsOneTheRunnerAcceptsAndTheServerCanRelyOn(t *testing.T) {
	m := shippedManifest(t)
	assert.Equal(t, []string{"/dhow-deploy-ssh"}, m.Entrypoint)
	assert.True(t, m.Mutating, "a deploy step changes a server, so it is never retried by itself")

	assert.True(t, m.Inputs[jobspec.TargetInput].Required, "the server pins the target by this input name")
	artifact := m.Inputs[jobspec.TargetArtifactInput]
	assert.False(t, artifact.Required, "only a deploy needs the artifact, a check does not")
	assert.True(t, artifact.IsArtifact(), "the runner hands the archive over as a file for this input")
	assert.Equal(t, map[string]string{jobspec.TargetArtifactInput: "web"}, m.ArtifactNames(map[string]string{jobspec.TargetArtifactInput: "web", jobspec.TargetInput: "prod"}))

	env, err := m.Env(map[string]string{"target": "prod", "artifact": "web", "health_check_url": "https://{host}/healthz"})
	require.NoError(t, err)
	assert.Equal(t, "atomic-symlink", env["DHOW_INPUT_STRATEGY"])
	assert.Equal(t, "200-399", env["DHOW_INPUT_HEALTH_CHECK_STATUS"])
	assert.Equal(t, "3", env["DHOW_INPUT_HEALTH_CHECK_STABLE"])
	assert.Equal(t, "https://{host}/healthz", env["DHOW_INPUT_HEALTH_CHECK_URL"])
	assert.NotContains(t, env, "DHOW_INPUT_KEEP_RELEASES")
}
