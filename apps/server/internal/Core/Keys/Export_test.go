package keys_test

import (
	"bytes"
	"io"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

func TestExportOpensWithTheAgePrivateKeyOnly(t *testing.T) {
	ring, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	recipient, err := keys.ParseRecipient(identity.Recipient().String())
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, keys.Export(&out, ring, recipient))
	assert.False(t, bytes.Contains(out.Bytes(), ring.Marshal()))
	assert.False(t, bytes.Contains(out.Bytes(), []byte("kek 1 ")))

	r, err := age.Decrypt(bytes.NewReader(out.Bytes()), identity)
	require.NoError(t, err)
	plain, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, ring.Marshal(), plain)
	restored, err := keys.ParseKeyring(plain)
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 2}, restored.Versions())

	other, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	_, err = age.Decrypt(bytes.NewReader(out.Bytes()), other)
	require.Error(t, err)
}

func TestParseRecipientTakesOnePublicKey(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	pub := identity.Recipient().String()
	broken := pub[:len(pub)-1] + "q"
	if pub[len(pub)-1] == 'q' {
		broken = pub[:len(pub)-1] + "p"
	}
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"private key", identity.String(), keys.ErrRecipientSecret},
		{"private key in lower case", " " + string(bytes.ToLower([]byte(identity.String()))), keys.ErrRecipientSecret},
		{"empty", "", keys.ErrRecipientInvalid},
		{"two keys", pub + "\n" + pub, keys.ErrRecipientInvalid},
		{"comment", "# " + pub, keys.ErrRecipientInvalid},
		{"ssh key", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl", keys.ErrRecipientInvalid},
		{"broken checksum", broken, keys.ErrRecipientInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := keys.ParseRecipient(tt.in)
			require.ErrorIs(t, err, tt.want)
			assert.NotContains(t, err.Error(), identity.String())
		})
	}
	_, err = keys.ParseRecipient(pub)
	require.NoError(t, err)
}
