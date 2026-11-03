package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func TestKeyringPathSources(t *testing.T) {
	credDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(credDir, config.CredentialKeyring), []byte("x"), 0o600))
	emptyDir := t.TempDir()
	tests := []struct {
		name    string
		env     map[string]string
		want    config.Keyring
		wantErr string
	}{
		{"file", map[string]string{config.EnvKeyringFile: "/etc/dhow/keyring"}, config.Keyring{Path: "/etc/dhow/keyring"}, ""},
		{
			"credential",
			map[string]string{config.EnvCredentialsDirectory: credDir},
			config.Keyring{Path: filepath.Join(credDir, config.CredentialKeyring), Credential: true},
			"",
		},
		{"credential directory without the keyring", map[string]string{config.EnvCredentialsDirectory: emptyDir}, config.Keyring{}, "is required"},
		{"missing", map[string]string{}, config.Keyring{}, "is required"},
		{"both", map[string]string{config.EnvKeyringFile: "/etc/dhow/keyring", config.EnvCredentialsDirectory: credDir}, config.Keyring{}, "more than once"},
		{"relative", map[string]string{config.EnvKeyringFile: "secrets/dhow-keyring"}, config.Keyring{}, "absolute"},
		{"not clean", map[string]string{config.EnvKeyringFile: "/etc/dhow/../dhow/keyring"}, config.Keyring{}, "clean"},
		{"value in the environment", map[string]string{"DHOW_KEYRING": "kek 1 abc", config.EnvKeyringFile: "/etc/dhow/keyring"}, config.Keyring{}, "never read from an environment variable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.LoadKeyring(env(tt.env))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), "kek 1 abc")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadRequiresTheKeyring(t *testing.T) {
	vars := valid()
	delete(vars, config.EnvKeyringFile)
	_, err := config.Load(env(vars))
	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvKeyringFile+": is required")

	cfg, err := config.Load(env(valid()))
	require.NoError(t, err)
	assert.Equal(t, "/run/secrets/dhow-keyring", cfg.Keyring.Path)
}
