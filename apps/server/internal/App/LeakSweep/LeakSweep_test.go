//go:build leaksweep

package leaksweep_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
)

const (
	envDatabase = "DHOW_LEAK_SWEEP_DATABASE_URL"
	envKeyFile  = "DHOW_LEAK_SWEEP_SECRET_KEY_FILE"
	envNeedles  = "DHOW_LEAK_SWEEP_NEEDLES"
	envDirs     = "DHOW_LEAK_SWEEP_DIRS"
	chunkBytes  = 4 << 20
	minNeedle   = 12
)

type needle struct {
	name  string
	forms [][]byte
}

type hit struct {
	needle string
	file   string
}

func required(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	require.NotEmpty(t, v, "%s is required", name)
	return v
}

func encodedForms(name string, secret []byte) needle {
	n := needle{name: name, forms: [][]byte{secret, []byte(hex.EncodeToString(secret)), []byte(strings.ToUpper(hex.EncodeToString(secret)))}}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		for shift := range 3 {
			text := enc.EncodeToString(append(make([]byte, shift), secret...))
			start := 0
			if shift > 0 {
				start = 4
			}
			if end := len(text) - 4; end-start >= minNeedle {
				n.forms = append(n.forms, []byte(text[start:end]))
			}
		}
	}
	return n
}

func deployKeyNeedles(t *testing.T, label string, pemText []byte) []needle {
	t.Helper()
	out := []needle{{name: label + " (PEM)", forms: [][]byte{pemText, bytes.ReplaceAll(pemText, []byte("\n"), []byte(`\n`))}}}
	raw, err := ssh.ParseRawPrivateKey(pemText)
	require.NoError(t, err, "%s does not parse as a private key", label)
	key, ok := raw.(*ed25519.PrivateKey)
	require.True(t, ok, "%s is not an Ed25519 key", label)
	return append(out, encodedForms(label+" (private seed)", key.Seed()))
}

func sealedNeedles(t *testing.T, ctx context.Context, dsn string, secret []byte) []needle {
	t.Helper()
	deploy, err := appcrypto.NewSealer(secret, config.DeployKeySealLabel)
	require.NoError(t, err)
	webhooks, err := appcrypto.NewSealer(secret, config.WebhookDeliveryKeyLabel)
	require.NoError(t, err)
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	rows, err := conn.Query(ctx, `SELECT "Id", "DeployKeySealed", "PreviousDeployKeySealed", "WebhookSecretSealed" FROM "Repositories"`)
	require.NoError(t, err)
	defer rows.Close()
	var out []needle
	for rows.Next() {
		var id uuid.UUID
		var key, previous, hook []byte
		require.NoError(t, rows.Scan(&id, &key, &previous, &hook))
		for kind, sealed := range map[string][]byte{"deploy key": key, "previous deploy key": previous} {
			if len(sealed) == 0 {
				continue
			}
			plain, err := deploy.Open(sealed, []byte("deploy-key:"+id.String()))
			require.NoError(t, err, "the %s of repository %s does not open with this server key", kind, id)
			out = append(out, deployKeyNeedles(t, kind+" of repository "+id.String(), plain)...)
		}
		if len(hook) > 0 {
			plain, err := webhooks.Open(hook, []byte("webhook-secret:"+id.String()))
			require.NoError(t, err, "the webhook secret of repository %s does not open with this server key", id)
			out = append(out, encodedForms("webhook secret of repository "+id.String(), plain))
		}
	}
	require.NoError(t, rows.Err())
	return out
}

func listedNeedles(t *testing.T, path string) []needle {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var out []needle
	s := bufio.NewScanner(f)
	for s.Scan() {
		name, value, ok := strings.Cut(s.Text(), "=")
		if !ok || value == "" {
			continue
		}
		require.GreaterOrEqual(t, len(value), minNeedle, "the %s is too short to look for", name)
		out = append(out, encodedForms(name, []byte(value)))
	}
	require.NoError(t, s.Err())
	return out
}

func longest(needles []needle) int {
	most := 0
	for _, n := range needles {
		for _, f := range n.forms {
			most = max(most, len(f))
		}
	}
	return most
}

func scanFile(path string, needles []needle, overlap int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	found := map[string]bool{}
	buf := make([]byte, 0, chunkBytes+overlap)
	chunk := make([]byte, chunkBytes)
	for {
		n, err := f.Read(chunk)
		buf = append(buf, chunk[:n]...)
		for _, nd := range needles {
			for _, form := range nd.forms {
				if bytes.Contains(buf, form) {
					found[nd.name] = true
				}
			}
		}
		if len(buf) > overlap {
			buf = append(buf[:0], buf[len(buf)-overlap:]...)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	return names, nil
}

func sweep(t *testing.T, dirs []string, needles []needle) ([]hit, int) {
	t.Helper()
	overlap := longest(needles)
	var hits []hit
	files := 0
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				t.Logf("skipped %s: %v", path, err)
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			names, err := scanFile(path, needles, overlap)
			if err != nil {
				t.Logf("skipped %s: %v", path, err)
				return nil
			}
			files++
			for _, name := range names {
				hits = append(hits, hit{needle: name, file: path})
			}
			return nil
		})
		require.NoError(t, err)
	}
	return hits, files
}

func serverKey(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(required(t, envKeyFile))
	require.NoError(t, err)
	text := string(bytes.TrimSpace(raw))
	if key, err := hex.DecodeString(text); err == nil {
		return key
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if key, err := enc.DecodeString(text); err == nil {
			return key
		}
	}
	require.FailNow(t, "the server key is neither hex nor base64")
	return nil
}

func TestNoCredentialIsStoredInClear(t *testing.T) {
	ctx := context.Background()
	secret := serverKey(t)
	needles := sealedNeedles(t, ctx, required(t, envDatabase), secret)
	require.NotEmpty(t, needles, "no repository with a sealed credential was found")
	needles = append(needles, encodedForms("server secret key", secret))
	needles = append(needles, listedNeedles(t, required(t, envNeedles))...)
	dirs := strings.Split(required(t, envDirs), ":")
	hits, files := sweep(t, dirs, needles)
	names := make([]string, 0, len(needles))
	for _, n := range needles {
		names = append(names, n.name)
	}
	sort.Strings(names)
	t.Logf("looked for %d credentials in %d files under %s", len(needles), files, strings.Join(dirs, ", "))
	for _, name := range names {
		t.Logf("  %s", name)
	}
	require.Positive(t, files, "there was nothing to sweep")
	for _, h := range hits {
		t.Errorf("found the %s in clear in %s", h.needle, h.file)
	}
}
