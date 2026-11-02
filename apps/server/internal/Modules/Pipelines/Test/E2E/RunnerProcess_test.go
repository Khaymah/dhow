package e2e_test

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	"github.com/Khaymah/dhow/internal/gitproof/gitprooftest"
	"github.com/Khaymah/dhow/internal/logging"
)

const procWait = 90 * time.Second

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found")
		dir = parent
	}
}

func buildRunner(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dhow-runner")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-tags", "dhowtest", "-o", path, "./apps/runner/cmd/dhow-runner")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build the runner with the fake driver: %s", out)
	return path
}

type process struct {
	dir    string
	stderr *apptest.SyncBuffer
}

func startRunner(t *testing.T, env *apptest.Env, owner *apptest.Client, org uuid.UUID, recording string) process {
	t.Helper()
	srv := httptest.NewUnstartedServer(env.Handler)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	base := t.TempDir()
	ca := filepath.Join(base, "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))
	bin := buildRunner(t)
	vars := []string{
		"DHOW_RUNNER_STATE_DIR=" + filepath.Join(base, "state"), "DHOW_RUNNER_CA_FILE=" + ca, "DHOW_RUNNER_SERVER_ORIGIN=" + apptest.PublicOrigin,
		"DHOW_RUNNER_DRIVER=fake", "DHOW_RUNNER_MAX_MEMORY=1GiB", "DHOW_RUNNER_MAX_DISK=1GiB", "DHOW_RUNNER_MAX_PIDS=512",
		"DHOW_RUNNER_CANCEL_GRACE=1s", "DHOW_RUNNER_LOG_LEVEL=debug", "DHOW_RUNNER_CHECKOUT_IMAGE=" + apptest.TestImage,
	}
	token, _ := env.CreateRegistrationToken(owner, org, apptest.RegistrationTokenSpec{})
	tokenFile := filepath.Join(base, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte(token+"\n"), 0o600))
	reg := exec.CommandContext(t.Context(), bin, "register", "--url", srv.URL, "--token-file", tokenFile, "--name", "e2e")
	reg.Env = vars
	out, err := reg.CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, os.Remove(tokenFile))
	rec := filepath.Join(base, "recording.json")
	require.NoError(t, os.WriteFile(rec, []byte(recording), 0o600))
	p := process{dir: base, stderr: &apptest.SyncBuffer{}}
	cmd := exec.CommandContext(context.WithoutCancel(t.Context()), bin, "run")
	cmd.Env = append(append([]string(nil), vars...), "DHOW_RUNNER_FAKE_RECORDING="+rec)
	cmd.Stdout, cmd.Stderr = p.stderr, p.stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			_ = cmd.Process.Kill()
		}
	})
	return p
}

func TestARealRunnerTakesAGenericRunFromPushToTheLastJob(t *testing.T) {
	env := apptest.NewWithOptions(t, apptest.Options{CheckoutImage: apptest.TestImage})
	owner, d := env.SetupAndLogin()
	org, ownerID := env.OrganizationID(d.OrgSlug), uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	repo := env.SeedSSHRepository(org, project, ownerID)
	source := pipeline(strings.ReplaceAll(outputsPipeline, "    image: IMAGE\n", "    image: IMAGE\n    resources:\n      memory: 256Mi\n      disk: 256Mi\n      pids: 256\n"))
	commit := apptest.DiscoveryCommit(source)
	recording, err := json.Marshal(map[string]any{"Steps": []map[string]any{{
		"Output":  []map[string]any{{"Data": gitprooftest.Log(commit.Objects)}},
		"Outputs": "dhow_found=true\nversion=4.5.6\n",
	}}})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	stop, err := app.StartPipelines(ctx, env.Wired.Pipelines, logging.New(&apptest.SyncBuffer{}, slog.LevelInfo))
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		stop()
	})
	run := env.Deliver(triggers.Trigger{
		OrganizationID: org, ProjectID: project, RepositoryID: repo.ID, DeliveryID: uuid.New(), Provider: "generic",
		Event: triggers.EventPush, Ref: "refs/heads/main", Sha: commit.Sha,
	}).RunID
	p := startRunner(t, env, owner, org, string(recording))
	ended := assert.Eventually(t, func() bool {
		state, _ := env.RunState(run)
		return state == "succeeded" || state == "failed"
	}, procWait, 200*time.Millisecond, "the run ends")
	require.True(t, ended, "jobs %v; runner output:\n%s", env.RunJobs(run), p.stderr.String())
	state, reason := env.RunState(run)
	require.Equal(t, "succeeded", state, "%s: %s", reason, p.stderr.String())

	jobs := env.RunJobs(run)
	for _, key := range []string{"dhow.discovery", "build", "ship"} {
		assert.Equal(t, "succeeded", jobs[key].State, key)
	}
	var env2 []byte
	require.NoError(t, env.Pool.QueryRow(context.Background(), `SELECT "Spec"->'env' FROM "Jobs" WHERE "Id" = $1`, jobs["ship"].Job).Scan(&env2))
	assert.JSONEq(t, `{"VERSION":"4.5.6"}`, string(env2), "the output of build reached ship")
	assert.Zero(t, env.CountRows(`SELECT count(*) FROM "CommitStatusReports"`), "a generic repository reports no status (decision 14)")
	g := gitWorld{env: env, repo: repo}
	g.assertNoKey(t)
	for _, line := range strings.Split(string(repo.PrivateKey), "\n") {
		if len(line) > 20 && !strings.HasPrefix(line, "-----") {
			assert.NotContains(t, p.stderr.String(), line, "the deploy key appears in the runner's log")
		}
	}
	require.NoError(t, filepath.WalkDir(p.dir, func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err == nil {
			assert.NotContains(t, string(raw), strings.TrimSpace(string(repo.PrivateKey)), "the deploy key is in %s", path)
		}
		return nil
	}))
}
