package keys_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func deploySpec() keys.Spec {
	return keys.Spec{
		Table: "Repositories", Column: "DeployKeySealed", Label: labelDeployKey, Owner: keys.OwnerProject,
		MaxPlaintext: 4096, MaxSealed: 8192,
	}
}

func webhookSpec() keys.Spec {
	return keys.Spec{
		Table: "Repositories", Column: "WebhookSecretSealed", Label: labelWebhook, Owner: keys.OwnerProject,
		MaxPlaintext: 256, MaxSealed: 1024,
	}
}

func secretKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	_, err := rand.Read(k)
	require.NoError(t, err)
	return k
}

func legacySeal(t *testing.T, secret []byte, label, rowContext string, plain []byte) []byte {
	t.Helper()
	s, err := appcrypto.NewSealer(secret, label)
	require.NoError(t, err)
	out, err := s.Seal(plain, []byte(rowContext))
	require.NoError(t, err)
	return out
}

func dbEnvelope(t *testing.T) (*keys.Envelope, *sqlc.Queries) {
	t.Helper()
	pool := dbtest.NewPool(t)
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	m := keys.NewManager(pool, provider, nil)
	_, err = m.Register(t.Context())
	require.NoError(t, err)
	return m.Envelope(), sqlc.New(pool)
}

func TestAColumnReadsBothFormatsAndWritesTheEnvelopeFormat(t *testing.T) {
	env, q := dbEnvelope(t)
	secret := secretKey(t)
	col, err := keys.NewValues(env, secret).Column(deploySpec())
	require.NoError(t, err)
	owner := keys.ProjectOwner(uuid.New())
	old := legacySeal(t, secret, labelDeployKey, "deploy-key:1", []byte("an old deploy key"))
	require.Equal(t, keys.LegacyFormat, old[0])
	assert.True(t, keys.IsLegacy(old))

	got, err := col.Open(t.Context(), owner, []byte("deploy-key:1"), old)
	require.NoError(t, err)
	assert.Equal(t, []byte("an old deploy key"), got)

	sealed, err := col.SealIn(t.Context(), q, owner, []byte("deploy-key:1"), []byte("a new deploy key"))
	require.NoError(t, err)
	assert.Equal(t, keys.FormatVersion, sealed[0])
	got, err = col.Open(t.Context(), owner, []byte("deploy-key:1"), sealed)
	require.NoError(t, err)
	assert.Equal(t, []byte("a new deploy key"), got)

	moved, err := col.Reseal(t.Context(), q, owner, []byte("deploy-key:1"), old)
	require.NoError(t, err)
	assert.Equal(t, keys.FormatVersion, moved[0], "a reseal writes the envelope format")
	assert.False(t, bytes.Contains(moved, []byte("an old deploy key")))
	got, err = col.Open(t.Context(), owner, []byte("deploy-key:1"), moved)
	require.NoError(t, err)
	assert.Equal(t, []byte("an old deploy key"), got)
	_, err = col.Open(t.Context(), owner, []byte("deploy-key:2"), moved)
	require.ErrorIs(t, err, keys.ErrSealed, "the reseal keeps the row context")
}

