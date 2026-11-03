//go:build leaksweep

package leaksweep_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	"github.com/Khaymah/dhow/internal/leaksweep"
)

const (
	envDatabase = "DHOW_LEAK_SWEEP_DATABASE_URL"
	envKeyFile  = "DHOW_LEAK_SWEEP_SECRET_KEY_FILE"
	envNeedles  = "DHOW_LEAK_SWEEP_NEEDLES"
	envDirs     = "DHOW_LEAK_SWEEP_DIRS"
	envIdentity = "DHOW_LEAK_SWEEP_RUNNER_IDENTITY"
	envKeyring  = "DHOW_LEAK_SWEEP_KEYRING_FILE"
	tokenChars  = `[A-Za-z0-9_-]`
)

var identityFiles = []string{"runner-state/identity", "runner-state/identity.pending"}

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

func readKeyring(t *testing.T) (*keys.Keyring, keys.KeyProvider) {
	t.Helper()
	raw, err := os.ReadFile(required(t, envKeyring))
	require.NoError(t, err)
	ring, err := keys.ParseKeyring(raw)
	clear(raw)
	require.NoError(t, err)
	provider, err := keys.NewFileProvider(ring)
	require.NoError(t, err)
	return ring, provider
}

func plainNeedles(t *testing.T, spec keys.Spec, name string, plain []byte) []leaksweep.Needle {
	t.Helper()
	switch spec.Label {
	case config.DeployKeySealLabel:
		out, err := leaksweep.PEMKey(name, plain)
		require.NoError(t, err)
		return out
	case config.TotpSealLabel:
		return []leaksweep.Needle{
			checked{t}.needle(leaksweep.Encoded(name, plain)),
			checked{t}.needle(leaksweep.Encoded(name+" as text", []byte(auth.TotpSecretText(plain)))),
		}
	case config.InstanceSettingsSealLabel:
		var fields map[string]string
		if json.Unmarshal(plain, &fields) != nil {
			fields = map[string]string{"": string(plain)}
		}
		var out []leaksweep.Needle
		for field, v := range fields {
			if len(v) < leaksweep.MinValueBytes {
				t.Logf("the %s %s is shorter than %d bytes; it is not looked for", name, field, leaksweep.MinValueBytes)
				continue
			}
			out = append(out, checked{t}.needle(leaksweep.Encoded(strings.TrimSpace(name+" "+field), []byte(v))))
		}
		return out
	}
	return []leaksweep.Needle{checked{t}.needle(leaksweep.Encoded(name, plain))}
}

func sealedNeedles(t *testing.T, pool *pgxpool.Pool, secret []byte, provider keys.KeyProvider) []leaksweep.Needle {
	t.Helper()
	ctx := context.Background()
	reg, err := app.ValueRegistry()
	require.NoError(t, err)
	values := keys.NewValues(keys.NewEnvelope(keys.NewPostgresStore(pool), provider), secret)
	var out []leaksweep.Needle
	opened := map[byte]int{}
	for _, l := range reg.Locations() {
		spec := l.Spec()
		col, err := values.Column(spec)
		require.NoError(t, err)
		var rows []keys.Value
		require.NoError(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			rows, err = l.Scan(ctx, tx, "", 1<<20, false)
			return err
		}))
		for _, v := range rows {
			name := spec.Name() + " of row " + v.Key
			plain, err := col.Open(ctx, v.Owner, v.Context, v.Sealed)
			require.NoError(t, err, "the sweep cannot open %s with this keyring and server key", name)
			opened[v.Sealed[0]]++
			out = append(out, plainNeedles(t, spec, name, plain)...)
		}
	}
	t.Logf("opened %d values in the envelope format and %d in the format from before the keyring", opened[keys.FormatVersion], opened[keys.LegacyFormat])
	require.NotEmpty(t, out, "no sealed value was found")
	deks, err := apptest.DataKeysInClear(ctx, pool, provider)
	require.NoError(t, err)
	for i, dek := range deks {
		out = append(out, checked{t}.needle(leaksweep.Encoded(fmt.Sprintf("data key %d after unwrapping", i+1), dek)))
	}
	return out
}

func keyringNeedles(t *testing.T, ring *keys.Keyring) []leaksweep.Needle {
	t.Helper()
	keks, err := apptest.KeyringKeys(ring)
	require.NoError(t, err)
	out := make([]leaksweep.Needle, 0, len(keks))
	for i, k := range keks {
		out = append(out, checked{t}.needle(leaksweep.Encoded(fmt.Sprintf("keyring key %d", i+1), k)))
	}
	return out
}

func uncoveredSealed(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	reg, err := app.ValueRegistry()
	require.NoError(t, err)
	known := map[string]bool{}
	for _, l := range reg.Locations() {
		known[l.Spec().Name()] = true
	}
	rows, err := pool.Query(context.Background(), `SELECT table_name || '.' || column_name,
		(xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM %I WHERE %I IS NOT NULL', table_name, column_name),
			false, true, '')))[1]::text::bigint
		FROM information_schema.columns
		WHERE table_schema = 'public' AND data_type = 'bytea' AND (column_name LIKE '%Sealed' OR column_name IN ('Secret', 'Secrets'))
		ORDER BY 1`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	seen := 0
	for rows.Next() {
		var name string
		var n int64
		require.NoError(t, rows.Scan(&name, &n))
		seen++
		if n > 0 && !known[name] {
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
	pool, err := pgxpool.New(ctx, required(t, envDatabase))
	require.NoError(t, err)
	defer pool.Close()
	grantKey, err := appcrypto.DeriveKey(secret, config.ArtifactGrantKeyLabel)
	require.NoError(t, err)
	ring, provider := readKeyring(t)
	needles := sealedNeedles(t, pool, secret, provider)
	needles = append(needles, keyringNeedles(t, ring)...)
	needles = append(needles, checked{t}.needle(leaksweep.Encoded("server secret key", secret)), checked{t}.needle(leaksweep.Encoded("artifact grant key", grantKey)))
	needles = append(needles, listedNeedles(t, required(t, envNeedles))...)
	needles = append(needles, runnerKeys(t, required(t, envIdentity))...)
	require.Empty(t, uncoveredSealed(t, pool), "the demo made sealed values that no value location covers; register these columns")

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
