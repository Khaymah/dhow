package leaksweep_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/leaksweep"
)

func deployKey(t *testing.T) ([]byte, []leaksweep.Needle) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(priv, "dhow")
	require.NoError(t, err)
	text := pem.EncodeToMemory(block)
	needles, err := leaksweep.PEMKey("deploy key", text)
	require.NoError(t, err)
	return text, needles
}

func write(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, data := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func names(hits []leaksweep.Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Name+" in "+filepath.Base(h.File))
	}
	return out
}

func TestAPEMLoggedAsBytesBySlogIsFound(t *testing.T) {
	text, needles := deployKey(t)
	dir := t.TempDir()
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("checkout", "credential", text)
	require.NotContains(t, log.String(), "PRIVATE KEY", "slog writes bytes as base64, so the PEM header is not there in clear")
	write(t, dir, "server.log", log.Bytes())
	write(t, dir, "clean.log", []byte("nothing here\n"))
	hits, stats, err := leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Files)
	assert.Contains(t, names(hits), "deploy key (PEM) in server.log")
}

func TestEveryFormOfAValueIsFound(t *testing.T) {
	value := []byte("s3cret-webhook-value-0123456789")
	n, err := leaksweep.Encoded("webhook secret", value)
	require.NoError(t, err)
	for i, form := range n.Forms {
		dir := t.TempDir()
		write(t, dir, "x.txt", append(append([]byte("prefix "), form...), " suffix"...))
		hits, _, err := leaksweep.Sweep([]string{dir}, []leaksweep.Needle{n}, nil, leaksweep.DefaultLimits())
		require.NoError(t, err)
		assert.Len(t, hits, 1, "form %d (%q)", i, form)
	}
}

func TestAKeyInsideAnArtifactIsFound(t *testing.T) {
	text, needles := deployKey(t)
	dir := t.TempDir()
	write(t, dir, "blobs/ab/abcdef", tarGz(t, map[string][]byte{"dist/app.txt": []byte("app 1\n"), "dist/.ssh/id": text}))
	write(t, dir, "blobs/cd/cdef01", tarGz(t, map[string][]byte{"dist/app.txt": []byte("app 2\n")}))
	hits, stats, err := leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, 2, stats.Archives)
	assert.Equal(t, 3, stats.Entries)
	require.NotEmpty(t, hits)
	for _, h := range hits {
		assert.True(t, strings.HasSuffix(h.File, "abcdef!dist/.ssh/id"), h.File)
	}
}

func TestAGzippedLogIsReadToo(t *testing.T) {
	text, needles := deployKey(t)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(text)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	dir := t.TempDir()
	write(t, dir, "old.log.gz", buf.Bytes())
	hits, _, err := leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	assert.NotEmpty(t, hits)
}

func TestArchivesPastTheLimitsFailTheSweep(t *testing.T) {
	_, needles := deployKey(t)
	dir := t.TempDir()
	write(t, dir, "many", tarGz(t, map[string][]byte{"a": []byte("1"), "b": []byte("2"), "c": []byte("3")}))
	_, _, err := leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.Limits{ArchiveEntries: 2, ArchiveBytes: 1 << 20})
	require.ErrorIs(t, err, leaksweep.ErrArchive)
	_, _, err = leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.Limits{ArchiveEntries: 10, ArchiveBytes: 512})
	require.ErrorIs(t, err, leaksweep.ErrArchive, "a bomb that unpacks past the byte budget stops the sweep")
}

func TestTheSweepFailsClosed(t *testing.T) {
	_, needles := deployKey(t)
	full := t.TempDir()
	write(t, full, "a.log", []byte("ok\n"))
	_, _, err := leaksweep.Sweep([]string{full, filepath.Join(full, "missing")}, needles, nil, leaksweep.DefaultLimits())
	require.Error(t, err, "a missing folder is an error, not a clean result")
	_, _, err = leaksweep.Sweep([]string{full, t.TempDir()}, needles, nil, leaksweep.DefaultLimits())
	require.ErrorIs(t, err, leaksweep.ErrNoFiles)
	_, _, err = leaksweep.Sweep([]string{write(t, full, "file", []byte("x"))}, needles, nil, leaksweep.DefaultLimits())
	require.Error(t, err, "a file where a folder should be is an error")
	_, err = leaksweep.Encoded("API token", nil)
	require.ErrorIs(t, err, leaksweep.ErrShortValue)
	_, err = leaksweep.Encoded("API token", []byte("short"))
	require.ErrorIs(t, err, leaksweep.ErrShortValue)
	_, _, err = leaksweep.Sweep([]string{full}, []leaksweep.Needle{{Name: "empty"}}, nil, leaksweep.DefaultLimits())
	require.ErrorIs(t, err, leaksweep.ErrShortValue)
}

func TestANeedleIsAllowedOnlyWhereItBelongs(t *testing.T) {
	n, err := leaksweep.Encoded("runner signing key", []byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	n.Allowed = []string{"runner-state/identity"}
	dir := t.TempDir()
	write(t, dir, "runner-state/identity", n.Forms[1])
	write(t, dir, "runner-state/journal/1.json", n.Forms[1])
	hits, _, err := leaksweep.Sweep([]string{dir}, []leaksweep.Needle{n}, nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, []string{"runner signing key in 1.json"}, names(hits))
}

func TestAPatternFindsATokenItWasNotGiven(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "runner.log", []byte(`{"msg":"renewed","token":"dhow_rat_Zq3k9T1Xv0aB8cD7eF6gH5iJ4kL3mN2o"}`))
	write(t, dir, "clean.log", []byte(`{"msg":"the access token starts with dhow_rat_"}`))
	p := leaksweep.Pattern{Name: "runner access token", Re: regexp.MustCompile(`dhow_rat_[A-Za-z0-9_-]{20,}`)}
	hits, _, err := leaksweep.Sweep([]string{dir}, nil, []leaksweep.Pattern{p}, leaksweep.DefaultLimits())
	require.NoError(t, err)
	assert.Equal(t, []string{"runner access token in runner.log"}, names(hits))
}
