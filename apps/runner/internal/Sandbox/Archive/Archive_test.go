package archive_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	archive "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Archive"
)

func write(t *testing.T, dir, name, content string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), mode))
}

func workspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "dist/app.js", "console.log(1)\n", 0o644)
	write(t, dir, "dist/bin/run", "#!/bin/sh\n", 0o755)
	write(t, dir, "dist/z/deep.txt", "deep", 0o600)
	require.NoError(t, os.Symlink("app.js", filepath.Join(dir, "dist/latest.js")))
	write(t, dir, "notes.txt", "not packed", 0o644)
	return dir
}

func pack(t *testing.T, dir string, paths ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, archive.Pack(dir, paths, &buf))
	return buf.Bytes()
}

func TestPackIsDeterministicAcrossTimesOwnersAndOrder(t *testing.T) {
	a := workspace(t)
	b := workspace(t)
	later := time.Now().Add(72 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(b, "dist/app.js"), later, later))
	require.NoError(t, os.Chmod(filepath.Join(b, "dist/z/deep.txt"), 0o640))
	first := pack(t, a, "dist")
	second := pack(t, b, "dist/z", "dist", "dist/app.js")
	assert.Equal(t, sha256.Sum256(first), sha256.Sum256(second), "same files give the same digest")

	gz, err := gzip.NewReader(bytes.NewReader(first))
	require.NoError(t, err)
	assert.True(t, gz.ModTime.IsZero() || gz.ModTime.Unix() == 0)
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, h.Name)
		assert.Zero(t, h.Uid)
		assert.Zero(t, h.Gid)
		assert.Empty(t, h.Uname)
		assert.Equal(t, int64(0), h.ModTime.Unix())
	}
	assert.Equal(t, []string{"dist/", "dist/app.js", "dist/bin/", "dist/bin/run", "dist/latest.js", "dist/z/", "dist/z/deep.txt"}, names)
}

