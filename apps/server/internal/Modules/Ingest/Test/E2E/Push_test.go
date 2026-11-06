package e2e_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	"github.com/Khaymah/dhow/internal/ingestpush"
)

func (w world) pushOptions(serverURL string) ingestpush.Options {
	return ingestpush.Options{
		Server: serverURL, Token: w.token.Value, Retries: 0,
		Meta: ingestpush.Meta{Name: "web", Ref: "main", Sha: shaNext},
	}
}

func (w world) writeArchive(t *testing.T) (string, string) {
	t.Helper()
	body := w.archive()
	path := filepath.Join(t.TempDir(), "web.tar.gz")
	require.NoError(t, os.WriteFile(path, body, 0o600))
	return path, apptest.DigestHex(body)
}

func pushFailure(t *testing.T, err error) *ingestpush.Error {
	t.Helper()
	var e *ingestpush.Error
	require.True(t, errors.As(err, &e), "%v", err)
	return e
}

func TestTheRealClientUploadsAndTheServerRecordsTheSameDigest(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	path, digest := w.writeArchive(t)
	o := w.pushOptions(srv.URL)
	o.Meta.Provenance = map[string]string{"Provider": "github-actions", "Repository": "Khaymah/dhow", "RunId": "42"}

	res, err := ingestpush.Push(context.Background(), o, path)
	require.NoError(t, err)
	assert.Equal(t, digest, res.Digest)
	assert.True(t, res.Created)
	assert.True(t, res.Triggered)
	assert.NotEmpty(t, res.RunID)
	assert.True(t, w.env.BlobOnDisk(digest))
	assert.Equal(t, 1, w.runsOfKind(t, "artifact"))

	var stored, provenance string
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT encode("Digest", 'hex'), "Provenance"::text FROM "IngestedArtifacts" WHERE "ProjectId" = $1`, w.project).Scan(&stored, &provenance))
	assert.Equal(t, digest, stored, "the digest the client printed is the one the server recorded")
	assert.JSONEq(t, `{"Provider":"github-actions","Repository":"Khaymah/dhow","RunId":"42"}`, provenance)

	again, err := ingestpush.Push(context.Background(), o, path)
	require.NoError(t, err)
	assert.False(t, again.Created, "a repeated push is a 200")
	assert.Equal(t, res.RunID, again.RunID, "and it does not start a second run")
	assert.Equal(t, 1, w.runsOfKind(t, "artifact"))
}

func TestTheRealClientPacksAFolderTheServerAccepts(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("1"), 0o600))

	res, err := ingestpush.Push(context.Background(), w.pushOptions(srv.URL), dir)
	require.NoError(t, err)
	assert.True(t, res.Created)
	assert.True(t, w.env.BlobOnDisk(res.Digest))
}

func TestTheRealClientCanStoreWithoutStartingARun(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	path, _ := w.writeArchive(t)
	o := w.pushOptions(srv.URL)
	o.Meta.NoTrigger = true

	res, err := ingestpush.Push(context.Background(), o, path)
	require.NoError(t, err)
	assert.True(t, res.Created)
	assert.False(t, res.Triggered)
	assert.Zero(t, w.runsOfKind(t, "artifact"))
}

func TestTheRealServerRefusalsReachTheClientWithTheirCodes(t *testing.T) {
	w := newWorldWith(t, func(c *config.Config) { c.Artifacts.IngestMaxBytes = 2 << 20 })
	srv := w.env.Serve()
	path, _ := w.writeArchive(t)

	o := w.pushOptions(srv.URL)
	o.Meta.Ref = "feature/x"
	_, err := ingestpush.Push(context.Background(), o, path)
	e := pushFailure(t, err)
	assert.Equal(t, ingestpush.CodeRefNotAllowed, e.Code)
	assert.Equal(t, ingestpush.ClassToken, e.Class)

	o = w.pushOptions(srv.URL)
	o.Token = ingestpush.TokenPrefix + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcde"
	_, err = ingestpush.Push(context.Background(), o, path)
	e = pushFailure(t, err)
	assert.Equal(t, ingestpush.CodeIngestTokenInvalid, e.Code)

	revoked := w.env.SeedIngestToken(w.project, w.ownerID, apptest.IngestSeed{Revoked: true})
	o = w.pushOptions(srv.URL)
	o.Token = revoked.Value
	_, err = ingestpush.Push(context.Background(), o, path)
	assert.Equal(t, ingestpush.CodeIngestTokenRevoked, pushFailure(t, err).Code)

	big := filepath.Join(t.TempDir(), "big.tar.gz")
	require.NoError(t, os.WriteFile(big, apptest.TarGz(map[string]string{"blob": string(randomBytes(t, 3<<20))}), 0o600))
	_, err = ingestpush.Push(context.Background(), w.pushOptions(srv.URL), big)
	e = pushFailure(t, err)
	assert.Equal(t, ingestpush.CodeArtifactTooLarge, e.Code)
	assert.Equal(t, ingestpush.ClassRefused, e.Class)
	assert.Zero(t, w.env.IngestedCount(w.project))
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	out := make([]byte, n)
	_, err := rand.Read(out)
	require.NoError(t, err)
	return out
}
