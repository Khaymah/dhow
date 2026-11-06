package e2e_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const (
	exitPolicy = 30
	releaseOne = "1-aaaaaaaa-1"
	releaseTwo = "2-bbbbbbbb-1"
	releaseTre = "3-cccccccc-1"
)

func TestADeployUpdateAndAFailedUpdateRollBackToThePreviousImages(t *testing.T) {
	d := newDeployment(t, "single", 8081, 1)
	h := d.hosts[0]
	v1, v2, v3 := publishApp(t, "v1"), publishApp(t, "v2"), publishApp(t, "v3")

	first := d.deploy("compose-single-first", spec{release: releaseOne, compose: d.compose(v1, healthyCheck(), ""), health: true})
	require.Equal(t, 0, first.exit, first.output)
	assert.Equal(t, "v1", h.served(d.port))
	assert.Equal(t, "releases/"+releaseOne, h.current(d.path))
	assert.Equal(t, []string{v1}, h.runningImages(d.project))
	assert.Contains(t, h.tryExec("cat", d.path+"/releases/"+releaseOne+"/compose.dhow.yaml"), v1)

	update := d.deploy("compose-single-update", spec{release: releaseTwo, compose: d.compose(v2, healthyCheck(), ""), health: true})
	require.Equal(t, 0, update.exit, update.output)
	assert.Equal(t, "v2", h.served(d.port))
	assert.Equal(t, "releases/"+releaseTwo, h.current(d.path))
	assert.Equal(t, []string{v2}, h.runningImages(d.project))

	broken := d.deploy("compose-single-unhealthy", spec{release: releaseTre, compose: d.compose(v3, failingCheck(), ""), health: true})
	assert.Equal(t, deployrelease.ExitRolledBack, broken.exit, broken.output)
	assert.Contains(t, broken.output, "DEPLOY_ROLLED_BACK")
	assert.Equal(t, "v2", h.served(d.port), "the previous version serves again")
	assert.Equal(t, "releases/"+releaseTwo, h.current(d.path))
	assert.Equal(t, []string{v2}, h.runningImages(d.project), "the services run the images of the previous release")
	assert.True(t, broken.has(deployrelease.EventRolledBack))
	_, group := broken.groupEvent(deployrelease.EventRolledBack)
	assert.True(t, group)
	for _, e := range broken.events {
		if e.Event == deployrelease.EventRolledBack && e.HostID != "" {
			assert.Equal(t, deployrelease.CauseReload, e.Reason)
		}
	}
	assert.Contains(t, h.releases(d.path), releaseTwo)
}

