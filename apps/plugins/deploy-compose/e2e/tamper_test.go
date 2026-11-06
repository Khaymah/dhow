package e2e_test

import (
	"bytes"
	"fmt"
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

func (d *deployment) plantAndFollow(image, writerVolume, readerVolume, tail string) string {
	return fmt.Sprintf(`services:
  writer:
    image: %[1]s
    command: ["sh", "-c", "ln -sfn / /data/l; exec httpd -f -p 8080 -h /www"]
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/"]
      interval: 1s
      retries: 2
      start_period: 1s
    volumes:
      - '%[2]s'
  reader:
    image: %[1]s
    ports:
      - "%[4]d:8080"
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/"]
      interval: 1s
      retries: 2
      start_period: 1s
    volumes:
      - '%[3]s'
%[5]s`, image, writerVolume, readerVolume, d.port, tail)
}

func TestAWritableBindThatPlantsALinkIsRefusedAndNamedVolumesDeployFine(t *testing.T) {
	d := newDeployment(t, "plant", 8092, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")

	exploit := d.plantAndFollow(v1, "./data:/data", "./data/l:/x:ro", "")
	res := d.deploy("compose-single-writable-bind-refused", spec{release: releaseOne, compose: exploit, extra: map[string]string{"data/keep": "x"}})
	assert.Equal(t, exitPolicy, res.exit, res.output)
	assert.Contains(t, res.output, "COMPOSE_UNSAFE_BIND_WRITABLE")
	assert.Empty(t, h.releases(d.path), "refused before anything reached the server")
	assert.Empty(t, h.runningImages(d.project))

	named := d.plantAndFollow(v1, "data:/data", "data:/x:ro", "volumes:\n  data: {}\n")
	ok := d.deploy("", spec{release: releaseOne, compose: named})
	require.Equal(t, 0, ok.exit, ok.output)
	assert.Equal(t, "v1", h.served(d.port))
}

func TestAReadOnlyThatIsOnlyReadOnlyAfterInterpolationIsRefusedBeforeTheServerIsTouched(t *testing.T) {
	d := newDeployment(t, "interp", 8093, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")
	writer := "      - type: bind\n        source: ./data\n        target: /data\n        read_only: \"${RO:-false}\"\n"
	exploit := d.plantAndFollow(v1, "unused:/unused", "./data/l:/x:ro", "")
	exploit = strings.Replace(exploit, "      - 'unused:/unused'\n", writer, 1)
	res := d.deploy("compose-single-interpolated-read-only-refused", spec{release: releaseOne, compose: exploit, extra: map[string]string{"data/keep": "x"}})
	assert.Equal(t, exitPolicy, res.exit, res.output)
	assert.Contains(t, res.output, "COMPOSE_UNSAFE_INTERPOLATION")
	assert.Empty(t, h.releases(d.path), "refused before anything reached the server")
	assert.Empty(t, h.runningImages(d.project))
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
