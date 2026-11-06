package stack_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/stack"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

func failUp(id string) func(argv []string) deployrelease.ExecResult {
	return func(argv []string) deployrelease.ExecResult {
		if strings.Contains(strings.Join(argv, " "), "releases/"+id) && argv[len(argv)-5] == "up" {
			return deployrelease.ExecResult{ExitCode: 1}
		}
		return deployrelease.ExecResult{}
	}
}

func violationCode(t *testing.T, err error) string {
	t.Helper()
	var v *policy.Violation
	require.True(t, errors.As(err, &v), "%v", err)
	return v.Code
}

func TestARollbackNeverRunsAComposeFileThatChangedAfterItWasChecked(t *testing.T) {
	for name, tamper := range map[string]func(*fakeHost){
		"the compose file": func(h *fakeHost) {
			require.NoError(t, os.WriteFile(h.path("/srv/app/releases/r1/compose.yaml"), []byte(composeFile(1, "    privileged: true\n")), 0o644))
		},
		"the compose file with a harmless edit": func(h *fakeHost) {
			require.NoError(t, os.WriteFile(h.path("/srv/app/releases/r1/compose.yaml"), []byte(composeFile(1, "    restart: always\n")), 0o644))
		},
		"the override": func(h *fakeHost) {
			require.NoError(t, os.WriteFile(h.path("/srv/app/releases/r1/compose.dhow.yaml"), []byte("services:\n    web:\n        image: "+digest(9)+"\n"), 0o644))
		},
		"the record": func(h *fakeHost) {
			require.NoError(t, os.Remove(h.path("/srv/app/.dhow/compose-r1.json")))
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, "a")
			okDeploy(t, r, 1)
			tamper(r.hosts[0])
			r.hosts[0].docker = failUp("r2")
			mark := len(r.log.forHost("a"))
			f, rec := r.deploy("r2", rollout{files: map[string]string{"compose.yaml": composeFile(2, "")}})
			require.NotNil(t, f)
			assert.Equal(t, deployrelease.ExitRollbackFailed, f.Exit)
			for _, line := range onlyDocker(r.log.forHost("a")[mark:]) {
				assert.NotContains(t, line, "releases/r1", "nothing of the changed release ran")
			}
			assert.True(t, rec.has(deployrelease.EventRollbackFailed))
			_, group := rec.group(deployrelease.EventRollbackFailed)
			assert.True(t, group)
		})
	}
}

func TestAResumedReleaseIsCheckedBeforeItIsStartedToo(t *testing.T) {
	r := newRig(t, "a")
	okDeploy(t, r, 1)
	h := r.hosts[0]
	act := &stack.Activator{Project: project, File: "compose.yaml"}
	rel := deployrelease.Release{DeployPath: "/srv/app", Path: "/srv/app/releases/r1", ID: "r1"}
	mark := len(r.log.forHost("a"))

	require.NoError(t, act.Activate(context.Background(), h, rel))

	require.NoError(t, os.WriteFile(h.path("/srv/app/releases/r1/compose.yaml"), []byte(composeFile(1, "    cap_add: [SYS_ADMIN]\n")), 0o644))
	err := act.Activate(context.Background(), h, rel)
	require.Error(t, err)
	assert.Equal(t, stack.CodeChanged, violationCode(t, err))
	var f *deployrelease.Failure
	require.ErrorAs(t, err, &f)
	assert.Equal(t, stack.ExitPolicy, f.Exit)
	assert.Len(t, onlyDocker(r.log.forHost("a")[mark:]), 1, "only the first, unchanged, start ran")
}

func TestAnUnrecordedReleaseIsNotStarted(t *testing.T) {
	r := newRig(t, "a")
	h := r.hosts[0]
	require.NoError(t, h.MkdirAll("/srv/app/releases/r7", 0o755))
	require.NoError(t, h.MkdirAll("/srv/app/.dhow", 0o755))
	require.NoError(t, os.WriteFile(h.path("/srv/app/releases/r7/compose.yaml"), []byte(composeFile(1, "")), 0o644))
	act := &stack.Activator{Project: project, File: "compose.yaml"}
	err := act.Activate(context.Background(), h, deployrelease.Release{DeployPath: "/srv/app", Path: "/srv/app/releases/r7", ID: "r7"})
	require.Error(t, err)
	assert.Equal(t, stack.CodeChanged, violationCode(t, err))
	assert.Empty(t, r.log.all())
}