func TestAColumnRefusesEveryValueThatIsNotItsOwn(t *testing.T) {
	env, q := dbEnvelope(t)
	secret := secretKey(t)
	values := keys.NewValues(env, secret)
	deploy, err := values.Column(deploySpec())
	require.NoError(t, err)
	hooks, err := values.Column(webhookSpec())
	require.NoError(t, err)
	owner, other := keys.ProjectOwner(uuid.New()), keys.ProjectOwner(uuid.New())
	_, err = deploy.SealIn(t.Context(), q, other, []byte("deploy-key:1"), []byte("make the other data key"))
	require.NoError(t, err)
	sealed, err := deploy.SealIn(t.Context(), q, owner, []byte("deploy-key:1"), []byte("row one"))
	require.NoError(t, err)
	old := legacySeal(t, secret, labelDeployKey, "deploy-key:1", []byte("row one"))
	withoutLegacy, err := keys.NewValues(env, nil).Column(deploySpec())
	require.NoError(t, err)
	otherSecret, err := keys.NewValues(env, secretKey(t)).Column(deploySpec())
	require.NoError(t, err)

	tests := []struct {
		name   string
		column *keys.Column
		owner  keys.Owner
		ctx    string
		value  []byte
		want   error
	}{
		{"the wrong owner", deploy, other, "deploy-key:1", sealed, keys.ErrSealed},
		{"the wrong kind of owner", deploy, keys.OrganizationOwner(owner.ID), "deploy-key:1", sealed, keys.ErrWrongOwnerKind},
		{"the wrong row", deploy, owner, "deploy-key:2", sealed, keys.ErrSealed},
		{"the wrong column", hooks, owner, "deploy-key:1", sealed, keys.ErrSealed},
		{"a truncated value", deploy, owner, "deploy-key:1", sealed[:len(sealed)-1], keys.ErrSealed},
		{"a value cut to its header", deploy, owner, "deploy-key:1", sealed[:keys.Overhead-1], keys.ErrSealed},
		{"the M5.1 draft format", deploy, owner, "deploy-key:1", append([]byte{2}, sealed[1:]...), keys.ErrSealed},
		{"an unknown format byte", deploy, owner, "deploy-key:1", append([]byte{0xff}, sealed[1:]...), keys.ErrSealed},
		{"a zero format byte", deploy, owner, "deploy-key:1", append([]byte{0}, sealed[1:]...), keys.ErrSealed},
		{"an empty value", deploy, owner, "deploy-key:1", nil, keys.ErrSealed},
		{"an old value for another row", deploy, owner, "deploy-key:2", old, keys.ErrSealed},
		{"an old value in another column", hooks, owner, "deploy-key:1", old, keys.ErrSealed},
		{"an old value truncated", deploy, owner, "deploy-key:1", old[:len(old)-1], keys.ErrSealed},
		{"an old value with no server key", withoutLegacy, owner, "deploy-key:1", old, keys.ErrSealed},
		{"an old value with another server key", otherSecret, owner, "deploy-key:1", old, keys.ErrSealed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.column.Open(t.Context(), tt.owner, []byte(tt.ctx), tt.value)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestAColumnSealsOnlyForItsOwnerKindAndSize(t *testing.T) {
	env, q := dbEnvelope(t)
	col, err := keys.NewValues(env, nil).Column(webhookSpec())
	require.NoError(t, err)
	_, err = col.SealIn(t.Context(), q, keys.UserOwner(uuid.New()), []byte("webhook-secret:1"), []byte("x"))
	require.ErrorIs(t, err, keys.ErrWrongOwnerKind)
	_, err = col.SealIn(t.Context(), q, keys.ProjectOwner(uuid.New()), []byte("webhook-secret:1"), make([]byte, 257))
	require.ErrorIs(t, err, keys.ErrTooLarge)
	owner := keys.ProjectOwner(uuid.New())
	sealed, err := col.SealIn(t.Context(), q, owner, []byte("webhook-secret:1"), make([]byte, 256))
	require.NoError(t, err)
	assert.Len(t, sealed, 256+keys.Overhead)
	_, err = env.Destroy(t.Context(), owner)
	require.NoError(t, err)
	_, err = col.Open(t.Context(), owner, []byte("webhook-secret:1"), sealed)
	require.ErrorIs(t, err, keys.ErrShredded, "a value under a destroyed data key never opens")
}

func TestASpecMustFitItsColumnOnceSealed(t *testing.T) {
	good := deploySpec()
	require.NoError(t, good.Valid())
	tight := good
	tight.MaxSealed = tight.MaxPlaintext + keys.Overhead - 1
	require.ErrorContains(t, tight.Valid(), "does not fit")
	for name, mutate := range map[string]func(*keys.Spec){
		"no table":         func(s *keys.Spec) { s.Table = "" },
		"no label":         func(s *keys.Spec) { s.Label = "" },
		"an unknown owner": func(s *keys.Spec) { s.Owner = "team" },
		"no plaintext":     func(s *keys.Spec) { s.MaxPlaintext = 0 },
	} {
		s := good
		mutate(&s)
		require.Error(t, s.Valid(), name)
	}
	_, err := keys.NewRegistry(keys.RowLocation[int]{Of: good}, keys.RowLocation[string]{Of: good})
	require.ErrorIs(t, err, keys.ErrDuplicateLocation)
}

func TestTheOldFormatIsRefusedOnceItsColumnIsConverted(t *testing.T) {
	pool := dbtest.NewPool(t)
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	m := keys.NewManager(pool, provider, nil)
	_, err = m.Register(t.Context())
	require.NoError(t, err)
	secret := secretKey(t)
	owner := keys.ProjectOwner(uuid.New())
	planted := legacySeal(t, secret, labelDeployKey, "deploy-key:1", []byte("a key planted with the server key"))

	values := keys.NewValues(m.Envelope(), secret)
	col, err := values.Column(deploySpec())
	require.NoError(t, err)
	_, err = col.Open(t.Context(), owner, []byte("deploy-key:1"), planted)
	require.NoError(t, err, "before the backfill the old format still reads")
	values.BackfillDone(keys.BackfillName(deploySpec()))
	_, err = col.Open(t.Context(), owner, []byte("deploy-key:1"), planted)
	require.ErrorIs(t, err, keys.ErrSealed, "after the backfill a planted old value is refused")

	_, err = pool.Exec(t.Context(), `INSERT INTO "Backfills" ("Name", "DoneAt") VALUES ($1, now())`, keys.BackfillName(deploySpec()))
	require.NoError(t, err)
	booted := keys.NewValues(m.Envelope(), secret)
	require.NoError(t, booted.LoadLegacyState(t.Context(), pool))
	other, err := booted.Column(webhookSpec())
	require.NoError(t, err)
	again, err := booted.Column(deploySpec())
	require.NoError(t, err)
	_, err = again.Open(t.Context(), owner, []byte("deploy-key:1"), planted)
	require.ErrorIs(t, err, keys.ErrSealed, "a server that starts after the backfill refuses it too")
	hook := legacySeal(t, secret, labelWebhook, "webhook-secret:1", []byte("a webhook secret not yet converted"))
	_, err = other.Open(t.Context(), owner, []byte("webhook-secret:1"), hook)
	require.NoError(t, err, "a column whose backfill is not done still reads its old values")
}
