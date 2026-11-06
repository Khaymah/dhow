package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	output "github.com/Khaymah/dhow/apps/cli/internal/Output"
	"github.com/Khaymah/dhow/internal/ingestpush"
)

const (
	pushSha   = "2222222222222222222222222222222222222222"
	pushToken = ingestpush.TokenPrefix + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcde"
)

type pushServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests int
	headers  http.Header
	reply    func(w http.ResponseWriter, digest string)
}

func newPushServer(t *testing.T, reply func(w http.ResponseWriter, digest string)) *pushServer {
	t.Helper()
	s := &pushServer{reply: reply}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		s.mu.Lock()
		s.requests++
		s.headers = r.Header.Clone()
		s.mu.Unlock()
		s.reply(w, filepath.Base(r.URL.Path))
	}))
	t.Cleanup(s.Close)
	return s
}

func created(w http.ResponseWriter, digest string) {
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"Success":true,"Data":{"Name":"web","Digest":"` + digest +
		`","SizeBytes":5,"Created":true,"Triggered":true,"RunId":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b","RunNumber":3,"RunUrl":"https://dhow.example.com/runs/3"}}`))
}

func refuse(status int, code string) func(http.ResponseWriter, string) {
	return func(w http.ResponseWriter, _ string) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"Success":false,"Error":{"Code":"` + code + `","Message":"x"}}`))
	}
}

func buildArchive(t *testing.T) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "index.html", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg}))
	_, err := tw.Write([]byte("hi"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	path := filepath.Join(t.TempDir(), "web.tar.gz")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	sum := sha256.Sum256(buf.Bytes())
	return path, hex.EncodeToString(sum[:])
}

func runPushCmd(args []string, env map[string]string, stdin string) (string, string, int) {
	var out, errOut bytes.Buffer
	code := Main(args, Env{Getenv: func(k string) string { return env[k] }, In: strings.NewReader(stdin), Out: &out, Err: &errOut})
	return out.String(), errOut.String(), code
}

func pushArgs(path string, extra ...string) []string {
	return append([]string{"artifact", "push", "--name", "web", "--ref", "main", "--sha", pushSha, "--retries", "0", "--lang", "en"}, append(extra, "--", path)...)
}

func TestAPushUploadsAndReportsTheRun(t *testing.T) {
	path, digest := buildArchive(t)
	srv := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	out, errOut, code := runPushCmd(pushArgs(path, "--server", srv.URL, "--provenance", "Provider=github-actions"), map[string]string{EnvIngestToken: pushToken}, "")
	assert.Equal(t, ExitValid, code, errOut)
	assert.Empty(t, errOut)
	assert.Contains(t, out, "Stored web as sha256:"+digest)
	assert.Contains(t, out, "Run #3 started: https://dhow.example.com/runs/3")
	assert.Equal(t, "Bearer "+pushToken, srv.headers.Get("Authorization"))
	assert.JSONEq(t, `{"Provider":"github-actions"}`, srv.headers.Get(ingestpush.HeaderProvenance))
	assert.NotContains(t, out+errOut, pushToken)
}

func TestTheJSONResultCarriesTheDigest(t *testing.T) {
	path, digest := buildArchive(t)
	srv := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	out, errOut, code := runPushCmd(pushArgs(path, "--format", "json"), map[string]string{EnvIngestToken: pushToken, EnvServer: srv.URL}, "")
	require.Equal(t, ExitValid, code, errOut)
	var got resultReport
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, digest, got.Digest)
	assert.True(t, got.Created)
	assert.Equal(t, "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", got.RunID)
	assert.Equal(t, 1, got.Attempts)
}

func TestTheTokenCanComeFromStandardInput(t *testing.T) {
	path, _ := buildArchive(t)
	srv := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	_, errOut, code := runPushCmd(pushArgs(path, "--server", srv.URL, "--token-stdin"), nil, pushToken+"\n")
	assert.Equal(t, ExitValid, code, errOut)
	assert.Equal(t, "Bearer "+pushToken, srv.headers.Get("Authorization"))

	_, errOut, code = runPushCmd(pushArgs(path, "--server", srv.URL, "--token-stdin"), map[string]string{EnvIngestToken: pushToken}, "")
	assert.Equal(t, ExitUsage, code, "stdin wins over the environment, and an empty stdin is an error")
	assert.Contains(t, errOut, "no token")
}

func TestATokenFlagIsRefusedWithoutEchoingIt(t *testing.T) {
	path, _ := buildArchive(t)
	srv := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	for _, args := range [][]string{
		pushArgs(path, "--server", srv.URL, "--token="+pushToken),
		pushArgs(path, "--server", srv.URL, "--token", pushToken),
		pushArgs(path, "--server", srv.URL, "-token="+pushToken),
	} {
		out, errOut, code := runPushCmd(args, nil, "")
		assert.Equal(t, ExitUsage, code)
		assert.Contains(t, errOut, "never read from a flag")
		assert.NotContains(t, out+errOut, pushToken)
	}
	assert.Zero(t, srv.requests)
}

func TestAnUnknownOptionDoesNotEchoItsValue(t *testing.T) {
	path, _ := buildArchive(t)
	_, errOut, code := runPushCmd(pushArgs(path, "--password=hunter2"), nil, "")
	assert.Equal(t, ExitUsage, code)
	assert.Contains(t, errOut, "--password is not an option")
	assert.NotContains(t, errOut, "hunter2")
}

func TestUsageErrors(t *testing.T) {
	path, _ := buildArchive(t)
	env := map[string]string{EnvIngestToken: pushToken}
	cases := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"no subcommand", []string{"artifact"}, nil, "dhow artifact push"},
		{"unknown subcommand", []string{"artifact", "pull"}, nil, "pull is not a command"},
		{"no path", []string{"artifact", "push", "--server", "https://x.example"}, env, "give one folder"},
		{"two paths", pushArgs(path, "--server", "https://x.example", "--", "a"), env, "give one folder"},
		{"no server", pushArgs(path), env, "--server is required"},
		{"no name", []string{"artifact", "push", "--server", "https://x.example", "--ref", "main", "--sha", pushSha, path}, env, "--name is required"},
		{"bad format", pushArgs(path, "--format", "xml"), env, "--format is text or json"},
		{"bad lang", pushArgs(path, "--lang", "fr"), env, "--lang is en or ar"},
		{"bad retries", pushArgs(path, "--retries", "11"), env, "--retries is a number"},
		{"missing value", []string{"artifact", "push", "--server"}, env, "--server needs a value"},
		{"bad provenance arg", pushArgs(path, "--provenance", "novalue"), env, "Key=Value"},
		{"no token", pushArgs(path, "--server", "https://x.example"), nil, "no token"},
		{"short token", pushArgs(path, "--server", "https://x.example"), map[string]string{EnvIngestToken: "dhow_ing_x"}, "not shaped like"},
		{"http server", pushArgs(path, "--server", "http://dhow.example.com"), env, "https://"},
		{"bad name", pushArgs(path, "--server", "https://x.example", "--name", "a b"), env, "--name must be"},
		{"bad sha", []string{"artifact", "push", "--server", "https://x.example", "--name", "web", "--ref", "main", "--sha", "xyz", path}, env, "--sha must be"},
		{"not gzip", pushArgs(writeFile(t, "plain.tar.gz", "hello"), "--server", "https://x.example"), env, "not a .tar.gz"},
		{"missing file", pushArgs(filepath.Join(t.TempDir(), "nope"), "--server", "https://x.example"), env, "cannot be read"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runPushCmd(tc.args, tc.env, "")
			assert.Equal(t, ExitUsage, code, out+errOut)
			assert.Contains(t, out+errOut, tc.want)
			assert.NotContains(t, out+errOut, pushToken)
		})
	}
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestEveryServerRefusalHasAnExitCodeAndAMessageInBothLanguages(t *testing.T) {
	path, _ := buildArchive(t)
	cases := []struct {
		status int
		code   string
		exit   int
		hint   string
	}{
		{401, "INGEST_TOKEN_INVALID", ExitToken, "not known to this server"},
		{401, "INGEST_TOKEN_REVOKED", ExitToken, "revoked"},
		{401, "INGEST_TOKEN_EXPIRED", ExitToken, "expired"},
		{403, "REF_NOT_ALLOWED_FOR_TOKEN", ExitToken, "ref patterns"},
		{422, "DIGEST_MISMATCH", ExitRefused, "do not match their digest"},
		{422, "ARCHIVE_UNSAFE", ExitRefused, "not safe to unpack"},
		{413, "ARTIFACT_TOO_LARGE", ExitRefused, "larger than this server accepts"},
		{413, "ARTIFACT_QUOTA_EXCEEDED", ExitRefused, "no room left"},
		{413, "INGEST_TOKEN_QUOTA", ExitRefused, "uploaded as much"},
		{411, "LENGTH_REQUIRED", ExitRefused, "size of the file"},
		{408, "UPLOAD_TOO_SLOW", ExitTemporary, "too slow"},
		{429, "RATE_LIMITED", ExitTemporary, "Too many uploads"},
		{503, "SERVICE_UNAVAILABLE", ExitTemporary, "busy"},
		{422, "VALIDATION_FAILED", ExitRefused, "one of the details"},
		{418, "TEAPOT", ExitRefused, "refused the upload"},
		{500, "BOOM", ExitTemporary, "could not take the upload"},
	}
	for _, tc := range cases {
		for _, lang := range []string{"en", "ar"} {
			t.Run(tc.code+"/"+lang, func(t *testing.T) {
				srv := newPushServer(t, refuse(tc.status, tc.code))
				out, errOut, code := runPushCmd(
					[]string{"artifact", "push", "--server", srv.URL, "--name", "web", "--ref", "main", "--sha", pushSha, "--retries", "0", "--lang", lang, path},
					map[string]string{EnvIngestToken: pushToken}, "")
				assert.Equal(t, tc.exit, code)
				assert.Empty(t, out)
				assert.Contains(t, errOut, "("+tc.code+")")
				assert.NotContains(t, errOut, pushToken)
				if lang == "en" {
					assert.Contains(t, errOut, tc.hint)
				} else {
					assert.NotContains(t, errOut, "The ", "the Arabic message must not fall back to English")
				}
			})
		}
	}
}

func TestATemporaryFailureIsRetriedAndTheUserSeesIt(t *testing.T) {
	path, _ := buildArchive(t)
	calls := 0
	srv := newPushServer(t, func(w http.ResponseWriter, d string) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			refuse(503, "SERVICE_UNAVAILABLE")(w, d)
			return
		}
		created(w, d)
	})
	out, errOut, code := runPushCmd(
		[]string{"artifact", "push", "--server", srv.URL, "--name", "web", "--ref", "main", "--sha", pushSha, "--retries", "2", "--lang", "en", path},
		map[string]string{EnvIngestToken: pushToken}, "")
	assert.Equal(t, ExitValid, code, errOut)
	assert.Contains(t, errOut, "attempt 1 of 3 failed")
	assert.Contains(t, out, "Stored web")
	assert.Equal(t, 2, srv.requests)
}

func TestADryRunPrintsTheDigestAndHeadersAndNeverTheToken(t *testing.T) {
	path, digest := buildArchive(t)
	srv := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	for _, format := range []string{"text", "json"} {
		out, errOut, code := runPushCmd(
			pushArgs(path, "--dry-run", "--server", srv.URL, "--format", format, "--provenance", "Provider=github-actions"),
			map[string]string{EnvIngestToken: pushToken}, pushToken)
		require.Equal(t, ExitValid, code, errOut)
		assert.Contains(t, out, digest)
		assert.Contains(t, out, "Dhow-Ingest-Sha")
		assert.Contains(t, out, "Dhow-Ingest-Provenance")
		assert.NotContains(t, out+errOut, pushToken)
	}
	assert.Zero(t, srv.requests, "a dry run sends nothing")

	out, _, code := runPushCmd(pushArgs(path, "--dry-run", "--format", "json"), nil, "")
	require.Equal(t, ExitValid, code, "a dry run needs neither a token nor a server")
	var got dryRunReport
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.True(t, got.DryRun)
	assert.Equal(t, digest, got.Digest)
	assert.Empty(t, got.URL)
	assert.Equal(t, hiddenAuthorization, got.Headers["Authorization"])
}

func TestAFolderIsPackedByAPush(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o600))
	one, _, code := runPushCmd(pushArgs(dir, "--dry-run", "--format", "json"), nil, "")
	require.Equal(t, ExitValid, code)
	two, _, _ := runPushCmd(pushArgs(dir, "--dry-run", "--format", "json"), nil, "")
	assert.Equal(t, one, two, "the same folder gives the same digest")
	var got dryRunReport
	require.NoError(t, json.Unmarshal([]byte(one), &got))
	assert.True(t, got.Packed)
}

func TestTheServerFromTheEnvironmentIsUsedAndAFlagWins(t *testing.T) {
	path, _ := buildArchive(t)
	good := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	bad := newPushServer(t, refuse(500, "BOOM"))
	_, errOut, code := runPushCmd(pushArgs(path, "--server", good.URL), map[string]string{EnvIngestToken: pushToken, EnvServer: bad.URL}, "")
	assert.Equal(t, ExitValid, code, errOut)
	assert.Zero(t, bad.requests)
}

func TestEveryPushMessageExistsInBothLanguages(t *testing.T) {
	codes := []string{
		"INGEST_TOKEN_INVALID", "INGEST_TOKEN_REVOKED", "INGEST_TOKEN_EXPIRED", "REF_NOT_ALLOWED_FOR_TOKEN", "DIGEST_MISMATCH", "ARCHIVE_UNSAFE",
		"ARTIFACT_TOO_LARGE", "ARTIFACT_QUOTA_EXCEEDED", "LENGTH_REQUIRED", "UPLOAD_TOO_SLOW", "INGEST_TOKEN_QUOTA", "RATE_LIMITED",
		"SERVICE_UNAVAILABLE", "VALIDATION_FAILED", "PROJECT_ARCHIVED", "NETWORK", "TIMEOUT", "REDIRECT", "BAD_RESPONSE",
		"RESPONSE_DIGEST_DIFFERS", "HTTP_4XX", "HTTP_5XX", "HTTP_UNEXPECTED", "HTTP_413", "TLS",
	}
	en, ar := output.NewCatalog("en"), output.NewCatalog("ar")
	for _, code := range codes {
		key := output.CodeKey(code)
		assert.NotEqual(t, string(key), en.T(key), code)
		assert.NotEqual(t, string(key), ar.T(key), code)
		assert.NotEqual(t, en.T(key), ar.T(key), code)
	}
}

func TestAFolderWithGitAtItsRootIsRefusedWithExitOne(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o600))
	for _, lang := range []string{"en", "ar"} {
		out, errOut, code := runPushCmd([]string{"artifact", "push", "--dry-run", "--lang", lang, "--name", "web", "--ref", "main", "--sha", pushSha, "--", dir}, nil, "")
		assert.Equal(t, ExitRefused, code, lang)
		assert.Empty(t, out)
		assert.Contains(t, errOut, "(PACK_HAS_GIT_DIR)")
	}
	out, _, code := runPushCmd([]string{"artifact", "push", "--dry-run", "--include-git", "--name", "web", "--ref", "main", "--sha", pushSha, "--", dir}, nil, "")
	assert.Equal(t, ExitValid, code)
	assert.Contains(t, out, "Archive: ")
}

func TestThePacksSizeIsPrinted(t *testing.T) {
	path, _ := buildArchive(t)
	srv := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	out, errOut, code := runPushCmd(pushArgs(path, "--server", srv.URL), map[string]string{EnvIngestToken: pushToken}, "")
	require.Equal(t, ExitValid, code, errOut)
	assert.Contains(t, out, "Archive: 1 entries, 2 bytes unpacked,")
}

func TestAPathShapedLikeATokenIsRefusedWithoutEchoingIt(t *testing.T) {
	for _, args := range [][]string{
		{"artifact", "push", "--dry-run", "--name", "web", "--ref", "main", "--sha", pushSha, "--", pushToken},
		{"artifact", "push", "--dry-run", "--name", "web", "--ref", "main", "--sha", pushSha, pushToken},
	} {
		out, errOut, code := runPushCmd(args, nil, "")
		assert.Equal(t, ExitUsage, code)
		assert.Contains(t, errOut, "looks like an ingest token")
		assert.NotContains(t, out+errOut, pushToken)
	}
}

func TestMaxBytesIsCheckedBeforeAnyUpload(t *testing.T) {
	path, _ := buildArchive(t)
	srv := newPushServer(t, func(w http.ResponseWriter, d string) { created(w, d) })
	_, errOut, code := runPushCmd(pushArgs(path, "--server", srv.URL, "--max-bytes", "10"), map[string]string{EnvIngestToken: pushToken}, "")
	assert.Equal(t, ExitRefused, code)
	assert.Contains(t, errOut, "ARTIFACT_TOO_LARGE")
	assert.Zero(t, srv.requests)
	_, errOut, code = runPushCmd(pushArgs(path, "--max-bytes", "0"), nil, "")
	assert.Equal(t, ExitUsage, code, errOut)
}
