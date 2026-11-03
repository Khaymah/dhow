package keys_test

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

const (
	labelDeployKey = "dhow/git-deploy-key/v1"
	labelWebhook   = "dhow/git-webhook-secret/v1"
)

func TestSealedValueHidesThePlaintextAndIsNeverRepeated(t *testing.T) {
	env, _ := newMemoryEnvelope(t)
	owner := keys.ProjectOwner(uuid.New())
	plain := []byte("a deploy key that must never show up in the database")
	a, err := env.Seal(t.Context(), owner, labelDeployKey, []byte("deploy-key:1"), plain)
	require.NoError(t, err)
	b, err := env.Seal(t.Context(), owner, labelDeployKey, []byte("deploy-key:1"), plain)
	require.NoError(t, err)

	assert.False(t, bytes.Contains(a, plain))
	assert.False(t, bytes.Contains(a, plain[:16]))
	assert.NotEqual(t, a, b)
	assert.Equal(t, keys.FormatVersion, a[0])
	assert.Len(t, a, len(plain)+keys.Overhead)
	assert.True(t, keys.IsSealedV2(a))
	assert.Equal(t, a[1:17], b[1:17], "one data key per owner")

	for _, sealed := range [][]byte{a, b} {
		got, err := env.Open(t.Context(), owner, labelDeployKey, []byte("deploy-key:1"), sealed)
		require.NoError(t, err)
		assert.Equal(t, plain, got)
	}
}

func TestSealedValueOpensForItsOwnRowOwnerAndPurposeOnly(t *testing.T) {
	env, _ := newMemoryEnvelope(t)
	projectA, projectB := keys.ProjectOwner(uuid.New()), keys.ProjectOwner(uuid.New())
	_, err := env.Seal(t.Context(), projectB, labelDeployKey, []byte("deploy-key:1"), []byte("make B's data key"))
	require.NoError(t, err)
	sealed, err := env.Seal(t.Context(), projectA, labelDeployKey, []byte("deploy-key:1"), []byte("row one"))
	require.NoError(t, err)

	tests := []struct {
		name    string
		owner   keys.Owner
		label   string
		context string
	}{
		{"another row", projectA, labelDeployKey, "deploy-key:2"},
		{"another column", projectA, labelWebhook, "deploy-key:1"},
		{"another owner with the same context", projectB, labelDeployKey, "deploy-key:1"},
		{"another kind with the same id", keys.OrganizationOwner(projectA.ID), labelDeployKey, "deploy-key:1"},
		{"the instance", keys.InstanceOwner(), labelDeployKey, "deploy-key:1"},
		{"label and context shifted", projectA, labelDeployKey + "d", "eploy-key:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.Open(t.Context(), tt.owner, tt.label, []byte(tt.context), sealed)
			assert.ErrorIs(t, err, keys.ErrSealed)
		})
	}
	got, err := env.Open(t.Context(), projectA, labelDeployKey, []byte("deploy-key:1"), sealed)
	require.NoError(t, err)
	assert.Equal(t, []byte("row one"), got)
}

func TestTamperedValuesDoNotOpen(t *testing.T) {
	env, _ := newMemoryEnvelope(t)
	owner := keys.UserOwner(uuid.New())
	sealed, err := env.Seal(t.Context(), owner, "dhow/totp-secret/v1", []byte("user"), []byte("JBSWY3DPEHPK3PXP"))
	require.NoError(t, err)
	for i := range sealed {
		changed := bytes.Clone(sealed)
		changed[i] ^= 0x01
		_, err := env.Open(t.Context(), owner, "dhow/totp-secret/v1", []byte("user"), changed)
		assert.ErrorIs(t, err, keys.ErrSealed, "byte %d", i)
	}
	for _, short := range [][]byte{nil, {}, {keys.FormatVersion}, sealed[:keys.Overhead-1]} {
		_, err := env.Open(t.Context(), owner, "dhow/totp-secret/v1", []byte("user"), short)
		assert.ErrorIs(t, err, keys.ErrSealed)
	}
	v1 := bytes.Clone(sealed)
	v1[0] = 1
	_, err = env.Open(t.Context(), owner, "dhow/totp-secret/v1", []byte("user"), v1)
	assert.ErrorIs(t, err, keys.ErrSealed)
}

