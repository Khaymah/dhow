package keys_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const oldFormat = byte(1)

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

func TestAColumnWritesTheEnvelopeFormatAndResealsIt(t *testing.T) {
	env, q := dbEnvelope(t)
	col, err := keys.NewValues(env).Column(deploySpec())
	require.NoError(t, err)
	owner := keys.ProjectOwner(uuid.New())

	sealed, err := col.SealIn(t.Context(), q, owner, []byte("deploy-key:1"), []byte("a new deploy key"))
	require.NoError(t, err)
	assert.Equal(t, keys.FormatVersion, sealed[0])
	got, err := col.Open(t.Context(), owner, []byte("deploy-key:1"), sealed)
	require.NoError(t, err)
	assert.Equal(t, []byte("a new deploy key"), got)

	moved, err := col.Reseal(t.Context(), q, owner, []byte("deploy-key:1"), sealed)
	require.NoError(t, err)
	assert.Equal(t, keys.FormatVersion, moved[0], "a reseal writes the envelope format")
	assert.NotEqual(t, sealed, moved)
	assert.False(t, bytes.Contains(moved, []byte("a new deploy key")))
	got, err = col.Open(t.Context(), owner, []byte("deploy-key:1"), moved)
	require.NoError(t, err)
	assert.Equal(t, []byte("a new deploy key"), got)
	_, err = col.Open(t.Context(), owner, []byte("deploy-key:2"), moved)
	require.ErrorIs(t, err, keys.ErrSealed, "the reseal keeps the row context")
}

func TestAColumnRefusesEveryValueThatIsNotItsOwn(t *testing.T) {
	env, q := dbEnvelope(t)
	values := keys.NewValues(env)
	deploy, err := values.Column(deploySpec())
	require.NoError(t, err)
	hooks, err := values.Column(webhookSpec())
	require.NoError(t, err)
	owner, other := keys.ProjectOwner(uuid.New()), keys.ProjectOwner(uuid.New())
	_, err = deploy.SealIn(t.Context(), q, other, []byte("deploy-key:1"), []byte("make the other data key"))
	require.NoError(t, err)
	sealed, err := deploy.SealIn(t.Context(), q, owner, []byte("deploy-key:1"), []byte("row one"))
	require.NoError(t, err)
	old := append([]byte{oldFormat}, sealed[1:]...)

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
		{"the old format with an envelope body", deploy, owner, "deploy-key:1", old, keys.ErrSealed},
		{"the old format with a body of the old size", deploy, owner, "deploy-key:1", append([]byte{oldFormat}, make([]byte, 60)...), keys.ErrSealed},
		{"the old format alone", deploy, owner, "deploy-key:1", []byte{oldFormat}, keys.ErrSealed},
		{"an empty value", deploy, owner, "deploy-key:1", nil, keys.ErrSealed},
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
	col, err := keys.NewValues(env).Column(webhookSpec())
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
