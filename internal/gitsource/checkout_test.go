package gitsource

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gitWorld struct {
	t       *testing.T
	root    string
	origin  string
	work    string
	home    string
	ssh     string
	sshLog  string
	marker  string
	planted string
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func newGitWorld(t *testing.T) *gitWorld {
	t.Helper()
	needGit(t)
	root := t.TempDir()
	w := &gitWorld{
		t: t, root: root, origin: filepath.Join(root, "srv", "team", "app.git"), work: filepath.Join(root, "work"),
		home: filepath.Join(root, "home"), ssh: filepath.Join(root, "bin", "git-ssh"), sshLog: filepath.Join(root, "ssh.log"),
		marker: filepath.Join(root, "PWNED"), planted: filepath.Join(root, "planted"),
	}
	for _, d := range []string{filepath.Dir(w.origin), w.work, w.home, filepath.Dir(w.ssh)} {
		require.NoError(t, os.MkdirAll(d, 0o700))
	}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + w.sshLog + "\n" +
		"env | grep -E '^(GIT_SSH_COMMAND|GIT_DIR|SSH_AUTH_SOCK)=' >> " + w.sshLog + "\n" +
		"for last; do :; done\n" +
		"path=$(printf '%s' \"$last\" | sed -e \"s/^git-upload-pack '//\" -e \"s/'\\$//\")\n" +
		"exec git-upload-pack \"" + filepath.Join(root, "srv") + "$path\"\n"
	require.NoError(t, os.WriteFile(w.ssh, []byte(script), 0o700))
	w.plain("init", "--quiet", "--bare", "-b", "main", w.origin)
	seed := filepath.Join(root, "seed")
	w.plain("init", "--quiet", "-b", "main", seed)
	w.commitFile(seed, "README", "one\n")
	w.plain("-C", seed, "push", "--quiet", w.origin, "main")
	return w
}

func (w *gitWorld) plain(args ...string) string {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = []string{
		"HOME=" + w.home, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com",
	}
	out, err := cmd.CombinedOutput()
	require.NoError(w.t, err, string(out))
	return strings.TrimSpace(string(out))
}

func (w *gitWorld) commitFile(repo, name, body string) string {
	w.t.Helper()
	require.NoError(w.t, os.WriteFile(filepath.Join(repo, name), []byte(body), 0o600))
	w.plain("-C", repo, "add", "--", name)
	w.plain("-C", repo, "commit", "--quiet", "-m", name)
	return w.plain("-C", repo, "rev-parse", "HEAD")
}

func (w *gitWorld) safe(args []string) (string, error) {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = w.work
	cmd.Env = append(Env(EnvOptions{Home: w.home}), "PATH="+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (w *gitWorld) url() URL {
	return URL{Transport: TransportSSH, User: "git", Host: "git.example.com", Path: "/team/app.git"}
}

func (w *gitWorld) config() []string {
	cfg, err := CheckoutConfig(w.ssh)
	require.NoError(w.t, err)
	return cfg
}

func (w *gitWorld) plant() {
	w.t.Helper()
	w.plain("init", "--quiet", "--bare", w.planted)
	w.t.Setenv("GIT_SSH_COMMAND", "touch "+w.marker)
	w.t.Setenv("GIT_SSH", "/bin/false")
	w.t.Setenv("GIT_DIR", w.planted)
	w.t.Setenv("GIT_CONFIG_PARAMETERS", "'core.hooksPath'='/tmp'")
	w.t.Setenv("GIT_CONFIG_COUNT", "1")
	w.t.Setenv("GIT_CONFIG_KEY_0", "core.sshCommand")
	w.t.Setenv("GIT_CONFIG_VALUE_0", "touch "+w.marker)
}

func (w *gitWorld) checkout(want string) (string, error) {
	w.t.Helper()
	cfg := w.config()
	if out, err := w.safe(InitArgs(cfg)); err != nil {
		return out, err
	}
	fetch, err := FetchArgsWith(cfg, w.url(), want, 1)
	require.NoError(w.t, err)
	if out, err := w.safe(fetch); err != nil {
		return out, err
	}
	head, err := w.safe(FetchHeadArgs(cfg))
	if err != nil {
		return head, err
	}
	co, err := CheckoutArgs(cfg, strings.TrimSpace(head))
	require.NoError(w.t, err)
	return w.safe(co)
}

func TestCheckoutIgnoresPlantedGitEnvironment(t *testing.T) {
	w := newGitWorld(t)
	w.plant()
	out, err := w.checkout("refs/heads/main")
	require.NoError(t, err, out)
	assert.NoFileExists(t, w.marker, "a planted GIT_SSH_COMMAND or GIT_CONFIG_* ran")
	assert.FileExists(t, filepath.Join(w.work, "README"))
	assert.DirExists(t, filepath.Join(w.work, ".git"), "a planted GIT_DIR redirected the repository")
	assert.NoFileExists(t, filepath.Join(w.planted, "FETCH_HEAD"), "the fetch wrote into the planted GIT_DIR")
	log, err := os.ReadFile(w.sshLog)
	require.NoError(t, err)
	assert.Contains(t, string(log), "-o SendEnv=GIT_PROTOCOL git@git.example.com git-upload-pack '/team/app.git'")
	assert.NotContains(t, string(log), "GIT_SSH_COMMAND=")
	assert.NotContains(t, string(log), "GIT_DIR=")
	cmd, err := ParseSSHInvocation(w.url(), []string{"-o", "SendEnv=GIT_PROTOCOL", "git@git.example.com", "git-upload-pack '/team/app.git'"})
	require.NoError(t, err)
	assert.Equal(t, UploadPackCommand(w.url()), cmd)
}

func TestRepositoryConfigCannotRunCommands(t *testing.T) {
	w := newGitWorld(t)
	cfg := w.config()
	out, err := w.safe(InitArgs(cfg))
	require.NoError(t, err, out)
	for _, kv := range [][2]string{
		{"core.sshCommand", "touch " + w.marker},
		{"core.fsmonitor", "touch " + w.marker},
		{"core.hooksPath", filepath.Join(w.root, "hooks")},
	} {
		w.plain("-C", w.work, "config", kv[0], kv[1])
	}
	hook := filepath.Join(w.root, "hooks", "post-checkout")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o700))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+w.marker+"\n"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(w.work, ".git", "hooks"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(w.work, ".git", "hooks", "post-checkout"), []byte("#!/bin/sh\ntouch "+w.marker+"\n"), 0o700))
	out, err = w.checkout("refs/heads/main")
	require.NoError(t, err, out)
	assert.NoFileExists(t, w.marker, "a command from the repository config or a hook ran")
	assert.FileExists(t, filepath.Join(w.work, "README"))
}