func TestPackAndUnpackRoundTrip(t *testing.T) {
	src := workspace(t)
	dst := t.TempDir()
	require.NoError(t, archive.Unpack(bytes.NewReader(pack(t, src, "dist")), dst, archive.Limits{}))
	got, err := os.ReadFile(filepath.Join(dst, "dist/app.js"))
	require.NoError(t, err)
	assert.Equal(t, "console.log(1)\n", string(got))
	info, err := os.Stat(filepath.Join(dst, "dist/bin/run"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	link, err := os.Readlink(filepath.Join(dst, "dist/latest.js"))
	require.NoError(t, err)
	assert.Equal(t, "app.js", link)
	_, err = os.Stat(filepath.Join(dst, "notes.txt"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestPackRefusesWhatCannotTravel(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ok.txt", "x", 0o644)
	require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(dir, "abs")))
	require.NoError(t, os.Symlink("../../outside", filepath.Join(dir, "up")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))
	for _, p := range []string{"abs", "up", "fifo"} {
		assert.ErrorIs(t, archive.Pack(dir, []string{p}, &bytes.Buffer{}), archive.ErrUnsupported, p)
	}
	assert.ErrorIs(t, archive.Pack(dir, []string{"missing"}, &bytes.Buffer{}), archive.ErrMissing)
	for _, p := range []string{"../x", "/etc", "~/x", ""} {
		assert.ErrorIs(t, archive.Pack(dir, []string{p}, &bytes.Buffer{}), archive.ErrPath, p)
	}
}

type tarEntry struct {
	name     string
	typeflag byte
	link     string
	body     string
}

func craft(t testing.TB, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typeflag, Linkname: e.link, Mode: 0o644, Size: int64(len(e.body)), Format: tar.FormatPAX}
		if e.typeflag != tar.TypeReg {
			h.Size = 0
		}
		require.NoError(t, tw.WriteHeader(h))
		if e.typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestUnpackRefusesEntriesThatLeaveTheWorkspace(t *testing.T) {
	cases := []struct {
		name    string
		entries []tarEntry
	}{
		{"dot dot", []tarEntry{{name: "../../etc/passwd", typeflag: tar.TypeReg, body: "x"}}},
		{"dot dot in the middle", []tarEntry{{name: "a/../../escape", typeflag: tar.TypeReg, body: "x"}}},
		{"absolute path", []tarEntry{{name: "/etc/passwd", typeflag: tar.TypeReg, body: "x"}}},
		{"absolute symlink", []tarEntry{{name: "l", typeflag: tar.TypeSymlink, link: "/etc"}}},
		{"symlink up", []tarEntry{{name: "a/l", typeflag: tar.TypeSymlink, link: "../../etc"}}},
		{"write through a symlink", []tarEntry{
			{name: "d/", typeflag: tar.TypeDir},
			{name: "l", typeflag: tar.TypeSymlink, link: "d"},
			{name: "l/x", typeflag: tar.TypeReg, body: "x"},
		}},
		{"character device", []tarEntry{{name: "tty", typeflag: tar.TypeChar}}},
		{"block device", []tarEntry{{name: "sda", typeflag: tar.TypeBlock}}},
		{"fifo", []tarEntry{{name: "pipe", typeflag: tar.TypeFifo}}},
		{"hard link out", []tarEntry{{name: "h", typeflag: tar.TypeLink, link: "../../etc/passwd"}}},
		{"hard link to nothing", []tarEntry{{name: "h", typeflag: tar.TypeLink, link: "missing"}}},
		{"hard link to a symlink", []tarEntry{
			{name: "s", typeflag: tar.TypeSymlink, link: "x"},
			{name: "h", typeflag: tar.TypeLink, link: "s"},
		}},
		{"file over a directory", []tarEntry{{name: "d/", typeflag: tar.TypeDir}, {name: "d", typeflag: tar.TypeReg, body: "x"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parent := t.TempDir()
			dst := filepath.Join(parent, "workspace")
			require.NoError(t, os.Mkdir(dst, 0o755))
			err := archive.Unpack(bytes.NewReader(craft(t, c.entries...)), dst, archive.Limits{})
			assert.ErrorIs(t, err, archive.ErrUnsafe)
			left, err := os.ReadDir(parent)
			require.NoError(t, err)
			assert.Len(t, left, 1, "nothing was written next to the workspace")
		})
	}
}

func TestUnpackAcceptsHardLinksInside(t *testing.T) {
	dst := t.TempDir()
	data := craft(t, tarEntry{name: "a.txt", typeflag: tar.TypeReg, body: "same"}, tarEntry{name: "b/c.txt", typeflag: tar.TypeLink, link: "a.txt"})
	require.NoError(t, archive.Unpack(bytes.NewReader(data), dst, archive.Limits{}))
	got, err := os.ReadFile(filepath.Join(dst, "b/c.txt"))
	require.NoError(t, err)
	assert.Equal(t, "same", string(got))
}

func TestUnpackStopsAtItsLimits(t *testing.T) {
	big := craft(t, tarEntry{name: "a", typeflag: tar.TypeReg, body: string(make([]byte, 4096))})
	assert.ErrorIs(t, archive.Unpack(bytes.NewReader(big), t.TempDir(), archive.Limits{MaxBytes: 1024}), archive.ErrTooLarge)
	many := craft(t, tarEntry{name: "a", typeflag: tar.TypeReg}, tarEntry{name: "b", typeflag: tar.TypeReg})
	assert.ErrorIs(t, archive.Unpack(bytes.NewReader(many), t.TempDir(), archive.Limits{MaxEntries: 1}), archive.ErrTooLarge)
	assert.ErrorIs(t, archive.Unpack(bytes.NewReader([]byte("not gzip")), t.TempDir(), archive.Limits{}), archive.ErrUnsafe)
}

func TestUnpackReplacesASymlinkInsteadOfWritingThroughIt(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "w")
	outside := filepath.Join(parent, "outside.txt")
	require.NoError(t, os.Mkdir(dst, 0o755))
	require.NoError(t, os.WriteFile(outside, []byte("keep"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dst, "f")))
	require.NoError(t, archive.Unpack(bytes.NewReader(craft(t, tarEntry{name: "f", typeflag: tar.TypeReg, body: "new"})), dst, archive.Limits{}))
	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(got))
}

func FuzzUnpack(f *testing.F) {
	f.Add(craft(f, tarEntry{name: "a/b", typeflag: tar.TypeReg, body: "x"}))
	f.Add(craft(f, tarEntry{name: "../x", typeflag: tar.TypeReg, body: "x"}))
	f.Add(craft(f, tarEntry{name: "l", typeflag: tar.TypeSymlink, link: "../.."}, tarEntry{name: "l/x", typeflag: tar.TypeReg}))
	f.Add([]byte{0x1f, 0x8b})
	f.Fuzz(func(t *testing.T, data []byte) {
		parent := t.TempDir()
		dst := filepath.Join(parent, "w")
		if err := os.Mkdir(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = archive.Unpack(bytes.NewReader(data), dst, archive.Limits{MaxEntries: 64, MaxBytes: 1 << 20})
		left, err := os.ReadDir(parent)
		if err != nil || len(left) != 1 {
			t.Fatalf("the archive wrote next to the workspace: %v", left)
		}
		err = filepath.WalkDir(dst, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(p)
				rel, _ := filepath.Rel(dst, p)
				if !archive.LinkStaysInside(filepath.ToSlash(rel), target) {
					t.Fatalf("a link leaves the workspace: %s -> %s", rel, target)
				}
			}
			if d.Type()&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice) != 0 {
				t.Fatalf("a special file was made: %s", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
