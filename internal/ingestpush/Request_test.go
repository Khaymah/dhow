package ingestpush_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/ingestpush"
)

func TestEndpointAcceptsOnlyHTTPSOrLoopbackHTTP(t *testing.T) {
	good := map[string]string{
		"https://dhow.example.com":         "https://dhow.example.com/ingest/v1/artifacts/d",
		"https://dhow.example.com/":        "https://dhow.example.com/ingest/v1/artifacts/d",
		"https://dhow.example.com/prefix/": "https://dhow.example.com/prefix/ingest/v1/artifacts/d",
		"http://127.0.0.1:8080":            "http://127.0.0.1:8080/ingest/v1/artifacts/d",
		"http://localhost:8080":            "http://localhost:8080/ingest/v1/artifacts/d",
		"http://[::1]:8080":                "http://[::1]:8080/ingest/v1/artifacts/d",
	}
	for in, want := range good {
		u, err := ingestpush.Endpoint(in, "d")
		require.NoError(t, err, in)
		assert.Equal(t, want, u.String())
	}
	for _, in := range []string{
		"", "dhow.example.com", "http://dhow.example.com", "ftp://dhow.example.com", "https://", "https://user:pass@dhow.example.com",
		"https://dhow.example.com?x=1", "https://dhow.example.com/#frag", "https://dhow.example.com?", "mailto:a@b.c", "http://127.0.0.1.evil.example",
	} {
		_, err := ingestpush.Endpoint(in, "d")
		assert.ErrorIs(t, err, ingestpush.ErrServer, in)
	}
}

func TestMetaValidationMatchesTheServerRules(t *testing.T) {
	ok := ingestpush.Meta{Name: "web", Ref: "main", Sha: testSha}
	require.NoError(t, ok.Validate())
	with := func(edit func(*ingestpush.Meta)) error {
		m := ok
		edit(&m)
		return m.Validate()
	}
	assert.NoError(t, with(func(m *ingestpush.Meta) { m.Ref = "refs/heads/release/1.0" }))
	assert.NoError(t, with(func(m *ingestpush.Meta) { m.Ref, m.RefKind = "v1.0.0", "tag" }))
	assert.NoError(t, with(func(m *ingestpush.Meta) { m.Sha = strings.Repeat("a", 64) }))
	assert.ErrorIs(t, with(func(m *ingestpush.Meta) { m.Name = "" }), ingestpush.ErrName)
	assert.ErrorIs(t, with(func(m *ingestpush.Meta) { m.Name = strings.Repeat("a", 65) }), ingestpush.ErrName)
	assert.ErrorIs(t, with(func(m *ingestpush.Meta) { m.Name = "-a" }), ingestpush.ErrName)
	assert.ErrorIs(t, with(func(m *ingestpush.Meta) { m.Ref = "" }), ingestpush.ErrRef)
	assert.ErrorIs(t, with(func(m *ingestpush.Meta) { m.Ref = "refs/tags/v1" }), ingestpush.ErrRef)
	assert.ErrorIs(t, with(func(m *ingestpush.Meta) { m.RefKind = "x" }), ingestpush.ErrRefKind)
	assert.ErrorIs(t, with(func(m *ingestpush.Meta) { m.Sha = strings.Repeat("a", 39) }), ingestpush.ErrSha)
}

