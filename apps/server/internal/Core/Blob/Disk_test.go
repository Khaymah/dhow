package blob_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	return dir
}

func store(t *testing.T, free int64) (*blob.Disk, string) {
	t.Helper()
	dir := privateDir(t)
	d, err := blob.NewDisk(blob.DiskOptions{
		Dir: dir, Reserve: 1 << 10, CheckEvery: 1 << 10,
		FreeSpace: func(string) (int64, error) { return free, nil },
	})
	require.NoError(t, err)
	return d, dir
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return err
	}))
	return out
}

func put(data []byte, size, maxBytes int64) blob.Put {
	return blob.Put{Digest: sha256.Sum256(data), Size: size, MaxBytes: maxBytes, Body: bytes.NewReader(data)}
}

func TestABlobIsStoredUnderItsDigestOnlyAfterItIsCommitted(t *testing.T) {
	d, dir := store(t, 1<<30)
	data := []byte("archive bytes")
	p, err := d.Stage(context.Background(), put(data, int64(len(data)), 1<<20))
	require.NoError(t, err)
	staged := files(t, dir)
	require.Len(t, staged, 1)
	assert.True(t, strings.Contains(staged[0], ".upload-"), "an upload is a temporary file until it is committed")
	stored, err := p.Commit()
	require.NoError(t, err)
	digest := blob.Digest(sha256.Sum256(data))
	assert.Equal(t, digest.Key(), stored.Key)
	assert.Equal(t, []string{filepath.FromSlash(digest.Key())}, files(t, dir))
	r, err := d.Open(context.Background(), digest)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	assert.EqualValues(t, len(data), r.Size())
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(digest.Key())))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestAStoreRefusesWhatDoesNotMatchAndLeavesNothing(t *testing.T) {
	data := []byte("archive bytes")
	cases := map[string]struct {
		p    blob.Put
		want error
		free int64
	}{
		"a wrong digest":             {blob.Put{Digest: sha256.Sum256([]byte("other")), Size: int64(len(data)), MaxBytes: 1 << 20, Body: bytes.NewReader(data)}, blob.ErrDigestMismatch, 1 << 30},
		"a shorter body":             {put(data, int64(len(data))+5, 1<<20), blob.ErrSizeMismatch, 1 << 30},
		"a declared size over limit": {put(data, 1<<21, 1<<20), blob.ErrTooLarge, 1 << 30},
		"an unknown size over limit": {put(bytes.Repeat([]byte("x"), 4096), -1, 1024), blob.ErrTooLarge, 1 << 30},
		"not enough room up front":   {put(data, int64(len(data)), 1<<20), blob.ErrNoSpace, 1 << 10},
		"room that runs out":         {put(bytes.Repeat([]byte("y"), 8192), -1, 1<<20), blob.ErrNoSpace, 1 << 9},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d, dir := store(t, c.free)
			_, err := d.Stage(context.Background(), c.p)
			assert.ErrorIs(t, err, c.want)
			assert.Empty(t, files(t, dir), "nothing is left behind")
		})
	}
}

func TestAnAbortedUploadLeavesNothing(t *testing.T) {
	d, dir := store(t, 1<<30)
	data := []byte("abandoned")
	p, err := d.Stage(context.Background(), put(data, -1, 1<<20))
	require.NoError(t, err)
	p.Abort()
	assert.Empty(t, files(t, dir))
	_, err = p.Commit()
	assert.Error(t, err, "an aborted upload cannot be committed")
}

func TestTheStoreRefusesAnOpenDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o755))
	_, err := blob.NewDisk(blob.DiskOptions{Dir: dir})
	assert.ErrorIs(t, err, blob.ErrDirUnsafe)
	_, err = blob.NewDisk(blob.DiskOptions{Dir: filepath.Join(dir, "missing")})
	assert.ErrorIs(t, err, blob.ErrDirUnsafe, "a missing directory stops the start")
	_, err = blob.NewDisk(blob.DiskOptions{Dir: "relative/blobs"})
	assert.ErrorIs(t, err, blob.ErrDirUnsafe)
	link := filepath.Join(privateDir(t), "link")
	require.NoError(t, os.Symlink(privateDir(t), link))
	_, err = blob.NewDisk(blob.DiskOptions{Dir: link})
	assert.ErrorIs(t, err, blob.ErrDirUnsafe, "a symlink is not the directory itself")
	created := filepath.Join(privateDir(t), "dev", "blobs")
	_, err = blob.NewDisk(blob.DiskOptions{Dir: created, Create: true})
	require.NoError(t, err, "development creates its directory")
}

func TestSweepTempRemovesOnlyOldUploads(t *testing.T) {
	d, dir := store(t, 1<<30)
	data := []byte("keep me")
	p, err := d.Stage(context.Background(), put(data, -1, 1<<20))
	require.NoError(t, err)
	_, err = p.Commit()
	require.NoError(t, err)
	shard := filepath.Join(dir, "ab")
	require.NoError(t, os.Mkdir(shard, 0o700))
	old := filepath.Join(shard, ".upload-old")
	require.NoError(t, os.WriteFile(old, []byte("x"), 0o600))
	past := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(old, past, past))
	require.NoError(t, os.WriteFile(filepath.Join(shard, ".upload-new"), []byte("x"), 0o600))
	n, err := d.SweepTemp(context.Background(), time.Now().Add(-24*time.Hour).UnixNano())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Len(t, files(t, dir), 2, "the committed blob and the recent upload stay")
}

func TestDigestsAreLowercaseHexOnly(t *testing.T) {
	for _, bad := range []string{"", "abc", strings.Repeat("A", 64), strings.Repeat("g", 64), "../" + strings.Repeat("a", 61), strings.Repeat("a", 65)} {
		_, err := blob.ParseDigest(bad)
		assert.ErrorIs(t, err, blob.ErrDigestInvalid, bad)
	}
	d, err := blob.ParseDigest(strings.Repeat("ab", 32))
	require.NoError(t, err)
	assert.Equal(t, "ab/"+strings.Repeat("ab", 32), d.Key())
}

func FuzzParseDigest(f *testing.F) {
	f.Add(strings.Repeat("0", 64))
	f.Add("../../etc/passwd")
	f.Add(strings.Repeat("F", 64))
	f.Fuzz(func(t *testing.T, s string) {
		d, err := blob.ParseDigest(s)
		if err != nil {
			return
		}
		if d.String() != s || d.Key() != s[:2]+"/"+s || strings.Contains(d.Key(), "..") {
			t.Fatalf("a parsed digest does not round trip to a key inside its shard: %q -> %q", s, d.Key())
		}
	})
}