func TestTheOverrideMustPinEveryServiceOfTheFile(t *testing.T) {
	r := newRig(t, "a")
	files := "services:\n  web:\n    image: " + digest(1) + "\n  worker:\n    image: " + digest(2) + "\n"
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": files}})
	require.Nil(t, f)
	h := r.hosts[0]
	short := []byte("services:\n    web:\n        image: " + digest(1) + "\n")
	require.NoError(t, os.WriteFile(h.path("/srv/app/releases/r1/compose.dhow.yaml"), short, 0o644))
	rec, err := os.ReadFile(h.path("/srv/app/.dhow/compose-r1.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(h.path("/srv/app/.dhow/compose-r1.json"), []byte(strings.ReplaceAll(string(rec), `"OverrideSum":"`, `"OverrideSum":"00`)), 0o644))
	act := &stack.Activator{Project: project, File: "compose.yaml"}
	err = act.Activate(context.Background(), h, deployrelease.Release{DeployPath: "/srv/app", Path: "/srv/app/releases/r1", ID: "r1"})
	require.Error(t, err)
	assert.Equal(t, stack.CodeChanged, violationCode(t, err))
}

func TestTheEnvironmentOfTheServerAndOfTheArtifactNeverReachInterpolation(t *testing.T) {
	r := newRig(t, "a")
	extra := "    environment:\n      - X=${HOME}\n      - Y=${LEAKED}\n"
	f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, extra), ".env": "LEAKED=secret\nCOMPOSE_PROFILES=x\n"}})
	require.Nil(t, f)
	for _, line := range onlyDocker(r.log.forHost("a")) {
		assert.True(t, strings.HasPrefix(line, "env -i PATH=/usr/local/bin:/usr/bin:/bin DOCKER_CONFIG=/home/deploy/.docker docker compose "), line)
		assert.Contains(t, line, "--env-file=/dev/null", "the .env of the artifact is never loaded")
		assert.NotContains(t, line, "HOME=")
	}
}

func TestACOMPOSEVariableInTheEnvFileOfTheServerIsRefused(t *testing.T) {
	for _, body := range []string{"COMPOSE_FILE=/etc/x.yaml\n", "A=1\nexport COMPOSE_PROJECT_NAME=x\n", "  COMPOSE_PROFILES=a\n"} {
		r := newRig(t, "a")
		require.NoError(t, r.hosts[0].MkdirAll("/srv/app/shared", 0o755))
		require.NoError(t, os.WriteFile(r.hosts[0].path("/srv/app/shared/.env"), []byte(body), 0o600))
		f, _ := r.deploy("r1", rollout{files: map[string]string{"compose.yaml": composeFile(1, "")}})
		require.NotNil(t, f, body)
		assert.Empty(t, onlyDocker(r.log.forHost("a")), "docker never ran: %q", body)
	}
}

func TestTheHomeFolderOfTheAccountMustLookLikeAPath(t *testing.T) {
	r := newRig(t, "a")
	h := r.hosts[0]
	rel := deployrelease.Release{DeployPath: "/srv/app", Path: "/srv/app/releases/r1", ID: "r1"}
	require.NoError(t, h.MkdirAll(rel.Path, 0o755))
	require.NoError(t, os.WriteFile(h.path(rel.Path+"/compose.yaml"), []byte(composeFile(1, "")), 0o644))
	h.home = "relative/path; rm -rf /"
	act := &stack.Activator{Project: project, File: "compose.yaml"}
	err := act.Prepare(context.Background(), h, rel)
	require.Error(t, err)
	assert.Empty(t, onlyDocker(r.log.all()))
}
