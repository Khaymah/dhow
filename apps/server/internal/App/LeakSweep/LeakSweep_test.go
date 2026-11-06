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
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
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
	envTargets  = "DHOW_LEAK_SWEEP_EXPECT_TARGETS"
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
	if leaksweep.LooksLikePrivateKey(plain) {
		out, err := leaksweep.PrivateKey(name, plain)
		require.NoError(t, err)
		return out
	}
	if len(plain) < leaksweep.MinValueBytes {
		t.Logf("the %s is shorter than %d bytes; it is not looked for", name, leaksweep.MinValueBytes)
		return nil
	}
	return []leaksweep.Needle{checked{t}.needle(leaksweep.Encoded(name, plain))}
}

func pendingHostKeyNeedles(t *testing.T, pool *pgxpool.Pool) []leaksweep.Needle {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT "Host", "Port", "PendingHostKey" FROM "DeployTargetHosts" WHERE "PendingHostKey" IS NOT NULL AND NOT "IsDeleted"`)
	require.NoError(t, err)
	defer rows.Close()
	var out []leaksweep.Needle
	for rows.Next() {
		var host, key string
		var port int
		require.NoError(t, rows.Scan(&host, &port, &key))
		needles, err := leaksweep.PendingHostKey(host, port, key)
		require.NoError(t, err)
		out = append(out, needles...)
	}
	require.NoError(t, rows.Err())
	return out
}

func deployTargetsWithAKey(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "DeployTargets" WHERE "KeySecretName" IS NOT NULL AND NOT "IsDeleted"`).Scan(&n))
	return n
}

func privateKeyNeedles(needles []leaksweep.Needle) []leaksweep.Needle {
	var out []leaksweep.Needle
	for _, n := range needles {
		if strings.Contains(n.Name, " secret of row ") && strings.HasSuffix(n.Name, "(private seed)") {
			out = append(out, n)
		}
	}
	return out
}

type sealedSet struct {
	needles []leaksweep.Needle
	opened  map[string]int
	plain   map[string]bool
}

func kindName(spec keys.Spec, owner keys.Owner) string {
	switch spec.Name() {
	case "SecretVersions.ValueSealed":
		switch owner.Kind {
		case keys.OwnerOrganization:
			return "organization secret"
		case keys.OwnerProject:
			return "project secret"
		case keys.OwnerEnvironment:
			return "environment secret"
		}
	case "RegistryCredentials.PasswordSealed":
		return "registry password"
	}
	return spec.Name()
}

