package unit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	secrets "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets"
)

func TestNames(t *testing.T) {
	for name, ok := range map[string]bool{
		"TOKEN": true, "_x": true, "a1_B2": true, "1TOKEN": false, "": false, "WITH-DASH": false, "space x": false,
		"A234567890123456789012345678901234567890123456789012345678901234":  true,
		"A2345678901234567890123456789012345678901234567890123456789012345": false,
	} {
		assert.Equal(t, ok, secrets.ValidName(name), name)
	}
	for name, reserved := range map[string]bool{"DHOW_TOKEN": true, "dhow_x": true, "DHOWTOKEN": false, "MY_DHOW_X": false} {
		assert.Equal(t, reserved, secrets.Reserved(name), name)
	}
}

func TestRegistryHostNormalization(t *testing.T) {
	for raw, want := range map[string]string{
		"docker.io": "registry-1.docker.io", "index.docker.io": "registry-1.docker.io", "docker.io:443": "docker.io:443",
		" GHCR.io ": "ghcr.io", "registry.example.com:5000": "registry.example.com:5000", "xn--bcher-kva.example": "xn--bcher-kva.example",
	} {
		got, ok := secrets.NormalizeHost(raw)
		assert.True(t, ok, raw)
		assert.Equal(t, want, got, raw)
	}
	for _, raw := range []string{
		"", "localhost", "10.0.0.1", "[::1]", "::1", "https://ghcr.io", "ghcr.io/team", "user@ghcr.io", "*.example.com",
		"ghcr.io:0", "ghcr.io:65536", "ghcr.io:08", "bücher.example", "ghcr.io?x", "ghcr.io#x",
	} {
		_, ok := secrets.NormalizeHost(raw)
		assert.False(t, ok, raw)
	}
}