func TestLiteralPathspecsTurnOffMagic(t *testing.T) {
	w := newGitWorld(t)
	out, err := w.checkout("refs/heads/main")
	require.NoError(t, err, out)
	head, err := w.safe(FetchHeadArgs(SafeConfig()))
	require.NoError(t, err, head)
	list, err := ListFileArgs(strings.TrimSpace(head), ":(glob)*")
	require.NoError(t, err)
	listing, err := w.safe(list)
	require.NoError(t, err, listing)
	assert.Empty(t, listing, "the pathspec was read as glob magic")
	_, ok := BlobFromListing([]byte(listing), "README")
	assert.False(t, ok)
}

func TestSubmodulesAreNotFetched(t *testing.T) {
	w := newGitWorld(t)
	inner := filepath.Join(w.root, "inner")
	w.plain("init", "--quiet", "-b", "main", inner)
	innerSha := w.commitFile(inner, "secret", "x\n")
	seed := filepath.Join(w.root, "seed")
	modules := "[submodule \"sub\"]\n\tpath = sub\n\turl = file://" + inner + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(seed, ".gitmodules"), []byte(modules), 0o600))
	w.plain("-C", seed, "add", "--", ".gitmodules")
	w.plain("-C", seed, "update-index", "--add", "--cacheinfo", "160000,"+innerSha+",sub")
	w.plain("-C", seed, "commit", "--quiet", "-m", "submodule")
	w.plain("-C", seed, "push", "--quiet", w.origin, "main")
	out, err := w.checkout("refs/heads/main")
	require.NoError(t, err, out)
	assert.FileExists(t, filepath.Join(w.work, ".gitmodules"))
	assert.NoFileExists(t, filepath.Join(w.work, "sub", "secret"), "the file:// submodule was fetched")
}

