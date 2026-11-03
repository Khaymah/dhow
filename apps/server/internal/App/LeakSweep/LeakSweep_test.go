//go:build leaksweep

package leaksweep_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	"github.com/Khaymah/dhow/internal/leaksweep"
)

const (
	envDatabase = "DHOW_LEAK_SWEEP_DATABASE_URL"
	envKeyFile  = "DHOW_LEAK_SWEEP_SECRET_KEY_FILE"
	envNeedles  = "DHOW_LEAK_SWEEP_NEEDLES"
	envDirs     = "DHOW_LEAK_SWEEP_DIRS"
	envIdentity = "DHOW_LEAK_SWEEP_RUNNER_IDENTITY"
	tokenChars  = `[A-Za-z0-9_-]`
)

var (
	identityFiles = []string{"runner-state/identity", "runner-state/identity.pending"}
	knownSealed   = map[string]bool{
		`"Repositories"."DeployKeySealed"`: true, `"Repositories"."PreviousDeployKeySealed"`: true,
		`"Repositories"."WebhookSecretSealed"`: true,
	}
)

func required(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	require.NotEmpty(t, v, "%s is required", name)
	return v
}

type checked struct{ t *testing.T }

func (c checked) needle(n leaksweep.Needle, err error) leaksweep.Needle {
	c.t.Helper()
	require.NoError(c.t, err)
	return n
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

func sealedNeedles(t *testing.T, conn *pgx.Conn, secret []byte) []leaksweep.Needle {
	t.Helper()
	ctx := context.Background()
	deploy, err := appcrypto.NewSealer(secret, config.DeployKeySealLabel)
	require.NoError(t, err)
	webhooks, err := appcrypto.NewSealer(secret, config.WebhookDeliveryKeyLabel)
	require.NoError(t, err)
	rows, err := conn.Query(ctx, `SELECT "Id", "DeployKeySealed", "PreviousDeployKeySealed", "WebhookSecretSealed" FROM "Repositories"`)
	require.NoError(t, err)
	defer rows.Close()
	var out []leaksweep.Needle
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
			keys, err := leaksweep.PEMKey(kind+" of repository "+id.String(), plain)
			require.NoError(t, err)
			out = append(out, keys...)
		}
		if len(hook) > 0 {
			plain, err := webhooks.Open(hook, []byte("webhook-secret:"+id.String()))
			require.NoError(t, err, "the webhook secret of repository %s does not open with this server key", id)
			out = append(out, checked{t}.needle(leaksweep.Encoded("webhook secret of repository "+id.String(), plain)))
		}
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, out, "no repository with a sealed credential was found")
	return out
}

func uncoveredSealed(t *testing.T, conn *pgx.Conn) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(), `SELECT quote_ident(table_name) || '.' || quote_ident(column_name),
		(xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM %I WHERE %I IS NOT NULL', table_name, column_name),
			false, true, '')))[1]::text::bigint
		FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name LIKE '%Sealed' AND data_type = 'bytea' ORDER BY 1`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	seen := 0
	for rows.Next() {
		var name string
		var n int64
		require.NoError(t, rows.Scan(&name, &n))
		seen++
		if n > 0 && !knownSealed[name] {
			out = append(out, name)
		}
	}
	require.NoError(t, rows.Err())
	require.Positive(t, seen, "no sealed column was found")
	return out
}

func listedNeedles(t *testing.T, path string) []leaksweep.Needle {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var out []leaksweep.Needle
	s := bufio.NewScanner(f)
	for s.Scan() {
		name, value, ok := strings.Cut(s.Text(), "=")
		require.True(t, ok, "a needle line is name=value")
		out = append(out, checked{t}.needle(leaksweep.Encoded(name, []byte(value))))
	}
	require.NoError(t, s.Err())
	require.NotEmpty(t, out)
	return out
}

func runnerKeys(t *testing.T, path string) []leaksweep.Needle {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var id struct {
		SigningSeed   []byte
		EncryptionKey []byte
	}
	require.NoError(t, json.Unmarshal(raw, &id))
	out := []leaksweep.Needle{
		checked{t}.needle(leaksweep.Encoded("runner signing key", id.SigningSeed)),
		checked{t}.needle(leaksweep.Encoded("runner encryption key", id.EncryptionKey)),
	}
	for i := range out {
		out[i].Allowed = identityFiles
	}
	return out
}

func patterns() []leaksweep.Pattern {
	return []leaksweep.Pattern{
		{Name: "API token", Re: regexp.MustCompile(config.APITokenPrefix + tokenChars + `{20,}`)},
		{Name: "runner access token", Re: regexp.MustCompile(config.RunnerAccessTokenPrefix + tokenChars + `{20,}`)},
		{Name: "runner registration token", Re: regexp.MustCompile(config.RunnerRegistrationTokenPrefix + tokenChars + `{20,}`)},
		{
			Name: "artifact or cache download grant", Allowed: []string{"pg_dump/Jobs.sql"},
			Re: regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])A[Qg]AAAA` + tokenChars + `{49}(?:$|[^A-Za-z0-9_-])`),
		},
	}
}

func TestNoCredentialIsStoredInClear(t *testing.T) {
	ctx := context.Background()
	secret := serverKey(t)
	conn, err := pgx.Connect(ctx, required(t, envDatabase))
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	grantKey, err := appcrypto.DeriveKey(secret, config.ArtifactGrantKeyLabel)
	require.NoError(t, err)
	needles := sealedNeedles(t, conn, secret)
	needles = append(needles, checked{t}.needle(leaksweep.Encoded("server secret key", secret)), checked{t}.needle(leaksweep.Encoded("artifact grant key", grantKey)))
	needles = append(needles, listedNeedles(t, required(t, envNeedles))...)
	needles = append(needles, runnerKeys(t, required(t, envIdentity))...)
	require.Empty(t, uncoveredSealed(t, conn), "the demo made sealed values the sweep cannot open; teach it these columns")

	dirs := strings.Split(required(t, envDirs), ":")
	hits, stats, err := leaksweep.Sweep(dirs, needles, patterns(), leaksweep.DefaultLimits())
	require.NoError(t, err, "the sweep cannot vouch for what it could not read")
	listed := make([]string, 0, len(needles)+len(patterns()))
	for _, n := range needles {
		listed = append(listed, n.Name)
	}
	for _, p := range patterns() {
		listed = append(listed, "any "+p.Name+", by its shape")
	}
	sort.Strings(listed)
	t.Logf("looked for %d kinds of credential in %d files, %d archives and %d archive entries under %s",
		len(listed), stats.Files, stats.Archives, stats.Entries, strings.Join(dirs, ", "))
	for _, name := range listed {
		t.Logf("  %s", name)
	}
	for _, h := range hits {
		t.Errorf("found the %s in clear in %s", h.Name, h.File)
	}
}
