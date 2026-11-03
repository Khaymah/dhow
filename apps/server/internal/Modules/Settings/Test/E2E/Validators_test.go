package e2e_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
)

const (
	goodKeyHex = "9f1c4a7e2b8d06f35a1e9c7b4d2f80a6c3e5b7091d4f6a8e2c0b5d7f9a1e3c6b"
	goodDB     = "postgres://dhow:pw@db.internal:5432/dhow?sslmode=verify-full"
)

type badValue struct {
	name   string
	env    string
	group  string
	field  string
	envRaw string
	api    any
	rule   string
	extra  map[string]any
}

func badValues() []badValue {
	mail := map[string]any{"Host": "smtp.example.com", "From": "dhow@example.com"}
	return []badValue{
		{"a host with a port", settingspec.EnvSMTPHost, "mail", "Host", "smtp.example.com:25", "smtp.example.com:25", "smtpHost", map[string]any{"From": "dhow@example.com"}},
		{"a port out of range", settingspec.EnvSMTPPort, "mail", "Port", "70000", 70000, "smtpPort", mail},
		{"a sender that is not an address", settingspec.EnvSMTPFrom, "mail", "From", "not an address", "not an address", "smtpFrom", map[string]any{"Host": "smtp.example.com"}},
		{"a sender with a header break", settingspec.EnvSMTPFrom, "mail", "From", "a@example.com\r\nBcc: x@example.com", "a@example.com\r\nBcc: x@example.com", "smtpFrom", map[string]any{"Host": "smtp.example.com"}},
		{"a public limit of zero", settingspec.EnvRateLimitPublic, "rate-limits", "Public", "0", 0, "rateLimit", nil},
		{"an invitation limit past the bound", settingspec.EnvRateLimitInvitesOrg, "rate-limits", "InvitesPerOrganization", "100001", 100001, "rateLimit", nil},
		{"retention of zero days", settingspec.EnvLogRetentionDays, "logs", "RetentionDays", "0", 0, "retentionDays", nil},
		{"a link-local host", settingspec.EnvSMTPHost, "mail", "Host", "169.254.169.254", "169.254.169.254", "smtpHostBlocked", map[string]any{"From": "dhow@example.com"}},
		{"a cloud metadata name", settingspec.EnvSMTPHost, "mail", "Host", "Metadata.Google.Internal.", "Metadata.Google.Internal.", "smtpHostBlocked", map[string]any{"From": "dhow@example.com"}},
		{"the Alibaba metadata address", settingspec.EnvSMTPHost, "mail", "Host", "100.100.100.200", "100.100.100.200", "smtpHostBlocked", map[string]any{"From": "dhow@example.com"}},
		{"a log cap under 1 MiB", settingspec.EnvJobLogMaxBytes, "logs", "JobLogMaxBytes", "65536", 65536, "jobLogMaxBytes", nil},
	}
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestEveryValidatorRunsOnTheEnvironmentAndOnASave(t *testing.T) {
	env := apptest.New(t)
	owner, _, fresh := ownerWithTotp(t, env)
	for _, tt := range badValues() {
		t.Run(tt.name+"/env", func(t *testing.T) {
			vars := map[string]string{config.EnvDatabaseURL: goodDB, config.EnvSecretKey: goodKeyHex, config.EnvPublicURL: "https://dhow.example.com", config.EnvKeyringFile: "/run/secrets/dhow-keyring", tt.env: tt.envRaw}
			_, err := config.Load(envOf(vars))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.env)
		})
		t.Run(tt.name+"/database", func(t *testing.T) {
			fields := map[string]any{tt.field: tt.api}
			for k, v := range tt.extra {
				fields[k] = v
			}
			version := readSettings(t, owner).version(groupKeys[tt.group])
			res := put(owner, tt.group, version, fresh(), fields)
			require.Equal(t, http.StatusBadRequest, res.Status, string(res.Raw))
			assert.Equal(t, "SETTING_INVALID", errorCode(res))
			require.NotEmpty(t, res.Error.Details)
			assert.Equal(t, tt.field, res.Error.Details[0].Field)
			assert.NotEmpty(t, res.Error.Details[0].Message)
			assert.False(t, strings.HasPrefix(res.Error.Details[0].Message, "settings.invalid."), "the reason is translated: %s", res.Error.Details[0].Message)
		})
	}
	assert.Zero(t, env.AuditCount("system.settings.changed"))
}

func TestCrossFieldMailRulesRunOnTheMergedValues(t *testing.T) {
	env := apptest.New(t)
	owner, _, fresh := ownerWithTotp(t, env)
	tests := []struct {
		name   string
		fields map[string]any
		field  string
	}{
		{"a sender is required with a host", map[string]any{"Host": "smtp.example.com"}, "From"},
		{"a username without a password", map[string]any{"Host": "smtp.example.com", "From": "dhow@example.com", "Username": "dhow"}, "Username"},
		{"a password without TLS", map[string]any{"Host": "relay.internal", "From": "dhow@example.com", "Tls": "none", "Username": "dhow", "Password": "relay-credential"}, "Tls"},
		{"other settings without a host", map[string]any{"Username": "dhow", "Password": "relay-credential"}, "Host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := put(owner, "mail", 0, fresh(), tt.fields)
			require.Equal(t, http.StatusBadRequest, res.Status, string(res.Raw))
			fields := []string{}
			for _, d := range res.Error.Details {
				fields = append(fields, d.Field)
			}
			assert.Contains(t, fields, tt.field)
			assert.NotContains(t, string(res.Raw), "relay-credential")
		})
	}
	v := save(t, owner, "mail", fresh(), map[string]any{"Host": "localhost", "From": "Dhow <dhow@example.com>", "Tls": "none", "Port": 25})
	assert.Equal(t, "localhost", v.field(t, "Mail", "Host").text(t))
	assert.Equal(t, "Dhow <dhow@example.com>", v.field(t, "Mail", "From").text(t))
	assert.Equal(t, int64(25), v.field(t, "Mail", "Port").number(t))
	assert.Equal(t, "database", v.field(t, "Mail", "Tls").Source)
	v = save(t, owner, "mail", fresh(), map[string]any{"Host": "", "From": "", "Tls": "starttls", "Port": 587})
	assert.Equal(t, "default", v.field(t, "Mail", "Host").Source, "an empty string removes the saved value")
}
