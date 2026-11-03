package app_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func keyringFile(t *testing.T, ring *keys.Keyring) string {
	t.Helper()
	path := filepath.Join(apptest.PrivateDir(t), "dhow-keyring")
	require.NoError(t, keys.CreateKeyringFile(path, ring))
	return path
}

func keyMaterial(rings ...*keys.Keyring) []string {
	var out []string
	for _, ring := range rings {
		for _, l := range strings.Split(strings.TrimSpace(string(ring.Marshal())), "\n") {
			encoded := l[strings.LastIndexByte(l, ' ')+1:]
			raw, err := base64.RawURLEncoding.DecodeString(encoded)
			if err != nil {
				continue
			}
			out = append(out, encoded, encoded[:20], hex.EncodeToString(raw), base64.StdEncoding.EncodeToString(raw), string(raw),
				hex.EncodeToString(keys.CheckValueOf(raw)), base64.StdEncoding.EncodeToString(keys.CheckValueOf(raw)))
		}
	}
	return out
}

func assertNoKeys(t *testing.T, text string, rings ...*keys.Keyring) {
	t.Helper()
	for _, m := range keyMaterial(rings...) {
		require.NotContains(t, text, m, "key material leaked")
	}
}

func schemaURL(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var schema string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&schema))
	sep := "?"
	if strings.Contains(config.TestDatabaseURL(), "?") {
		sep = "&"
	}
	return config.TestDatabaseURL() + sep + "search_path=" + schema
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	saved := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		data, _ := io.ReadAll(r)
		done <- data
	}()
	defer func() {
		os.Stdout = saved
	}()
	fn()
	require.NoError(t, w.Close())
	os.Stdout = saved
	return string(<-done)
}

func TestServerStopsAtBootWhenTheKeyringDoesNotFit(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	provider, err := keys.NewFileProvider(ring)
	require.NoError(t, err)
	_, err = keys.NewManager(pool, provider, nil).Register(t.Context())
	require.NoError(t, err)
	other, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	withoutTwo := keyringFile(t, mustParse(t, firstLine(ring.Marshal())))

	malformed := filepath.Join(apptest.PrivateDir(t), "dhow-keyring")
	require.NoError(t, os.WriteFile(malformed, append(ring.Marshal(), []byte("kek 3 not-a-key\n")...), 0o600))
	shared := keyringFile(t, ring)
	require.NoError(t, os.Chmod(shared, 0o644))

	tests := []struct {
		name, path, code string
		version          int32
	}{
		{"a version the database needs is missing", withoutTwo, keys.CodeKeyringMissingVersion, 2},
		{"another key under the same versions", keyringFile(t, other), keys.CodeKeyringMismatch, 1},
		{"a malformed line", malformed, keys.CodeKeyringMalformed, 0},
		{"open to other users", shared, keys.CodeKeyringInsecure, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(config.EnvDev, "true")
			t.Setenv(config.EnvDatabaseURL, schemaURL(t, pool))
			t.Setenv(config.EnvSecretKey, "9f1c4a7e2b8d06f35a1e9c7b4d2f80a6c3e5b7091d4f6a8e2c0b5d7f9a1e3c6b")
			t.Setenv(config.EnvPublicURL, "http://localhost:58290")
			t.Setenv(config.EnvListenAddr, "127.0.0.1:58290")
			t.Setenv(config.EnvKeyringFile, tt.path)
			var runErr error
			logs := captureStdout(t, func() { runErr = app.Run(context.Background()) })
			require.Error(t, runErr)
			assert.Equal(t, tt.code, keys.CodeOf(runErr))
			var ke *keys.Error
			require.ErrorAs(t, runErr, &ke)
			assert.Equal(t, 79, ke.ExitCode())
			assert.Equal(t, tt.version, ke.Version)
			if tt.version > 0 {
				assert.Contains(t, runErr.Error(), fmt.Sprintf("KEK version %d", tt.version))
			}
			assert.Contains(t, logs, tt.code)
			assert.NotContains(t, logs, "listening")
			assertNoKeys(t, logs+runErr.Error(), ring, other)
		})
	}
}

func firstLine(data []byte) []byte {
	i := bytes.IndexByte(data, '\n')
	return data[:i+1]
}

func mustParse(t *testing.T, data []byte) *keys.Keyring {
	t.Helper()
	ring, err := keys.ParseKeyring(data)
	require.NoError(t, err)
	return ring
}

func TestReadinessReportsTheKeyring(t *testing.T) {
	env := apptest.New(t)
	res := env.Do(apptest.Request{Method: http.MethodGet, Path: "/health/ready"})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Contains(t, string(res.Raw), `{"Name":"keyring","Ok":true}`)

	_, err := env.Pool.Exec(t.Context(), `UPDATE "KeyEncryptionKeys" SET "CheckValue" = sha256('another keyring'::bytea)`)
	require.NoError(t, err)
	res = env.Do(apptest.Request{Method: http.MethodGet, Path: "/health/ready"})
	require.Equal(t, http.StatusServiceUnavailable, res.Status, string(res.Raw))
	assert.Contains(t, string(res.Raw), `{"Name":"keyring","Ok":false}`)
	assert.NotContains(t, string(res.Raw), keys.CodeKeyringMismatch, "the public probe names the check, never the reason")
	assert.Contains(t, env.Logs.String(), keys.CodeKeyringMismatch)
}

func TestKeyringStatusShowsStateAndNeverAKey(t *testing.T) {
	env := apptest.New(t)
	owner, _ := env.SetupAndLogin()
	res := owner.Get("/system/keys")
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var st struct {
		Provider       string
		ActiveVersion  int32
		ToRewrap       int64
		LoadedVersions []int32
		Versions       []struct {
			Version  int32
			State    string
			DataKeys int64
			Loaded   bool
		}
	}
	res.Decode(t, &st)
	assert.Equal(t, "fake", st.Provider)
	assert.Equal(t, int32(1), st.ActiveVersion)
	assert.Equal(t, []int32{1}, st.LoadedVersions)
	require.Len(t, st.Versions, 1)
	assert.Equal(t, "active", st.Versions[0].State)
	assert.True(t, st.Versions[0].Loaded)
	for _, field := range []string{"CheckValue", "Wrapped", "Key\"", "Dek", "Kek\""} {
		assert.NotContains(t, string(res.Raw), field)
	}
	assertNoKeys(t, string(res.Raw)+env.Logs.String(), apptest.TestKeyring(t))
}
