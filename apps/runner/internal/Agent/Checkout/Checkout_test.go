package checkout

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoURL = "ssh://git@git.example.com/team/app.git"

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type world struct {
	t       *testing.T
	root    string
	origin  string
	seed    string
	work    string
	scratch string
	ssh     string
	git     string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	w := &world{
		t: t, root: root, origin: filepath.Join(root, "srv", "team", "app.git"), seed: filepath.Join(root, "seed"),
		work: filepath.Join(root, "work"), scratch: filepath.Join(root, "scratch"), ssh: filepath.Join(root, "git-ssh"), git: git,
	}
	require.NoError(t, os.MkdirAll(w.work, 0o700))
	script := "#!/bin/sh\n" +
		"for last; do :; done\n" +
		"path=$(printf '%s' \"$last\" | sed -e \"s/^git-upload-pack '//\" -e \"s/'\\$//\")\n" +
		"exec git-upload-pack \"" + filepath.Join(root, "srv") + "$path\"\n"
	require.NoError(t, os.WriteFile(w.ssh, []byte(script), 0o700))
	w.plain("init", "--quiet", "--bare", "-b", "main", w.origin)
	w.plain("init", "--quiet", "-b", "main", w.seed)
	w.commit("README", "one\n")
	return w
}

func (w *world) plain(args ...string) string {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = []string{
		"HOME=" + w.root, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com",
	}
	out, err := cmd.CombinedOutput()
	require.NoError(w.t, err, string(out))
	return strings.TrimSpace(string(out))
}

func (w *world) commit(name, body string) string {
	w.t.Helper()
	require.NoError(w.t, os.WriteFile(filepath.Join(w.seed, name), []byte(body), 0o600))
	w.plain("-C", w.seed, "add", "--", name)
	w.plain("-C", w.seed, "commit", "--quiet", "-m", name)
	w.plain("-C", w.seed, "push", "--quiet", "--force", w.origin, "main")
	return w.plain("-C", w.seed, "rev-parse", "HEAD")
}

func (w *world) run(sha string, env map[string]string) (int, string) {
	w.t.Helper()
	vars := map[string]string{EnvURL: repoURL, EnvRef: "refs/heads/main", EnvSha: sha, EnvDepth: "1", "PATH": os.Getenv("PATH")}
	for k, v := range env {
		vars[k] = v
	}
	var out lockedBuffer
	code := Run(context.Background(), Options{
		Getenv: func(k string) string { return vars[k] }, Stdout: &out, Stderr: &out, Dir: w.work,
		Git: w.git, SSHHelper: w.ssh, ScratchDir: w.scratch,
	})
	return code, out.String()
}

func TestCheckoutFetchesTheCommit(t *testing.T) {
	w := newWorld(t)
	sha := w.plain("-C", w.seed, "rev-parse", "HEAD")
	code, out := w.run(sha, nil)
	require.Equal(t, ExitOK, code, out)
	assert.FileExists(t, filepath.Join(w.work, "README"))
	assert.Equal(t, sha, w.plain("-C", w.work, "rev-parse", "HEAD"))
}

func TestARefThatMovedOnIsFetchedByCommit(t *testing.T) {
	w := newWorld(t)
	old := w.plain("-C", w.seed, "rev-parse", "HEAD")
	w.commit("second", "two\n")
	code, out := w.run(old, nil)
	require.Equal(t, ExitOK, code, out)
	assert.Contains(t, out, "fetched by its id")
	assert.Equal(t, old, w.plain("-C", w.work, "rev-parse", "HEAD"))
	assert.NoFileExists(t, filepath.Join(w.work, "second"))
}

func TestACommitThatIsGoneIsRefMoved(t *testing.T) {
	w := newWorld(t)
	code, out := w.run(strings.Repeat("ab", 20), nil)
	assert.Equal(t, ExitRefMoved, code, out)
	assert.Contains(t, out, "CHECKOUT_REF_MOVED")
}

func TestADeletedBranchFallsBackToTheCommit(t *testing.T) {
	w := newWorld(t)
	sha := w.plain("-C", w.seed, "rev-parse", "HEAD")
	code, out := w.run(sha, map[string]string{EnvRef: "refs/heads/gone"})
	require.Equal(t, ExitOK, code, out)
}

func TestHostKeyMismatchIsReported(t *testing.T) {
	w := newWorld(t)
	script := "#!/bin/sh\nprintf 'Host key verification failed.\\n' >&2\n: > \"$DHOW_GIT_MARKERS/host-key-mismatch\"\nexit 255\n"
	require.NoError(t, os.WriteFile(w.ssh, []byte(script), 0o700))
	code, out := w.run(strings.Repeat("ab", 20), nil)
	assert.Equal(t, ExitHostKeyMismatch, code, out)
}

func TestDiskFullIsReported(t *testing.T) {
	w := newWorld(t)
	fake := filepath.Join(w.root, "git-full")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\ncase \"$*\" in *fetch*) echo 'fatal: write error: No space left on device' >&2; exit 128;; esac\nexec "+w.git+" \"$@\"\n"), 0o700))
	w.git = fake
	code, out := w.run(w.plain("-C", w.seed, "rev-parse", "HEAD"), nil)
	assert.Equal(t, ExitDiskFull, code, out)
}

