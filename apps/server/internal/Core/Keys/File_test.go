package keys_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestAppendKeyringVersionKeepsModeOwnerAndEveryKey(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	dir := privateDir(t)
	path := writeKeyring(t, dir, ring)
	if os.Geteuid() != 0 {
		require.NoError(t, os.Chmod(path, 0o400))
		_, err = keys.AppendKeyringVersion(path, keys.FileRules{})
		require.Error(t, err, "a read-only file is not changed")
		require.NoError(t, os.Chmod(path, 0o600))
	}

	v, err := keys.AppendKeyringVersion(path, keys.FileRules{})
	require.NoError(t, err)
	assert.Equal(t, int32(2), v)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	st, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	assert.Equal(t, os.Geteuid(), int(st.Uid))
	got, err := keys.ReadKeyringFile(path, keys.FileRules{})
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 2}, got.Versions())
	assert.True(t, bytes.HasPrefix(got.Marshal(), ring.Marshal()), "version 1 is unchanged")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")

	noNewline := filepath.Join(dir, "no-newline")
	require.NoError(t, os.WriteFile(noNewline, bytes.TrimSuffix(ring.Marshal(), []byte("\n")), 0o600))
	_, err = keys.AppendKeyringVersion(noNewline, keys.FileRules{})
	require.NoError(t, err)
	fixed, err := keys.ReadKeyringFile(noNewline, keys.FileRules{})
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 2}, fixed.Versions())

	require.NoError(t, os.Chmod(path, 0o644))
	_, err = keys.AppendKeyringVersion(path, keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(path, link))
	_, err = keys.AppendKeyringVersion(link, keys.FileRules{})
	assert.Equal(t, keys.CodeKeyringInsecure, keys.CodeOf(err))
}

func TestConcurrentGenerateNeverCorruptsTheKeyring(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	path := writeKeyring(t, privateDir(t), ring)
	const runs = 200
	var wg sync.WaitGroup
	var mu sync.Mutex
	added := map[int32]int{}
	full := 0
	var other []error
	for range runs {
		wg.Go(func() {
			v, err := keys.AppendKeyringVersion(path, keys.FileRules{})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				added[v]++
			case keys.CodeOf(err) == keys.CodeKeyringFull:
				full++
			default:
				other = append(other, err)
			}
		})
	}
	wg.Wait()
	require.Empty(t, other)
	assert.Len(t, added, keys.MaxVersions-1, "each new version is handed out once")
	for v, n := range added {
		assert.Equal(t, 1, n, "version %d", v)
	}
	assert.Equal(t, runs-(keys.MaxVersions-1), full)
	got, err := keys.ReadKeyringFile(path, keys.FileRules{})
	require.NoError(t, err)
	assert.Len(t, got.Versions(), keys.MaxVersions)
	assert.True(t, bytes.HasPrefix(got.Marshal(), ring.Marshal()))
}

func TestAFailedGenerateLeavesTheKeyringAsItWas(t *testing.T) {
	errDisk := errors.New("disk full")
	tests := []struct {
		name    string
		writeAt func(f *os.File, b []byte, off int64) (int, error)
	}{
		{"a short write with an error", func(f *os.File, b []byte, off int64) (int, error) {
			n, _ := f.WriteAt(b[:len(b)/2], off)
			return n, errDisk
		}},
		{"a short write without an error", func(f *os.File, b []byte, off int64) (int, error) {
			n, err := f.WriteAt(b[:len(b)-1], off)
			return n, err
		}},
		{"a write that reads back wrong", func(f *os.File, b []byte, off int64) (int, error) {
			bad := bytes.Clone(b)
			bad[10] = '!'
			return f.WriteAt(bad, off)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ring, err := keys.GenerateKeyring(1, 2)
			require.NoError(t, err)
			path := writeKeyring(t, privateDir(t), ring)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = keys.AppendKeyringVersionWith(path, tt.writeAt)
			require.Error(t, err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after, "the file is truncated back to its size before the write")
			got, err := keys.ReadKeyringFile(path, keys.FileRules{})
			require.NoError(t, err)
			assert.Equal(t, []int32{1, 2}, got.Versions())
			v, err := keys.AppendKeyringVersion(path, keys.FileRules{})
			require.NoError(t, err, "the next generate works")
			assert.Equal(t, int32(3), v)
		})
	}
}

func TestACutOffLastLineNamesTheLineToDelete(t *testing.T) {
	ring, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	path := writeKeyring(t, privateDir(t), ring)
	data := ring.Marshal()
	require.NoError(t, os.WriteFile(path, append(bytes.Clone(data), []byte("kek 3 abc")...), 0o600))
	_, err = keys.AppendKeyringVersion(path, keys.FileRules{})
	require.Error(t, err)
	assert.Equal(t, keys.CodeKeyringMalformed, keys.CodeOf(err))
	assert.Contains(t, err.Error(), "delete line 3")
	_, err = keys.ReadKeyringFile(path, keys.FileRules{})
	assert.Contains(t, err.Error(), "delete line 3", "the server names the same line at boot")

	require.NoError(t, os.WriteFile(path, append([]byte("kek 1 abc\n"), data...), 0o600))
	_, err = keys.ReadKeyringFile(path, keys.FileRules{})
	assert.NotContains(t, err.Error(), "delete line", "only a cut-off last line is a leftover of generate")
}