func TestAMissingImageFailsThePullAndLeavesTheRunningServicesAlone(t *testing.T) {
	d := newDeployment(t, "missing", 8082, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")
	require.Equal(t, 0, d.deploy("", spec{release: releaseOne, compose: d.compose(v1, healthyCheck(), "")}).exit)

	gone := "localhost:5000/app@sha256:" + strings.Repeat("0", 64)
	res := d.deploy("compose-single-missing-image", spec{release: releaseTwo, compose: d.compose(gone, healthyCheck(), "")})
	assert.Equal(t, deployrelease.ExitFailed, res.exit, res.output)
	assert.Equal(t, "v1", h.served(d.port))
	assert.Equal(t, "releases/"+releaseOne, h.current(d.path))
	assert.Equal(t, []string{releaseOne}, h.releases(d.path), "the release that could not be pulled is removed")
	assert.Equal(t, []string{v1}, h.runningImages(d.project))
	assert.True(t, res.has(deployrelease.EventFailed))
	assert.False(t, res.has(deployrelease.EventSwitched))
	assert.False(t, res.has(deployrelease.EventRolledBack))
}

func TestTheFileIsRefusedBeforeTheServerIsTouched(t *testing.T) {
	d := newDeployment(t, "policy", 8083, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")
	for name, tc := range map[string]struct{ compose, code string }{
		"a tag":       {d.compose("localhost:5000/app:v1", healthyCheck(), ""), "COMPOSE_IMAGE_NOT_PINNED"},
		"latest":      {d.compose("localhost:5000/app", healthyCheck(), ""), "COMPOSE_IMAGE_NOT_PINNED"},
		"privileged":  {d.compose(v1, healthyCheck(), "    privileged: true\n"), "COMPOSE_UNSAFE_PRIVILEGED"},
		"the root":    {d.composeMount(v1, "/:/host"), "COMPOSE_UNSAFE_BIND_ROOT"},
		"docker.sock": {d.composeMount(v1, "/var/run/docker.sock:/var/run/docker.sock"), "COMPOSE_UNSAFE_DOCKER_SOCKET"},
		"host net":    {d.compose(v1, healthyCheck(), "    network_mode: host\n"), "COMPOSE_UNSAFE_NETWORK_HOST"},
		"a build":     {d.compose(v1, healthyCheck(), "    build: .\n"), "COMPOSE_BUILD_NOT_ALLOWED"},
	} {
		t.Run(name, func(t *testing.T) {
			res := d.deploy("compose-single-refused-"+strings.ReplaceAll(name, " ", "-"), spec{release: releaseOne, compose: tc.compose})
			assert.Equal(t, exitPolicy, res.exit, res.output)
			assert.Contains(t, res.output, tc.code)
			assert.Empty(t, h.releases(d.path), "no release was uploaded")
			assert.Empty(t, h.current(d.path))
			assert.Empty(t, h.runningImages(d.project), "nothing was started")
			assert.True(t, res.has(deployrelease.EventFailed))
			assert.False(t, res.has(deployrelease.EventUploaded))
		})
	}
}

func TestTheAdministratorCanOpenThePolicyButNotThePinning(t *testing.T) {
	d := newDeployment(t, "unsafe", 8084, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")

	res := d.deploy("compose-single-unsafe-allowed", spec{release: releaseOne, compose: d.compose(v1, healthyCheck(), "    cap_add: [CHOWN]\n"), unsafe: true})
	require.Equal(t, 0, res.exit, res.output)
	assert.Equal(t, "v1", h.served(d.port))

	res = d.deploy("", spec{release: releaseTwo, compose: d.compose("localhost:5000/app:v1", healthyCheck(), "    cap_add: [CHOWN]\n"), unsafe: true})
	assert.Equal(t, exitPolicy, res.exit)
	assert.Contains(t, res.output, "COMPOSE_IMAGE_NOT_PINNED")
}

func TestTheImagesInputPinsAServiceThatTheFileNamesByTag(t *testing.T) {
	d := newDeployment(t, "pinned", 8085, 1)
	h := d.hosts[0]
	v2 := publishApp(t, "v2")

	res := d.deploy("", spec{release: releaseOne, compose: d.compose("localhost:5000/app:latest", healthyCheck(), "")})
	assert.Equal(t, exitPolicy, res.exit)

	res = d.deploy("compose-single-images-input", spec{release: releaseOne, compose: d.compose("localhost:5000/app:latest", healthyCheck(), ""), images: "web=" + v2})
	require.Equal(t, 0, res.exit, res.output)
	assert.Equal(t, "v2", h.served(d.port))
	assert.Equal(t, []string{v2}, h.runningImages(d.project))
}

func TestTheFirstReleaseThatDoesNotComeUpIsKept(t *testing.T) {
	d := newDeployment(t, "first", 8086, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")
	res := d.deploy("compose-single-first-unhealthy", spec{release: releaseOne, compose: d.compose(v1, failingCheck(), "")})
	assert.Equal(t, deployrelease.ExitFailed, res.exit, res.output)
	assert.Equal(t, "releases/"+releaseOne, h.current(d.path), "there is nothing to go back to, so current stays")
	assert.False(t, res.has(deployrelease.EventRolledBack))
}

func TestAWrongHostKeyStopsBeforeAnythingIsSent(t *testing.T) {
	d := newDeployment(t, "hostkey", 8087, 1)
	h := d.hosts[0]
	v1 := publishApp(t, "v1")
	_, authorized := newClientKey(t)
	fields := strings.Fields(authorized)
	res := d.deploy("compose-single-wrong-host-key", spec{
		release: releaseOne, compose: d.compose(v1, healthyCheck(), ""), knownBad: map[int]string{0: h.ip + " " + fields[0] + " " + fields[1]},
	})
	assert.Equal(t, deployrelease.ExitHostKeyMismatch, res.exit, res.output)
	assert.Empty(t, h.releases(d.path))
	assert.Empty(t, h.runningImages(d.project))
}