func TestDestroyShredsEveryValueOfTheOwner(t *testing.T) {
	env, store := newMemoryEnvelope(t)
	gone, kept := keys.EnvironmentOwner(uuid.New()), keys.EnvironmentOwner(uuid.New())
	var sealedGone [][]byte
	for i := range 5 {
		s, err := env.Seal(t.Context(), gone, "dhow/secret/v1", []byte{byte(i)}, []byte("production password"))
		require.NoError(t, err)
		sealedGone = append(sealedGone, s)
	}
	sealedKept, err := env.Seal(t.Context(), kept, "dhow/secret/v1", []byte{0}, []byte("staging password"))
	require.NoError(t, err)

	n, err := env.Destroy(t.Context(), gone)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	for i, s := range sealedGone {
		_, err := env.Open(t.Context(), gone, "dhow/secret/v1", []byte{byte(i)}, s)
		require.ErrorIs(t, err, keys.ErrShredded)
		require.ErrorIs(t, err, keys.ErrSealed)
	}
	got, err := env.Open(t.Context(), kept, "dhow/secret/v1", []byte{0}, sealedKept)
	require.NoError(t, err)
	assert.Equal(t, []byte("staging password"), got)

	fresh, err := env.Seal(t.Context(), gone, "dhow/secret/v1", []byte{9}, []byte("new value"))
	require.NoError(t, err)
	assert.NotEqual(t, sealedGone[0][1:17], fresh[1:17], "a new data key after shredding")
	assert.Equal(t, 2, store.activeRows())
}

func TestDataKeysAreMadeOncePerOwnerEvenUnderRace(t *testing.T) {
	env, store := newMemoryEnvelope(t)
	owner := keys.OrganizationOwner(uuid.New())
	var wg sync.WaitGroup
	sealed := make([][]byte, 32)
	for i := range sealed {
		wg.Go(func() {
			s, err := env.Seal(context.Background(), owner, "dhow/secret/v1", []byte("row"), []byte("value"))
			assert.NoError(t, err)
			sealed[i] = s
		})
	}
	wg.Wait()
	assert.Equal(t, 1, store.inserts)
	for _, s := range sealed {
		_, err := env.Open(t.Context(), owner, "dhow/secret/v1", []byte("row"), s)
		require.NoError(t, err)
	}
}

func TestSealRejectsBadInput(t *testing.T) {
	env, _ := newMemoryEnvelope(t)
	_, err := env.Seal(t.Context(), keys.Owner{Kind: keys.OwnerProject}, "dhow/x/v1", []byte("row"), []byte("x"))
	require.ErrorIs(t, err, keys.ErrBadOwner)
	_, err = env.Seal(t.Context(), keys.Owner{Kind: keys.OwnerInstance, ID: uuid.New()}, "dhow/x/v1", []byte("row"), []byte("x"))
	require.ErrorIs(t, err, keys.ErrBadOwner)
	_, err = env.Seal(t.Context(), keys.Owner{Kind: "team", ID: uuid.New()}, "dhow/x/v1", []byte("row"), []byte("x"))
	require.ErrorIs(t, err, keys.ErrBadOwner)
	_, err = env.Seal(t.Context(), keys.InstanceOwner(), "", []byte("row"), []byte("x"))
	require.ErrorIs(t, err, keys.ErrBadLabel)
	_, err = env.Seal(t.Context(), keys.InstanceOwner(), string(bytes.Repeat([]byte("l"), 256)), []byte("row"), []byte("x"))
	require.ErrorIs(t, err, keys.ErrBadLabel)
	_, err = env.Seal(t.Context(), keys.InstanceOwner(), "dhow/x/v1", []byte("row"), make([]byte, keys.MaxPlaintext+1))
	require.ErrorIs(t, err, keys.ErrTooLarge)
	_, err = env.Destroy(t.Context(), keys.Owner{Kind: keys.OwnerUser})
	require.ErrorIs(t, err, keys.ErrBadOwner)

	sealed, err := env.Seal(t.Context(), keys.InstanceOwner(), "dhow/x/v1", []byte("row"), nil)
	require.NoError(t, err)
	got, err := env.Open(t.Context(), keys.InstanceOwner(), "dhow/x/v1", []byte("row"), sealed)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func FuzzOpen(f *testing.F) {
	env, _ := newMemoryEnvelope(f)
	owner := keys.ProjectOwner(uuid.MustParse("0190f3a8-0000-7000-8000-000000000001"))
	sealed, err := env.Seal(context.Background(), owner, labelDeployKey, []byte("deploy-key:1"), []byte("seed value"))
	require.NoError(f, err)
	f.Add(sealed, []byte("deploy-key:1"))
	f.Add([]byte{keys.FormatVersion}, []byte(""))
	f.Add(append([]byte{keys.FormatVersion}, make([]byte, keys.Overhead)...), []byte("deploy-key:1"))
	f.Fuzz(func(t *testing.T, value, rowContext []byte) {
		got, err := env.Open(context.Background(), owner, labelDeployKey, rowContext, value)
		if len(rowContext) == 0 {
			assert.ErrorIs(t, err, keys.ErrBadContext)
			return
		}
		if err != nil {
			assert.ErrorIs(t, err, keys.ErrSealed)
			return
		}
		assert.Equal(t, []byte("seed value"), got)
		assert.Equal(t, sealed, value, "only the value that was sealed opens")
		assert.Equal(t, []byte("deploy-key:1"), rowContext)
	})
}
