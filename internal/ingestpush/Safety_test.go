package ingestpush_test

import (
	"context"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/ingestpush"
)

type fixedResolver struct{ addrs []netip.Addr }

func (r fixedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addrs, nil
}

func TestAFolderWithGitIsRefusedUnlessAskedFor(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[remote]"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o600))

	_, err := ingestpush.Prepare(dir, ingestpush.PrepareOptions{})
	e := pushError(t, err)
	assert.Equal(t, ingestpush.CodePackHasGit, e.Code)
	assert.Equal(t, ingestpush.ClassRefused, e.Class)

	pkg, err := ingestpush.Prepare(dir, ingestpush.PrepareOptions{IncludeGit: true})
	require.NoError(t, err)
	defer pkg.Close()
	assert.Positive(t, pkg.Entries)
	assert.Positive(t, pkg.Unpacked)

	nested := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(nested, "sub", ".git"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "sub", ".git", "x"), []byte("x"), 0o600))
	other, err := ingestpush.Prepare(nested, ingestpush.PrepareOptions{})
	require.NoError(t, err, "only an entry at the root of the folder is refused")
	other.Close()
}

func TestThePreflightUsesTheServerSizeLimits(t *testing.T) {
	path, _ := writeArchive(t, map[string]string{"zeros": string(make([]byte, 20000))})
	info, err := os.Stat(path)
	require.NoError(t, err)

	_, err = ingestpush.Prepare(path, ingestpush.PrepareOptions{MaxBytes: info.Size() - 1})
	assert.Equal(t, ingestpush.CodeArtifactTooLarge, pushError(t, err).Code)

	_, err = ingestpush.Prepare(path, ingestpush.PrepareOptions{MaxBytes: 1000})
	assert.Equal(t, ingestpush.CodeArtifactTooLarge, pushError(t, err).Code, "20000 unpacked bytes exceed 8 times 1000")

	pkg, err := ingestpush.Prepare(path, ingestpush.PrepareOptions{})
	require.NoError(t, err)
	pkg.Close()
}

func TestPlainHTTPMustResolveToLoopbackOnly(t *testing.T) {
	path, digest := writeArchive(t, map[string]string{"a": "b"})
	srv := newFake(t, stored(digest, true))
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	o := options("http://localhost:" + port)
	o.Retries = 0

	o.Resolver = fixedResolver{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.5")}}
	_, err = ingestpush.Push(context.Background(), o, path)
	e := pushError(t, err)
	assert.Equal(t, ingestpush.ClassInput, e.Class)
	assert.ErrorIs(t, e, ingestpush.ErrNotLoopback)
	assert.Empty(t, srv.requests())

	o.Resolver = fixedResolver{addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("198.51.100.1")}}
	_, err = ingestpush.Push(context.Background(), o, path)
	assert.ErrorIs(t, err, ingestpush.ErrNotLoopback, "one public address among the answers is enough to refuse")

	o.Resolver = fixedResolver{addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	_, err = ingestpush.Push(context.Background(), o, path)
	require.NoError(t, err)
}
