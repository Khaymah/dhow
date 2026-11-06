package e2e_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const actionScripts = "../../../../../../../apps/github-action/scripts"

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "dhow")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-o", bin, "github.com/Khaymah/dhow/apps/cli/cmd/dhow")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return bin
}

func runPushScript(t *testing.T, env []string) (string, string, int) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join(actionScripts, "push.sh"))
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), "bash", script)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		code = exit.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

func TestTheActionScriptDrivesTheRealCLIAgainstTheRealServer(t *testing.T) {
	for _, tool := range []string{"bash", "jq", "iconv"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	w := newWorld(t)
	srv := w.env.Serve()
	bin := buildCLI(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o600))
	outputs := filepath.Join(t.TempDir(), "github-output")
	require.NoError(t, os.WriteFile(outputs, nil, 0o600))
	env := []string{
		"DHOW_BIN=" + bin, "DHOW_INGEST_TOKEN=" + w.token.Value, "DHOW_ACTION_SERVER=" + srv.URL, "DHOW_ACTION_PATH=" + dir,
		"DHOW_ACTION_NAME=web", "DHOW_ACTION_REF=main", "DHOW_ACTION_REF_KIND=branch", "DHOW_ACTION_SHA=" + shaNext, "DHOW_ACTION_RETRIES=0",
		"DHOW_PROV_REPOSITORY=Khaymah/dhow", "DHOW_PROV_RUN_ID=42", "DHOW_PROV_RUN_ATTEMPT=1", "DHOW_PROV_WORKFLOW=CI", "DHOW_PROV_ACTOR=sara",
		"DHOW_PROV_EVENT=push", "DHOW_PROV_RUN_URL=https://github.com/Khaymah/dhow/actions/runs/42", "GITHUB_EVENT_NAME=push",
		"GITHUB_OUTPUT=" + outputs,
	}

	stdout, stderr, code := runPushScript(t, env)
	require.Zero(t, code, stdout+stderr)
	assert.NotContains(t, stdout+stderr, w.token.Value)
	written, err := os.ReadFile(outputs)
	require.NoError(t, err)

	var digest, runID string
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT encode("Digest", 'hex') FROM "IngestedArtifacts" WHERE "ProjectId" = $1`, w.project).Scan(&digest))
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT "Id"::text FROM "PipelineRuns" WHERE "ProjectId" = $1 AND "Trigger" = 'artifact'`, w.project).Scan(&runID))
	assert.Contains(t, string(written), "digest="+digest+"\n", "the Action reports the digest the server recorded")
	assert.Contains(t, string(written), "created=true\n")
	assert.Contains(t, string(written), "triggered=true\n")
	assert.Contains(t, string(written), "run-id="+runID+"\n")

	var provenance string
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT "Provenance"::text FROM "IngestedArtifacts" WHERE "ProjectId" = $1`, w.project).Scan(&provenance))
	assert.Contains(t, provenance, `"Provider": "github-actions"`)
	assert.Contains(t, provenance, `"Repository": "Khaymah/dhow"`)

	require.NoError(t, os.WriteFile(outputs, nil, 0o600))
	stdout, stderr, code = runPushScript(t, env)
	require.Zero(t, code, stdout+stderr)
	again, err := os.ReadFile(outputs)
	require.NoError(t, err)
	assert.Contains(t, string(again), "created=false\n")
	assert.Contains(t, string(again), "run-id="+runID+"\n")
	assert.Equal(t, 1, w.runsOfKind(t, "artifact"))

	require.NoError(t, os.WriteFile(outputs, nil, 0o600))
	bad := append([]string{}, env...)
	bad = append(bad, "DHOW_ACTION_REF=feature/x")
	stdout, stderr, code = runPushScript(t, bad)
	assert.Equal(t, 3, code, "a ref outside the token patterns exits 3")
	assert.Contains(t, stderr, "REF_NOT_ALLOWED_FOR_TOKEN")
	assert.NotContains(t, stdout+stderr, w.token.Value)
	empty, err := os.ReadFile(outputs)
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(empty)))
}