func TestFetchByShaAndAncestry(t *testing.T) {
	w := newGitWorld(t)
	seed := filepath.Join(w.root, "seed")
	old := w.plain("-C", seed, "rev-parse", "HEAD")
	w.commitFile(seed, "second", "two\n")
	w.plain("-C", seed, "push", "--quiet", w.origin, "main")
	cfg := w.config()
	out, err := w.safe(InitArgs(cfg))
	require.NoError(t, err, out)
	fetch, err := FetchArgsWith(cfg, w.url(), "refs/heads/main", 2)
	require.NoError(t, err)
	out, err = w.safe(fetch)
	require.NoError(t, err, out)
	anc, err := IsAncestorArgs(cfg, old)
	require.NoError(t, err)
	out, err = w.safe(anc)
	require.NoError(t, err, out)
	missing := strings.Repeat("ab", 20)
	anc, err = IsAncestorArgs(cfg, missing)
	require.NoError(t, err)
	_, err = w.safe(anc)
	require.Error(t, err)
}

func TestCheckoutConfigRefusesShellText(t *testing.T) {
	for _, bad := range []string{"", "ssh", "/bin/sh -c x", "/x;y", "/x/../y", "/x$(y)"} {
		_, err := CheckoutConfig(bad)
		assert.ErrorIs(t, err, ErrSSHCommand, bad)
	}
	cfg := SafeConfig()
	joined := strings.Join(cfg, " ")
	for _, want := range []string{"--literal-pathspecs", "core.sshCommand=/dev/null", "core.fsmonitor=false", "core.hooksPath=/dev/null", "protocol.allow=never"} {
		assert.Contains(t, joined, want)
	}
	assert.Equal(t, "--literal-pathspecs", cfg[0])
}

func TestEnvIsExplicit(t *testing.T) {
	env := Env(EnvOptions{Home: "/tmp/h", Askpass: "/run/dhow/bin/git-askpass"})
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		assert.NotContains(t, []string{"GIT_SSH_COMMAND", "GIT_SSH", "GIT_DIR", "GIT_CONFIG_PARAMETERS", "SSH_AUTH_SOCK"}, name)
	}
	assert.Contains(t, env, "GIT_CONFIG_NOSYSTEM=1")
	assert.Contains(t, env, "GIT_TERMINAL_PROMPT=0")
	assert.Contains(t, env, "HOME=/tmp/h")
	assert.Contains(t, env, "PATH="+FixedPath)
}

func TestFetchArgsWithChecksTheWant(t *testing.T) {
	u, err := Parse("https://git.example.com/app.git")
	require.NoError(t, err)
	for _, bad := range []string{"--upload-pack=x", "main", "refs/heads/x..y", strings.Repeat("A", 40), "-" + strings.Repeat("a", 39)} {
		_, err := FetchArgsWith(SafeConfig(), u, bad, 1)
		assert.Error(t, err, bad)
	}
	args, err := FetchArgsWith(SafeConfig(), u, strings.Repeat("a", 40), 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"--", u.String(), strings.Repeat("a", 40)}, args[len(args)-3:])
}

func knownHostKey(typ string) string {
	blob := binary.BigEndian.AppendUint32(nil, uint32(len(typ)))
	blob = append(blob, typ...)
	blob = append(blob, make([]byte, 32)...)
	return base64.StdEncoding.EncodeToString(blob)
}

