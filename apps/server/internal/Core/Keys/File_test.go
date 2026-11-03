package keys_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	return dir
}

func writeKeyring(t *testing.T, dir string, ring *keys.Keyring) string {
	t.Helper()
	path := filepath.Join(dir, "dhow-keyring")
	require.NoError(t, keys.CreateKeyringFile(path, ring))
	return path
}

func TestReadKeyringFileAcceptsAPrivateFile(t *testing.T) {
	ring, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	path := writeKeyring(t, privateDir(t), ring)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	got, err := keys.ReadKeyringFile(path, keys.FileRules{})
	require.NoError(t, err)
	assert.Equal(t, ring.Marshal(), got.Marshal())

	require.NoError(t, os.Chmod(path, 0o400))
	_, err = keys.ReadKeyringFile(path, keys.FileRules{})
	require.NoError(t, err)
}

func TestReadKeyringFileRefusesUnsafeFiles(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	tests := []struct {
		name  string
		mode  os.FileMode
		rules keys.FileRules
		want  string
	}{
		{"readable by others", 0o604, keys.FileRules{}, "open to other users"},
		{"writable by others", 0o602, keys.FileRules{}, "open to other users"},
		{"readable by the group", 0o640, keys.FileRules{}, "open to other users"},
		{"writable by the group", 0o620, keys.FileRules{}, "open to other users"},
		{"group write even for a credential", 0o660, keys.FileRules{AllowGroupRead: true}, "open to other users"},
		{"others even for a credential", 0o444, keys.FileRules{AllowGroupRead: true}, "open to other users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeKeyring(t, privateDir(t), ring)
			require.NoError(t, os.Chmod(path, tt.mode))
			_, err := keys.ReadKeyringFile(path, tt.rules)
			require.Error(t, err)
			assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))
			assert.Contains(t, err.Error(), tt.want)
		})
	}
	t.Run("a systemd credential may show the group read bit of its ACL mask", func(t *testing.T) {
		path := writeKeyring(t, privateDir(t), ring)
		require.NoError(t, os.Chmod(path, 0o440))
		_, err := keys.ReadKeyringFile(path, keys.FileRules{AllowGroupRead: true})
		require.NoError(t, err)
	})
}

func TestReadKeyringFileRefusesLinksDirectoriesAndMissingFiles(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	dir := privateDir(t)
	target := writeKeyring(t, dir, ring)
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))

	_, err = keys.ReadKeyringFile(link, keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))
	assert.Contains(t, err.Error(), "symbolic link")

	_, err = keys.ReadKeyringFile(dir, keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))

	_, err = keys.ReadKeyringFile(filepath.Join(dir, "missing"), keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringMissing, keys.CodeOf(err))

	_, err = keys.ReadKeyringFile("", keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringMissing, keys.CodeOf(err))

	big := filepath.Join(dir, "big")
	require.NoError(t, os.WriteFile(big, []byte(strings.Repeat("x", keys.MaxKeyringBytes+1)), 0o600))
	_, err = keys.ReadKeyringFile(big, keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))

	bad := filepath.Join(dir, "bad")
	require.NoError(t, os.WriteFile(bad, []byte("kek 1 short\n"), 0o600))
	_, err = keys.ReadKeyringFile(bad, keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringMalformed, keys.CodeOf(err))
}

func TestReadKeyringFileRefusesAFileOfAnotherUser(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing the owner of a file needs root")
	}
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	path := writeKeyring(t, privateDir(t), ring)
	require.NoError(t, os.Chown(path, 12345, 12345))
	_, err = keys.ReadKeyringFile(path, keys.FileRules{})
	require.Error(t, err)
	assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))
	assert.Contains(t, err.Error(), "belongs to user 12345")
}

func TestCreateKeyringFileNeverReplacesAFile(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	dir := privateDir(t)
	path := writeKeyring(t, dir, ring)
	other, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	err = keys.CreateKeyringFile(path, other)
	require.ErrorIs(t, err, os.ErrExist)
	got, err := keys.ReadKeyringFile(path, keys.FileRules{})
	require.NoError(t, err)
	assert.Equal(t, ring.Marshal(), got.Marshal())

	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(filepath.Join(dir, "elsewhere"), link))
	require.Error(t, keys.CreateKeyringFile(link, other))
	_, err = os.Stat(filepath.Join(dir, "elsewhere"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestReplaceKeyringFileKeepsModeAndOwner(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	dir := privateDir(t)
	path := writeKeyring(t, dir, ring)
	next, _, err := ring.WithNextVersion()
	require.NoError(t, err)
	require.NoError(t, keys.ReplaceKeyringFile(path, next))

	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	st, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	assert.Equal(t, os.Geteuid(), int(st.Uid))
	got, err := keys.ReadKeyringFile(path, keys.FileRules{})
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 2}, got.Versions())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")

	require.NoError(t, os.Chmod(path, 0o644))
	err = keys.ReplaceKeyringFile(path, next)
	assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))
}
