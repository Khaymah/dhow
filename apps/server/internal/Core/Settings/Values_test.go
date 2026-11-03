package instancesettings

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	"github.com/Khaymah/dhow/internal/logging"
)

func testValues(t *testing.T, pool *pgxpool.Pool, secret []byte) *keys.Values {
	t.Helper()
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	m := keys.NewManager(pool, provider, nil)
	_, err = m.Register(t.Context())
	require.NoError(t, err)
	return keys.NewValues(m.Envelope(), secret)
}

func randomSecret(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func newDBStore(t *testing.T, pool *pgxpool.Pool, secret []byte) *Store {
	t.Helper()
	if secret == nil {
		secret = randomSecret(t)
	}
	s, err := New(Options{DB: pool, Values: testValues(t, pool, secret), Log: logging.New(io.Discard, slog.LevelError)})
	require.NoError(t, err)
	return s
}

func TestSecretsSavedBeforeTheKeyringStillOpen(t *testing.T) {
	pool := dbtest.NewPool(t)
	secret := randomSecret(t)
	settingsSealer, err := appcrypto.NewSealer(secret, config.InstanceSettingsSealLabel)
	require.NoError(t, err)
	mail, err := settingsSealer.Seal([]byte(`{"Password":"old-relay-credential"}`), []byte("instance-settings:mail"))
	require.NoError(t, err)
	oidc, err := settingsSealer.Seal([]byte("Zq3vT8wLp2Rk9sXy4Nb7Hc1Jd6Fg0Ma5"), []byte("instance-settings:oidc:corp"))
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO "InstanceSettings" ("Group", "Value", "Secrets")
		VALUES ('mail', '{"Host": "smtp.example.com", "From": "dhow@example.com", "Username": "dhow"}'::jsonb, $1)`, mail)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO "OidcProviders" ("ProviderId", "Type", "Name", "Issuer", "ClientId", "Secret")
		VALUES ('corp', 'oidc', 'Corp', 'https://id.example.com', 'dhow', $1)`, oidc)
	require.NoError(t, err)

	s := newDBStore(t, pool, secret)
	require.NoError(t, s.Load(t.Context()))
	assert.Equal(t, "old-relay-credential", s.Current().Values.Mail.Password)
	p, ok := s.Current().Oidc.Find("corp")
	require.True(t, ok)
	assert.Equal(t, "Zq3vT8wLp2Rk9sXy4Nb7Hc1Jd6Fg0Ma5", p.Config.ClientSecret)
}

func TestTheLargestSecretsFitTheirColumnsOnceSealed(t *testing.T) {
	pool := dbtest.NewPool(t)
	s := newDBStore(t, pool, nil)
	worst := strings.Repeat("\x01", settingspec.SMTPPasswordMaxLength)
	require.Nil(t, settingspec.ValidateValue(settingspec.EnvSMTPPassword, worst))
	raw := settingspec.Raw{settingspec.EnvSMTPPassword: worst}
	for _, k := range settingspec.Keys() {
		if k.Secret && k.Env != settingspec.EnvSMTPPassword {
			t.Fatalf("a new secret setting %s needs its worst case in this test", k.Env)
		}
	}
	value, sealed, err := s.encode(t.Context(), sqlc.New(pool), settingspec.GroupMail, raw)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(sealed)-keys.Overhead, SecretsSpec().MaxPlaintext)
	_, err = pool.Exec(t.Context(), `INSERT INTO "InstanceSettings" ("Group", "Value", "Secrets") VALUES ('mail', $1::jsonb, $2)`, value, sealed)
	require.NoError(t, err, "the CHECK on the column takes the largest value")

	big, err := s.sealSecret(t.Context(), sqlc.New(pool), "corp", strings.Repeat("s", settingspec.OIDCMaxSecretBytes))
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO "OidcProviders" ("ProviderId", "Type", "Name", "Issuer", "ClientId", "Secret")
		VALUES ('corp', 'oidc', 'Corp', 'https://id.example.com', 'dhow', $1)`, big)
	require.NoError(t, err, "the CHECK on the column takes the largest client secret")
	for _, spec := range []keys.Spec{SecretsSpec(), OidcSecretSpec()} {
		require.NoError(t, spec.Valid())
	}
}

func TestASaveOfAProviderNeverWritesBackASecretThatWasJustSealedAgain(t *testing.T) {
	pool := dbtest.NewPool(t)
	secret := randomSecret(t)
	legacy, err := appcrypto.NewSealer(secret, config.InstanceSettingsSealLabel)
	require.NoError(t, err)
	old, err := legacy.Seal([]byte("Zq3vT8wLp2Rk9sXy4Nb7Hc1Jd6Fg0Ma5"), []byte("instance-settings:oidc:corp"))
	require.NoError(t, err)
	var actor uuid.UUID
	require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('o@example.com', 'O', 'x') RETURNING "Id"`).Scan(&actor))
	_, err = pool.Exec(t.Context(), `INSERT INTO "OidcProviders" ("ProviderId", "Type", "Name", "Issuer", "ClientId", "Secret")
		VALUES ('corp', 'oidc', 'Corp', 'https://id.example.com', 'dhow', $1)`, old)
	require.NoError(t, err)
	s := newDBStore(t, pool, secret)
	require.NoError(t, s.Load(t.Context()))

	tx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	resealed := append([]byte{keys.FormatVersion}, make([]byte, 96)...)
	_, err = tx.Exec(t.Context(), `SELECT 1 FROM "OidcProviders" WHERE "ProviderId" = 'corp' FOR UPDATE`)
	require.NoError(t, err)
	_, err = tx.Exec(t.Context(), `UPDATE "OidcProviders" SET "Secret" = $1 WHERE "ProviderId" = 'corp'`, resealed)
	require.NoError(t, err)
	name := "Corp SSO"
	saved := make(chan error, 1)
	go func() {
		_, err := s.ApplyOidc(context.Background(), OidcChange{Operation: OidcUpdate, ID: "corp", Version: 1, Patch: OidcPatch{Name: &name}}, actor)
		saved <- err
	}()
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, tx.Commit(t.Context()))
	require.Error(t, <-saved, "the locked read sees the value sealed again, which this store cannot open, so the save is refused")
	var stored []byte
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "Secret" FROM "OidcProviders" WHERE "ProviderId" = 'corp'`).Scan(&stored))
	assert.NotEqual(t, old, stored, "the save did not write the stale copy back over the value sealed again")
}