func sealedNeedles(t *testing.T, pool *pgxpool.Pool, provider keys.KeyProvider) sealedSet {
	t.Helper()
	ctx := context.Background()
	reg, err := app.ValueRegistry()
	require.NoError(t, err)
	values := keys.NewValues(keys.NewEnvelope(keys.NewPostgresStore(pool), provider))
	set := sealedSet{opened: map[string]int{}, plain: map[string]bool{}}
	for _, l := range reg.Locations() {
		spec := l.Spec()
		col, err := values.Column(spec)
		require.NoError(t, err)
		var rows []keys.Value
		require.NoError(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			rows, err = l.Scan(ctx, tx, "", 1<<20)
			return err
		}))
		for _, v := range rows {
			kind := kindName(spec, v.Owner)
			name := kind + " of row " + v.Key
			plain, err := col.Open(ctx, v.Owner, v.Context, v.Sealed)
			require.NoError(t, err, "the sweep cannot open %s with this keyring", name)
			set.opened[kind]++
			set.plain[string(plain)] = true
			set.needles = append(set.needles, plainNeedles(t, spec, name, plain)...)
		}
	}
	t.Logf("opened %d sealed values: %v", len(set.needles), set.opened)
	require.NotEmpty(t, set.needles, "no sealed value was found")
	deks, err := apptest.DataKeysInClear(ctx, pool, provider)
	require.NoError(t, err)
	for i, dek := range deks {
		set.needles = append(set.needles, checked{t}.needle(leaksweep.Encoded(fmt.Sprintf("data key %d after unwrapping", i+1), dek)))
	}
	return set
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
		{Name: "ingest token", Re: regexp.MustCompile(config.IngestTokenPrefix + tokenChars + `{20,}`)},
		{Name: "runner access token", Re: regexp.MustCompile(config.RunnerAccessTokenPrefix + tokenChars + `{20,}`)},
		{Name: "runner registration token", Re: regexp.MustCompile(config.RunnerRegistrationTokenPrefix + tokenChars + `{20,}`)},
		{
			Name: "artifact or cache download grant", Allowed: []string{"pg_dump/Jobs.sql"},
			Re: regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])A[Qg]AAAA` + tokenChars + `{49}(?:$|[^A-Za-z0-9_-])`),
		},
	}
}

var (
	scopedKinds = []string{"organization secret", "project secret", "environment secret", "registry password"}
	plantedKind = append([]string{"data key", "keyring key"}, scopedKinds...)
)

type world struct {
	secret  []byte
	needles []leaksweep.Needle
}

func buildWorld(t *testing.T) world {
	t.Helper()
	ctx := context.Background()
	secret := serverKey(t)
	pool, err := pgxpool.New(ctx, required(t, envDatabase))
	require.NoError(t, err)
	defer pool.Close()
	grantKey, err := appcrypto.DeriveKey(secret, config.ArtifactGrantKeyLabel)
	require.NoError(t, err)
	ring, provider := readKeyring(t)
	sealed := sealedNeedles(t, pool, provider)
	needles := sealed.needles
	needles = append(needles, keyringNeedles(t, ring)...)
	needles = append(needles, checked{t}.needle(leaksweep.Encoded("server secret key", secret)), checked{t}.needle(leaksweep.Encoded("artifact grant key", grantKey)))
	listed := listedNeedles(t, required(t, envNeedles))
	needles = append(needles, listed...)
	needles = append(needles, runnerKeys(t, required(t, envIdentity))...)
	needles = append(needles, pendingHostKeyNeedles(t, pool)...)
	require.Empty(t, uncoveredSealed(t, pool), "the demo made sealed values that no value location covers; register these columns")
	targets := deployTargetsWithAKey(t, pool)
	if os.Getenv(envTargets) != "" {
		require.Positive(t, targets, "%s is set, but the demo made no deploy target with a key", envTargets)
	}
	if targets > 0 {
		assert.NotEmpty(t, privateKeyNeedles(needles), "the demo has %d deploy targets with a key, but the sweep opened no private key", targets)
	}
	for _, kind := range scopedKinds {
		require.Positive(t, sealed.opened[kind], "the demo stored no %s, so the sweep would prove nothing about it", kind)
		var found bool
		for _, n := range listed {
			if n.Name == kind {
				found = true
				assert.True(t, sealed.plain[string(n.Forms[0])], "the %s in the needles file is not what the database holds sealed", kind)
			}
		}
		assert.True(t, found, "the needles file has no %s", kind)
	}
	return world{secret: secret, needles: needles}
}

func TestNoCredentialIsStoredInClear(t *testing.T) {
	w := buildWorld(t)
	needles := w.needles
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

func TestEachKindOfCredentialIsCaughtWhenPlanted(t *testing.T) {
	w := buildWorld(t)
	for _, kind := range plantedKind {
		t.Run(kind, func(t *testing.T) {
			var planted *leaksweep.Needle
			for i, n := range w.needles {
				if strings.HasPrefix(n.Name, kind) && len(n.Allowed) == 0 {
					planted = &w.needles[i]
					break
				}
			}
			require.NotNil(t, planted, "the sweep looks for no %s", kind)
			dir := t.TempDir()
			file := filepath.Join(dir, "leak.log")
			require.NoError(t, os.WriteFile(file, append(append([]byte(`{"msg":"x","value":"`), planted.Forms[0]...), []byte("\"}\n")...), 0o600))
			hits, _, err := leaksweep.Sweep([]string{dir}, w.needles, nil, leaksweep.DefaultLimits())
			require.NoError(t, err)
			require.NotEmpty(t, hits, "a %s planted on purpose is not found", kind)
			for _, h := range hits {
				assert.Equal(t, file, h.File)
				assert.True(t, strings.HasPrefix(h.Name, kind), "found %q while looking for a %s", h.Name, kind)
			}
			encoded := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(encoded, "encoded.log"), []byte(base64.StdEncoding.EncodeToString(planted.Forms[0])+"\n"), 0o600))
			hits, _, err = leaksweep.Sweep([]string{encoded}, w.needles, nil, leaksweep.DefaultLimits())
			require.NoError(t, err)
			assert.NotEmpty(t, hits, "a %s planted as base64 is not found", kind)
		})
	}
}

func TestAPrivateKeyOfATargetIsCaughtWhenPlanted(t *testing.T) {
	w := buildWorld(t)
	planted := privateKeyNeedles(w.needles)
	if len(planted) == 0 {
		require.Empty(t, os.Getenv(envTargets), "the demo made no deploy target with a key")
		t.Skip("the demo made no deploy target with a key")
	}
	for _, n := range planted {
		dir := t.TempDir()
		file := filepath.Join(dir, "leak.log")
		require.NoError(t, os.WriteFile(file, append(append([]byte(`{"msg":"x","value":"`), n.Forms[0]...), []byte("\"}\n")...), 0o600))
		hits, _, err := leaksweep.Sweep([]string{dir}, w.needles, nil, leaksweep.DefaultLimits())
		require.NoError(t, err)
		assert.NotEmpty(t, hits, "the %s planted on purpose is not found", n.Name)
	}
}

func TestAPendingHostKeyIsCaughtWhenPlantedAndAllowedInItsRowOnly(t *testing.T) {
	w := buildWorld(t)
	var pending []leaksweep.Needle
	for _, n := range w.needles {
		if strings.HasPrefix(n.Name, "pending host key of ") {
			pending = append(pending, n)
		}
	}
	if len(pending) == 0 {
		t.Skip("the demo left no pending host key")
	}
	for _, n := range pending {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "pg_dump"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pg_dump", "DeployTargetHosts.sql"), n.Forms[0], 0o600))
		hits, _, err := leaksweep.Sweep([]string{dir}, pending, nil, leaksweep.DefaultLimits())
		require.NoError(t, err)
		assert.Empty(t, hits, "the row of the host may hold its own pending key")
		file := filepath.Join(dir, "server.log")
		require.NoError(t, os.WriteFile(file, n.Forms[0], 0o600))
		hits, _, err = leaksweep.Sweep([]string{dir}, pending, nil, leaksweep.DefaultLimits())
		require.NoError(t, err)
		assert.NotEmpty(t, hits, "the %s planted on purpose is not found", n.Name)
	}
}
