package config_test

import (
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
)

const (
	goodKeyHex = "9f1c4a7e2b8d06f35a1e9c7b4d2f80a6c3e5b7091d4f6a8e2c0b5d7f9a1e3c6b"
	goodDB     = "postgres://dhow:pw@db.internal:5432/dhow?sslmode=verify-full"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func resolve(m map[string]string) (settingspec.Settings, error) {
	cfg, err := config.Load(env(m))
	if err != nil {
		return settingspec.Settings{}, err
	}
	s, errs := settingspec.Parse(cfg.Settings)
	if len(errs) > 0 {
		return settingspec.Settings{}, settingspec.Join(errs)
	}
	return s, nil
}

func valid() map[string]string {
	return map[string]string{
		config.EnvDatabaseURL: goodDB,
		config.EnvSecretKey:   goodKeyHex,
		config.EnvPublicURL:   "https://dhow.example.com/",
		config.EnvKeyringFile: "/run/secrets/dhow-keyring",
	}
}

func TestLoadValid(t *testing.T) {
	cfg, err := config.Load(env(valid()))
	require.NoError(t, err)
	assert.Equal(t, config.DefaultListenAddr, cfg.ListenAddr)
	assert.Equal(t, "https://dhow.example.com", cfg.PublicURL.String())
	assert.Len(t, cfg.SecretKey, 32)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.False(t, cfg.Dev)
}

func TestLoadAcceptsBase64Key(t *testing.T) {
	raw, err := hex.DecodeString(goodKeyHex)
	require.NoError(t, err)
	tests := []struct {
		name string
		enc  *base64.Encoding
	}{
		{"std", base64.StdEncoding},
		{"raw url", base64.RawURLEncoding},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			m[config.EnvSecretKey] = tt.enc.EncodeToString(raw)
			cfg, err := config.Load(env(m))
			require.NoError(t, err)
			assert.Equal(t, raw, cfg.SecretKey)
		})
	}
}

func TestLoadListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"ipv4", "0.0.0.0:8080", false},
		{"hostname", "localhost:9000", false},
		{"all interfaces", ":8080", false},
		{"ipv6 loopback", "[::1]:8080", false},
		{"ipv6 any", "[::]:8080", false},
		{"ipv6 address", "[2001:db8::1]:443", false},
		{"missing port", "localhost", true},
		{"port only", "8080", true},
		{"port out of range", "localhost:99999", true},
		{"ipv6 without brackets", "::1:8080", true},
		{"ipv6 port out of range", "[::1]:99999", true},
		{"ipv6 port not numeric", "[::1]:http", true},
		{"ipv6 empty port", "[::1]:", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			m[config.EnvListenAddr] = tt.addr
			cfg, err := config.Load(env(m))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "host:port")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.addr, cfg.ListenAddr)
		})
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name    string
		set     map[string]string
		wantErr string
	}{
		{"missing secret", map[string]string{config.EnvSecretKey: ""}, config.EnvSecretKey + ": is required"},
		{"placeholder secret", map[string]string{config.EnvSecretKey: "change_this_secret_in_production_please_123456"}, "placeholder"},
		{"short secret", map[string]string{config.EnvSecretKey: "9f1c4a7e2b8d06f35a1e9c7b4d2f80a6"}, "at least 32 bytes"},
		{"repeated secret", map[string]string{config.EnvSecretKey: strings.Repeat("ab", 32)}, "too little variety"},
		{"one repeated hex character", map[string]string{config.EnvSecretKey: strings.Repeat("a", 64)}, "too little variety"},
		{"one repeated digit", map[string]string{config.EnvSecretKey: strings.Repeat("0", 64)}, "too little variety"},
		{"one repeated base64 character", map[string]string{config.EnvSecretKey: strings.Repeat("Z", 44)}, "too little variety"},
		{"changeme", map[string]string{config.EnvSecretKey: "changeme"}, "placeholder"},
		{"secret", map[string]string{config.EnvSecretKey: "secret"}, "placeholder"},
		{"example", map[string]string{config.EnvSecretKey: "EXAMPLE-KEY-EXAMPLE-KEY-EXAMPLE-KEY"}, "placeholder"},
		{"not encoded secret", map[string]string{config.EnvSecretKey: "this is not hex or base64!!"}, "hex or base64"},
		{"missing database", map[string]string{config.EnvDatabaseURL: ""}, config.EnvDatabaseURL + ": is required"},
		{"dsn instead of url", map[string]string{config.EnvDatabaseURL: "host=db user=dhow"}, "postgres:// URL"},
		{"plaintext database", map[string]string{config.EnvDatabaseURL: "postgres://dhow:pw@db/dhow?sslmode=disable"}, "sslmode"},
		{"default sslmode", map[string]string{config.EnvDatabaseURL: "postgres://dhow:pw@db/dhow"}, "sslmode"},
		{"missing public url", map[string]string{config.EnvPublicURL: ""}, config.EnvPublicURL + ": is required"},
		{"http public url", map[string]string{config.EnvPublicURL: "http://dhow.example.com"}, "must use https"},
		{"relative public url", map[string]string{config.EnvPublicURL: "dhow.example.com"}, "absolute URL"},
		{"public url with query", map[string]string{config.EnvPublicURL: "https://dhow.example.com/?x=1"}, "query"},
		{"bad listen addr", map[string]string{config.EnvListenAddr: "8080"}, "host:port"},
		{"bad proxy", map[string]string{config.EnvTrustedProxies: "10.0.0.0/8, nope"}, `invalid IP "nope"`},
		{"bad cidr", map[string]string{config.EnvTrustedProxies: "10.0.0.0/99"}, "invalid CIDR"},
		{"bad level", map[string]string{config.EnvLogLevel: "loud"}, config.EnvLogLevel},
		{"bad dev flag", map[string]string{config.EnvDev: "maybe"}, config.EnvDev},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			for k, v := range tt.set {
				m[k] = v
			}
			_, err := config.Load(env(m))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestErrorsNeverEchoSecrets(t *testing.T) {
	m := map[string]string{
		config.EnvSecretKey:   "change_me_" + goodKeyHex,
		config.EnvDatabaseURL: "postgres://dhow:SuperSecretPw@db/dhow?sslmode=disable",
		config.EnvPublicURL:   "ftp://x",
	}
	_, err := config.Load(env(m))
	require.Error(t, err)
	for _, leak := range []string{goodKeyHex, "SuperSecretPw"} {
		assert.NotContains(t, err.Error(), leak)
	}
}

func TestDevRelaxesTransportRules(t *testing.T) {
	m := valid()
	m[config.EnvDev] = "true"
	m[config.EnvPublicURL] = "http://localhost:8080"
	m[config.EnvDatabaseURL] = "postgres://dhow@localhost/dhow?sslmode=disable"
	cfg, err := config.Load(env(m))
	require.NoError(t, err)
	assert.True(t, cfg.Dev)

	m[config.EnvSecretKey] = "dev"
	_, err = config.Load(env(m))
	assert.Error(t, err, "a weak secret must be rejected in dev mode")
}

func TestTrustedProxiesParsing(t *testing.T) {
	m := valid()
	m[config.EnvTrustedProxies] = " 10.0.0.0/8 , 192.168.1.10,::1 "
	cfg, err := config.Load(env(m))
	require.NoError(t, err)
	var got []string
	for _, p := range cfg.TrustedProxies {
		got = append(got, p.String())
	}
	assert.Equal(t, []string{"10.0.0.0/8", "192.168.1.10/32", "::1/128"}, got)
}

func TestValidate(t *testing.T) {
	publicURL := &url.URL{Scheme: "https", Host: "dhow.example.com"}
	good := config.Config{
		DatabaseURL: goodDB,
		SecretKey:   []byte(strings.Repeat("k", 32)),
		PublicURL:   publicURL,
		ListenAddr:  config.DefaultListenAddr,
	}
	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr []string
	}{
		{"valid", func(*config.Config) {}, nil},
		{"zero value", func(c *config.Config) { *c = config.Config{} }, []string{
			config.EnvDatabaseURL + ": is required",
			config.EnvSecretKey + ": is required",
			config.EnvPublicURL + ": is required",
			config.EnvListenAddr + ": is required",
		}},
		{"short key", func(c *config.Config) { c.SecretKey = []byte("short-key-material") }, []string{config.EnvSecretKey + ": must be at least 32 bytes"}},
		{"bad listen addr", func(c *config.Config) { c.ListenAddr = "nope" }, []string{config.EnvListenAddr + ": must be host:port"}},
		{"ipv6 listen addr", func(c *config.Config) { c.ListenAddr = "[::1]:8080" }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := good
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
			assert.NotContains(t, err.Error(), "short-key-material")
			assert.NotContains(t, err.Error(), goodDB)
		})
	}
}

func TestLoadListenAddrOnly(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{"default", map[string]string{}, config.DefaultListenAddr, false},
		{"explicit", map[string]string{config.EnvListenAddr: "0.0.0.0:8080"}, "0.0.0.0:8080", false},
		{"invalid", map[string]string{config.EnvListenAddr: "8080"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.LoadListenAddr(env(tt.env))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), config.EnvListenAddr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBreachedPasswordCheck(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    bool
		wantErr bool
	}{
		{"unset defaults to on", "", true, false},
		{"blank defaults to on", "  ", true, false},
		{"true", "true", true, false},
		{"false for air-gapped networks", "false", false, false},
		{"zero", "0", false, false},
		{"one", "1", true, false},
		{"garbage fails closed", "maybe", false, true},
		{"yes is not accepted", "yes", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			m[config.EnvBreachedPasswordCheck] = tt.value
			cfg, err := resolve(m)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), config.EnvBreachedPasswordCheck)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.BreachedPasswordCheck)
		})
	}
}