func TestProvenanceIsStringsOnlyAndBounded(t *testing.T) {
	raw, err := ingestpush.ProvenanceJSON(map[string]string{"Repository": "Khaymah/dhow", "RunId": "9"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"Repository":"Khaymah/dhow","RunId":"9"}`, raw)
	empty, err := ingestpush.ProvenanceJSON(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	many := map[string]string{}
	for i := range 17 {
		many["K"+strings.Repeat("a", i)] = "v"
	}
	bad := []map[string]string{
		many,
		{"1bad": "v"},
		{"has space": "v"},
		{"K": strings.Repeat("v", 257)},
		{"K": "line\nbreak"},
		{"K": "bell\x07"},
		{"K": "zero\u200bwidth"},
		{"K": "\xff"},
		{strings.Repeat("k", 33): "v"},
	}
	for _, p := range bad {
		_, err := ingestpush.ProvenanceJSON(p)
		assert.ErrorIs(t, err, ingestpush.ErrProvenance, "%v", p)
	}
}

func TestHeadersCarryOnlyWhatTheServerReads(t *testing.T) {
	m := ingestpush.Meta{Name: "web", Ref: "main", Sha: testSha}
	h, err := m.Headers(123)
	require.NoError(t, err)
	assert.Equal(t, "123", h.Get("Content-Length"))
	assert.Equal(t, "branch", h.Get(ingestpush.HeaderRefKind))
	assert.Empty(t, h.Get(ingestpush.HeaderTrigger))
	assert.Empty(t, h.Get(ingestpush.HeaderProvenance))
	assert.Empty(t, h.Get("Authorization"))
}

func TestPrepareHashesAFileAndPacksAFolderTheSameWay(t *testing.T) {
	path, digest := writeArchive(t, map[string]string{"a.txt": "a", "dir/b.txt": "b"})
	pkg, err := ingestpush.Prepare(path)
	require.NoError(t, err)
	defer pkg.Close()
	assert.Equal(t, digest, pkg.Digest)
	assert.False(t, pkg.Packed)
	assert.Positive(t, pkg.Size)
	assert.Equal(t, 2, pkg.Entries)
	assert.FileExists(t, pkg.Path)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("y"), 0o600))
	first, err := ingestpush.Prepare(dir)
	require.NoError(t, err)
	second, err := ingestpush.Prepare(dir)
	require.NoError(t, err)
	assert.True(t, first.Packed)
	assert.Equal(t, first.Digest, second.Digest, "the pack is reproducible")
	tmp := first.Path
	first.Close()
	second.Close()
	assert.NoFileExists(t, tmp, "the temporary archive is removed")
}

func TestPrepareRefusesWhatTheServerWouldRefuse(t *testing.T) {
	dir := t.TempDir()
	notGzip := filepath.Join(dir, "plain.tar.gz")
	require.NoError(t, os.WriteFile(notGzip, []byte("hello world"), 0o600))
	_, err := ingestpush.Prepare(notGzip)
	assert.ErrorIs(t, err, ingestpush.ErrNotGzip)

	empty := filepath.Join(dir, "empty.tar.gz")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	_, err = ingestpush.Prepare(empty)
	assert.ErrorIs(t, err, ingestpush.ErrEmpty)

	_, err = ingestpush.Prepare(t.TempDir())
	assert.ErrorIs(t, err, ingestpush.ErrEmpty)

	_, err = ingestpush.Prepare(filepath.Join(dir, "missing"))
	assert.ErrorIs(t, err, ingestpush.ErrSource)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}))
	_, err = tw.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	unsafe := filepath.Join(dir, "unsafe.tar.gz")
	require.NoError(t, os.WriteFile(unsafe, buf.Bytes(), 0o600))
	_, err = ingestpush.Prepare(unsafe)
	var pe *ingestpush.Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, ingestpush.CodeArchiveUnsafe, pe.Code)
	assert.Equal(t, ingestpush.ClassInput, pe.Class)

	escape := t.TempDir()
	require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(escape, "link")))
	_, err = ingestpush.Prepare(escape)
	assert.ErrorIs(t, err, ingestpush.ErrSource)
}

func TestTheTokenShapeIsChecked(t *testing.T) {
	assert.True(t, ingestpush.ValidToken(testToken))
	for _, bad := range []string{"", testToken + "x", testToken[:len(testToken)-1], testToken + "\n", " " + testToken, "dhow_ing_" + strings.Repeat("!", 43)} {
		assert.False(t, ingestpush.ValidToken(bad), bad)
	}
}
