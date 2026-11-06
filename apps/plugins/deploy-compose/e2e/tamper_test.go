package e2e_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func (h *composeHost) containerFacts(project string) []string {
	var out []string
	for _, id := range strings.Fields(h.tryExec("docker", "ps", "--all", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.ID}}")) {
		out = append(out, h.tryExec("docker", "inspect", "--format", "privileged={{.HostConfig.Privileged}} env={{json .Config.Env}}", id))
	}
	return out
}

func TestAReleaseWhoseFileWasChangedIsNotStartedByARollback(t *testing.T) {
	d := newDeployment(t, "tamper", 8088, 1)
	h := d.hosts[0]
	v1, v2 := publishApp(t, "v1"), publishApp(t, "v2")
	require.Equal(t, 0, d.deploy("", spec{release: releaseOne, compose: d.compose(v1, healthyCheck(), "")}).exit)

	file := d.path + "/releases/" + releaseOne + "/compose.yaml"
	out, err := dockerRun(bytes.NewReader([]byte(d.compose(v1, healthyCheck(), "    privileged: true\n"))), "exec", "-i", h.name, "tee", file)
	require.NoError(t, err, out)

	res := d.deploy("compose-single-tampered-rollback", spec{release: releaseTwo, compose: d.compose(v2, failingCheck(), "")})
	assert.Equal(t, deployrelease.ExitRollbackFailed, res.exit, res.output)
	assert.Contains(t, res.output, "DEPLOY_ROLLBACK_FAILED")
	assert.True(t, res.has(deployrelease.EventRollbackFailed))
	facts := h.containerFacts(d.project)
	assert.NotEmpty(t, facts)
	for _, fact := range facts {
		assert.Contains(t, fact, "privileged=false", "the changed file was never started")
	}
}

func TestTheEnvironmentOfTheServerAndTheEnvFileOfTheArtifactNeverReachCompose(t *testing.T) {
	d := newDeployment(t, "envleak", 8089, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")
	extra := "    environment:\n      - X=${HOME}\n      - Y=${LEAKED}\n"
	res := d.deploy("", spec{release: releaseOne, compose: d.compose(v1, healthyCheck(), extra), extra: map[string]string{".env": "LEAKED=from-the-artifact\n"}})
	require.Equal(t, 0, res.exit, res.output)
	facts := strings.Join(h.containerFacts(d.project), "\n")
	assert.Contains(t, facts, `"X="`)
	assert.Contains(t, facts, `"Y="`)
	assert.NotContains(t, facts, "from-the-artifact")
	assert.NotContains(t, facts, "/home/deploy")
}
