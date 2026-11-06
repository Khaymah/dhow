package stack_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/resolved"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/stack"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

func configIndex(lines []string, wantTail string) int {
	for i, l := range lines {
		if len(l) >= len(wantTail) && l[len(l)-len(wantTail):] == wantTail {
			return i
		}
	}
	return -1
}

func TestTheResolvedModelIsCheckedBeforeThePullAndBeforeEveryStart(t *testing.T) {
	r := newRig(t, "a")
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
	require.Nil(t, f)
	lines := r.log.forHost("a")
	cfg, pull, up := configIndex(lines, "config --format json"), configIndex(lines, "pull --quiet"), configIndex(lines, "--wait-timeout=30")
	require.GreaterOrEqual(t, cfg, 0)
	assert.Less(t, cfg, pull, "the model is checked before images are pulled")
	assert.Less(t, pull, up)
}

func TestAnythingTheResolvedModelDisagreesWithStopsTheDeployBeforeAnyPullOrStart(t *testing.T) {
	for name, edit := range map[string]func(model map[string]any){
		"another project": func(m map[string]any) { m["name"] = "other" },
		"another image": func(m map[string]any) {
			m["services"].(map[string]any)["web"].(map[string]any)["image"] = "evil.example.com/x@sha256:" + digest(1)[len(digest(1))-64:]
		},
		"a privileged service": func(m map[string]any) { m["services"].(map[string]any)["web"].(map[string]any)["privileged"] = true },
		"a writable bind": func(m map[string]any) {
			m["services"].(map[string]any)["web"].(map[string]any)["volumes"] = []any{map[string]any{"type": "bind", "source": "/srv/app/releases/r1/conf", "read_only": false}}
		},
		"a volume with options": func(m map[string]any) {
			m["volumes"] = map[string]any{"data": map[string]any{"driver_opts": map[string]any{"device": "/"}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, "a")
			r.hosts[0].resolve = edit
			f, rec := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
			require.NotNil(t, f)
			assert.Equal(t, stack.ExitPolicy, f.Exit)
			assert.Empty(t, onlyDocker(r.log.forHost("a")), "neither pull nor up ran")
			assert.True(t, rec.has(deployrelease.EventFailed))
			err := error(f)
			assert.Equal(t, resolved.Code, violationCode(t, err))
		})
	}
}

func TestAFailureOfConfigIsARefusalToo(t *testing.T) {
	r := newRig(t, "a")
	r.hosts[0].docker = func(argv []string) deployrelease.ExecResult {
		if isConfig(argv) {
			return deployrelease.ExecResult{ExitCode: 1}
		}
		return deployrelease.ExecResult{}
	}
	r.hosts[0].noConfig = true
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
	require.NotNil(t, f)
	assert.Equal(t, stack.ExitPolicy, f.Exit)
	assert.Empty(t, onlyDocker(r.log.forHost("a")))
}

func TestARollbackChecksTheResolvedModelOfThePreviousReleaseToo(t *testing.T) {
	r := newRig(t, "a")
	okDeploy(t, r, 1)
	r.hosts[0].docker = failUp("r2")
	r.hosts[0].resolve = func(m map[string]any) {
		if m["services"].(map[string]any)["web"].(map[string]any)["image"] == digest(1) {
			m["name"] = "other"
		}
	}
	mark := len(r.log.forHost("a"))
	f, _ := r.deploy("r2", rollout{files: map[string]string{"compose.yaml": composeFile(2, "")}})
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitRollbackFailed, f.Exit)
	for _, line := range onlyDocker(r.log.forHost("a")[mark:]) {
		assert.NotContains(t, line, "releases/r1")
	}
}

func TestTheComponentsOfAPathAreWalkedBeforeThePullToo(t *testing.T) {
	r := newRig(t, "a")
	h := r.hosts[0]
	rel := deployrelease.Release{DeployPath: "/srv/app", Path: "/srv/app/releases/r5", ID: "r5"}
	require.NoError(t, h.MkdirAll(rel.Path, 0o755))
	require.NoError(t, os.WriteFile(h.path(rel.Path+"/compose.yaml"), []byte(composeFile(1, "    volumes: ['./conf/sub:/c:ro']\n")), 0o644))
	require.NoError(t, os.Symlink("/etc", h.path(rel.Path+"/conf")))
	act := &stack.Activator{Project: project, File: "compose.yaml"}
	err := act.Prepare(context.Background(), h, rel)
	require.Error(t, err)
	assert.Equal(t, stack.CodeSource, violationCode(t, err))
	assert.Empty(t, r.log.all(), "nothing ran on the server")
	assert.NoFileExists(t, h.path(rel.Path+"/compose.dhow.yaml"))
}

func TestAnEnvFileWithDockerOrLoaderVariablesIsRefused(t *testing.T) {
	for _, body := range []string{"DOCKER_HOST=tcp://x:2375\n", "docker_config=/x\n", "LD_PRELOAD=/x.so\n", "export LD_LIBRARY_PATH=/x\n", "DOCKER_CONTEXT=x\n"} {
		r := newRig(t, "a")
		require.NoError(t, r.hosts[0].MkdirAll("/srv/app/shared", 0o755))
		require.NoError(t, os.WriteFile(r.hosts[0].path("/srv/app/shared/.env"), []byte(body), 0o600))
		f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
		require.NotNil(t, f, body)
		assert.Equal(t, stack.CodeEnvForbidden, violationCode(t, f), body)
		assert.Empty(t, onlyDocker(r.log.forHost("a")), body)
	}
	r := newRig(t, "a")
	require.NoError(t, r.hosts[0].MkdirAll("/srv/app/shared", 0o755))
	require.NoError(t, os.WriteFile(r.hosts[0].path("/srv/app/shared/.env"), []byte("DOCKERFILE=x\nLDAP=1\nCOMPOSER=1\n"), 0o600))
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
	require.Nil(t, f, "only the prefixes are refused")
}

func TestATopLevelNameMustBeTheProjectOfTheTarget(t *testing.T) {
	r := newRig(t, "a")
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": "name: other\n" + composeFile(1, "")}})
	require.NotNil(t, f)
	assert.Equal(t, stack.ExitPolicy, f.Exit)
	assert.Equal(t, "COMPOSE_UNSAFE_PROJECT_NAME", violationCode(t, f))

	f, _ = r.deploy("r2", rollout{files: map[string]string{"compose.yaml": "name: " + project + "\n" + composeFile(2, "")}})
	require.Nil(t, f)
}