func TestBadSettingsNeverReachGit(t *testing.T) {
	w := newWorld(t)
	marker := filepath.Join(w.root, "ran")
	require.NoError(t, os.WriteFile(filepath.Join(w.root, "git-spy"), []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o700))
	w.git = filepath.Join(w.root, "git-spy")
	for _, env := range []map[string]string{
		{EnvURL: "ext::sh -c touch% /tmp/x"},
		{EnvURL: "git@git.example.com:team/app.git"},
		{EnvRef: "--upload-pack=touch /tmp/x"},
		{EnvSha: "HEAD"},
		{EnvDepth: "0"},
	} {
		code, _ := w.run(strings.Repeat("ab", 20), env)
		assert.Equal(t, ExitFailed, code)
	}
	assert.NoFileExists(t, marker)
}

func TestRunSSHChecksWhatGitAsksAndPinsTheHost(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(root, "argv")
	fakeSSH := filepath.Join(root, "ssh")
	require.NoError(t, os.WriteFile(fakeSSH, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+log+"\n"), 0o700))
	env := map[string]string{envGitURL: repoURL, envMarkers: root, "HOME": root}
	o := HelperOptions{
		Getenv: func(k string) string { return env[k] }, Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		SSH: fakeSSH, SecretsDir: "/run/dhow/secrets", Connect: "/run/dhow/bin/connect",
	}
	code := RunSSH(context.Background(), []string{"-o", "SendEnv=GIT_PROTOCOL", "git@git.example.com", "git-upload-pack '/team/app.git'"}, o)
	require.Equal(t, ExitOK, code)
	argv, err := os.ReadFile(log)
	require.NoError(t, err)
	got := string(argv)
	for _, want := range []string{"-F\n/dev/null\n", "StrictHostKeyChecking=yes", "UserKnownHostsFile=/run/dhow/secrets/known_hosts", "IdentityFile=/run/dhow/secrets/git-credential", "IdentitiesOnly=yes", "--\ngit.example.com\ngit-upload-pack '/team/app.git'\n"} {
		assert.Contains(t, got, want)
	}
	require.NoError(t, os.Remove(log))
	code = RunSSH(context.Background(), []string{"-o", "ProxyCommand=touch /tmp/x", "git@git.example.com", "git-upload-pack '/team/app.git'"}, o)
	assert.Equal(t, ExitFailed, code)
	assert.NoFileExists(t, log, "ssh must not run for an invocation the helper does not expect")

	require.NoError(t, os.WriteFile(fakeSSH, []byte("#!/bin/sh\necho '@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@' >&2\nexit 255\n"), 0o700))
	code = RunSSH(context.Background(), []string{"git@git.example.com", "git-upload-pack '/team/app.git'"}, o)
	assert.Equal(t, ExitHostKeyMismatch, code)
	assert.FileExists(t, filepath.Join(root, hostKeyMarker))
}

func TestAskpassAnswersOnlyUserAndPassword(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, CredentialFile), []byte("ghs_token\n"), 0o600))
	var out bytes.Buffer
	o := HelperOptions{Stdout: &out, SecretsDir: dir}
	require.Equal(t, ExitOK, RunAskpass([]string{"Username for 'https://git.example.com': "}, o))
	require.Equal(t, ExitOK, RunAskpass([]string{"Password for 'https://x-access-token@git.example.com': "}, o))
	assert.Equal(t, "x-access-token\nghs_token\n", out.String())
	assert.Equal(t, ExitFailed, RunAskpass([]string{"Passphrase for key"}, o))
	assert.Equal(t, ExitFailed, RunAskpass(nil, o))
}

func TestHostKeyMismatchIsReportedWithoutTheMarker(t *testing.T) {
	w := newWorld(t)
	script := "#!/bin/sh\nprintf 'Host key verification failed.\\n' >&2\nexit 255\n"
	require.NoError(t, os.WriteFile(w.ssh, []byte(script), 0o700))
	code, out := w.run(strings.Repeat("ab", 20), nil)
	assert.Equal(t, ExitHostKeyMismatch, code, out)
}

func TestRunSSHReportsAMismatchItCannotRecord(t *testing.T) {
	root := t.TempDir()
	fakeSSH := filepath.Join(root, "ssh")
	require.NoError(t, os.WriteFile(fakeSSH, []byte("#!/bin/sh\necho 'Host key verification failed.' >&2\nexit 255\n"), 0o700))
	env := map[string]string{envGitURL: repoURL, envMarkers: filepath.Join(root, "missing", "dir"), "HOME": root}
	var stderr bytes.Buffer
	o := HelperOptions{Getenv: func(k string) string { return env[k] }, Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &stderr, SSH: fakeSSH}
	code := RunSSH(context.Background(), []string{"git@git.example.com", "git-upload-pack '/team/app.git'"}, o)
	assert.Equal(t, ExitHostKeyMismatch, code)
	assert.Contains(t, stderr.String(), "HOST_KEY_MISMATCH: could not record it")
}
