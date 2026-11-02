package gitsource

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sha1 = "0123456789abcdef0123456789abcdef01234567"

func TestFetchArgsPutTheAddressAndRefAfterDoubleDash(t *testing.T) {
	u, err := Parse("git@git.example.com:team/app.git")
	require.NoError(t, err)
	args, err := FetchArgs(u, "refs/heads/main", 1)
	require.NoError(t, err)
	assert.Equal(t, append(SafeConfig(), "fetch", "--quiet", "--depth=1", "--no-tags", "--", "ssh://git@git.example.com/team/app.git", "refs/heads/main"), args)
	assert.Contains(t, strings.Join(args, " "), "protocol.allow=never")
}

func TestFetchArgsRefuseBadInput(t *testing.T) {
	good, err := Parse("https://git.example.com/app.git")
	require.NoError(t, err)
	_, err = FetchArgs(good, "--upload-pack=touch", 1)
	require.Error(t, err)
	_, err = FetchArgs(good, "refs/heads/-oProxyCommand", 1)
	require.Error(t, err)
	_, err = FetchArgs(good, "refs/heads/main@{1}", 1)
	require.Error(t, err)
	_, err = FetchArgs(good, "refs/heads/main", 0)
	require.ErrorIs(t, err, ErrDepth)
	_, err = FetchArgs(URL{Transport: "ext", Host: "x", Path: "/x"}, "refs/heads/main", 1)
	require.ErrorIs(t, err, ErrNotFetched)
	_, err = FetchArgs(URL{Transport: TransportSSH, Host: "-oProxyCommand=x", Path: "/x"}, "refs/heads/main", 1)
	require.Error(t, err)
}

func TestFileArgsPutThePathAfterDoubleDash(t *testing.T) {
	args, err := ListFileArgs(sha1, "-x.yml")
	require.NoError(t, err)
	assert.Equal(t, []string{"ls-tree", "-z", "--full-tree", sha1, "--", "-x.yml"}, args[len(args)-6:])
	_, err = ListFileArgs("--output=/tmp/x", ".dhow.yml")
	require.Error(t, err)
	for _, bad := range []string{"", "/etc/passwd", "../x.yml", "a/../../x", "a//b", "a\nb", "a\x00b"} {
		_, err = ListFileArgs(sha1, bad)
		require.ErrorIs(t, err, ErrFilePath, bad)
	}
	blob, err := ReadBlobArgs(sha1)
	require.NoError(t, err)
	assert.Equal(t, []string{"cat-file", "blob", sha1}, blob[len(blob)-3:])
	_, err = ReadBlobArgs("--batch")
	require.ErrorIs(t, err, ErrObjectID)
}

func TestBlobFromListing(t *testing.T) {
	line := []byte("100644 blob " + sha1 + "\t-x.yml\x00")
	id, ok := BlobFromListing(line, "-x.yml")
	require.True(t, ok)
	assert.Equal(t, sha1, id)
	_, ok = BlobFromListing(line, "other.yml")
	assert.False(t, ok)
	_, ok = BlobFromListing([]byte("040000 tree "+sha1+"\t-x.yml\x00"), "-x.yml")
	assert.False(t, ok)
	_, ok = BlobFromListing(nil, "-x.yml")
	assert.False(t, ok)
}

func runGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + dir, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "PATH=" + os.Getenv("PATH")}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestAPipelinePathThatStartsWithADashIsReadAsAPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		out, err := runGit(t, dir, args...)
		require.NoError(t, err, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "-x.yml"), []byte("version: 1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dhow.yml"), []byte("other\n"), 0o600))
	out, err := runGit(t, dir, "add", "--", "-x.yml", ".dhow.yml")
	require.NoError(t, err, out)
	out, err = runGit(t, dir, "commit", "-q", "-m", "init")
	require.NoError(t, err, out)
	sha, err := runGit(t, dir, "rev-parse", "HEAD")
	require.NoError(t, err, sha)
	sha = strings.TrimSpace(sha)

	unsafe, err := runGit(t, dir, "ls-tree", "-z", "--full-tree", sha, "-x.yml")
	require.Error(t, err, "without -- git reads the path as an option: %s", unsafe)

	list, err := ListFileArgs(sha, "-x.yml")
	require.NoError(t, err)
	listing, err := runGit(t, dir, list...)
	require.NoError(t, err, listing)
	blob, ok := BlobFromListing([]byte(listing), "-x.yml")
	require.True(t, ok, listing)
	read, err := ReadBlobArgs(blob)
	require.NoError(t, err)
	content, err := runGit(t, dir, read...)
	require.NoError(t, err, content)
	assert.Equal(t, "version: 1\n", content)
}
