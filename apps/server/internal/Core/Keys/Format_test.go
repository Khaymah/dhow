package keys_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

const (
	saltBytes  = 32
	idBytes    = 16
	nonceBytes = 12
)

func TestValuesUseAFreshSubkeyPerValue(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	store := newMemoryStore(1)
	env := keys.NewEnvelope(store, provider)
	owner := keys.ProjectOwner(uuid.New())
	plain := []byte("a deploy key")
	sealed, err := env.Seal(t.Context(), owner, labelDeployKey, []byte("deploy-key:1"), plain)
	require.NoError(t, err)
	again, err := env.Seal(t.Context(), owner, labelDeployKey, []byte("deploy-key:1"), plain)
	require.NoError(t, err)

	require.Equal(t, byte(3), keys.FormatVersion, "0x01 is Sealer and 0x02 never shipped")
	require.Equal(t, 1+idBytes+saltBytes+nonceBytes+16, keys.Overhead)
	require.Equal(t, keys.FormatVersion, sealed[0])
	require.Len(t, sealed, len(plain)+keys.Overhead)
	assert.NotEqual(t, sealed[1+idBytes:1+idBytes+saltBytes], again[1+idBytes:1+idBytes+saltBytes], "a new salt for every value")

	id, err := uuid.FromBytes(sealed[1 : 1+idBytes])
	require.NoError(t, err)
	dk, err := store.DataKey(t.Context(), id)
	require.NoError(t, err)
	dek, err := provider.Unwrap(t.Context(), dk.KekVersion, dk.Wrapped, keys.DekAADForTest(id, owner, dk.KekVersion))
	require.NoError(t, err)
	salt := sealed[1+idBytes : 1+idBytes+saltBytes]
	subkey, err := hkdf.Key(sha256.New, dek, salt, "dhow/value/v2"+labelDeployKey, 32)
	require.NoError(t, err)
	block, err := aes.NewCipher(subkey)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	aad := append([]byte{keys.FormatVersion}, sealed[1:1+idBytes]...)
	aad = binary.BigEndian.AppendUint16(aad, uint16(len(labelDeployKey)))
	aad = append(append(aad, labelDeployKey...), "deploy-key:1"...)
	head := 1 + idBytes + saltBytes
	got, err := aead.Open(nil, sealed[head:head+nonceBytes], sealed[head+nonceBytes:], aad)
	require.NoError(t, err, "the value opens with HKDF-SHA256(DEK, salt, \"dhow/value/v2\"||label)")
	assert.Equal(t, plain, got)

	direct, err := aes.NewCipher(dek)
	require.NoError(t, err)
	directGCM, err := cipher.NewGCM(direct)
	require.NoError(t, err)
	_, err = directGCM.Open(nil, sealed[head:head+nonceBytes], sealed[head+nonceBytes:], aad)
	require.Error(t, err, "the data key never encrypts a value directly")
}

func TestARowContextIsRequired(t *testing.T) {
	env, _ := newMemoryEnvelope(t)
	owner := keys.InstanceOwner()
	_, err := env.Seal(t.Context(), owner, "dhow/x/v1", nil, []byte("x"))
	require.ErrorIs(t, err, keys.ErrBadContext)
	_, err = env.Seal(t.Context(), owner, "dhow/x/v1", []byte{}, []byte("x"))
	require.ErrorIs(t, err, keys.ErrBadContext)
	sealed, err := env.Seal(t.Context(), owner, "dhow/x/v1", []byte("row"), []byte("x"))
	require.NoError(t, err)
	_, err = env.Open(t.Context(), owner, "dhow/x/v1", nil, sealed)
	require.ErrorIs(t, err, keys.ErrBadContext)
}