func TestCheckKnownHost(t *testing.T) {
	u, err := Parse("ssh://git@git.example.com:2222/app.git")
	require.NoError(t, err)
	good := "[git.example.com]:2222 ssh-ed25519 " + knownHostKey("ssh-ed25519")
	require.NoError(t, CheckKnownHost(u, good))
	for _, bad := range []string{
		"",
		"git.example.com ssh-ed25519 " + knownHostKey("ssh-ed25519"),
		"[git.example.com]:2222 ssh-dss " + knownHostKey("ssh-dss"),
		"[git.example.com]:2222 ssh-ed25519 " + knownHostKey("ssh-rsa"),
		"[git.example.com]:2222 ssh-ed25519 !!!",
		good + "\n* ssh-ed25519 " + knownHostKey("ssh-ed25519"),
		"@cert-authority * ssh-ed25519 " + knownHostKey("ssh-ed25519"),
	} {
		assert.ErrorIs(t, CheckKnownHost(u, bad), ErrKnownHost, bad)
	}
	https, err := Parse("https://git.example.com/app.git")
	require.NoError(t, err)
	assert.ErrorIs(t, CheckKnownHost(https, good), ErrNotSSH)
}

func TestSSHArgs(t *testing.T) {
	u, err := Parse("ssh://git@git.example.com:2222/team/app.git")
	require.NoError(t, err)
	o := SSHOptions{Binary: "/usr/bin/ssh", KeyFile: "/run/dhow/secrets/key", KnownHostsFile: "/run/dhow/secrets/known_hosts", ProxyCommand: "/run/dhow/bin/connect"}
	args, err := SSHArgs(u, o, UploadPackCommand(u))
	require.NoError(t, err)
	joined := strings.Join(args, " ")
	assert.True(t, strings.HasPrefix(joined, "/usr/bin/ssh -F /dev/null -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/run/dhow/secrets/known_hosts"), joined)
	for _, want := range []string{"IdentitiesOnly=yes", "IdentityFile=/run/dhow/secrets/key", "GlobalKnownHostsFile=/dev/null", "BatchMode=yes", "ProxyCommand=/run/dhow/bin/connect %h %p", "-p 2222", "-l git"} {
		assert.Contains(t, joined, want)
	}
	assert.Equal(t, []string{"--", "git.example.com", "git-upload-pack '/team/app.git'"}, args[len(args)-3:])
	_, err = SSHArgs(u, o, "git-receive-pack '/team/app.git'")
	require.ErrorIs(t, err, ErrSSHUse)
	bad := o
	bad.KeyFile = "/x key"
	_, err = SSHArgs(u, bad, UploadPackCommand(u))
	require.ErrorIs(t, err, ErrSSHCommand)
}

func TestParseSSHInvocation(t *testing.T) {
	u, err := Parse("ssh://git@git.example.com:2222/team/app.git")
	require.NoError(t, err)
	cmd := "git-upload-pack '/team/app.git'"
	got, err := ParseSSHInvocation(u, []string{"-o", "SendEnv=GIT_PROTOCOL", "-p", "2222", "git@git.example.com", cmd})
	require.NoError(t, err)
	assert.Equal(t, cmd, got)
	for _, bad := range [][]string{
		{"-p", "22", "git@git.example.com", cmd},
		{"-o", "ProxyCommand=sh", "git@git.example.com", cmd},
		{"-p", "2222", "git@evil.example.com", cmd},
		{"-p", "2222", "git@git.example.com", "git-receive-pack '/team/app.git'"},
		{"-p", "2222", "git@git.example.com", "git-upload-pack '/other.git'"},
		{"-p", "2222", "git@git.example.com", cmd, "extra"},
		{"-p", "2222", "-oProxyCommand=x", "git@git.example.com", cmd},
	} {
		_, err := ParseSSHInvocation(u, bad)
		assert.ErrorIs(t, err, ErrSSHUse, strings.Join(bad, " "))
	}
}
