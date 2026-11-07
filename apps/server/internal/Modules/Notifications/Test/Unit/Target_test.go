package unit_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
)

func TestParseTargetAcceptsAnHTTPSAddressAndShowsOnlyTheHost(t *testing.T) {
	tests := []struct{ raw, display string }{
		{"https://hooks.example.com/services/T000/B000/XXXX", "hooks.example.com"},
		{"https://Hooks.Example.COM./a?b=c", "hooks.example.com"},
		{"https://hooks.example.com:8443/x", "hooks.example.com:8443"},
		{"https://hooks.example.com:443/x", "hooks.example.com"},
		{"https://203.0.113.9/x", "203.0.113.9"},
		{"https://[2001:db8::1]:9443/x", "[2001:db8::1]:9443"},
	}
	for _, tt := range tests {
		target, err := notifications.ParseTarget(tt.raw, false)
		require.NoError(t, err, tt.raw)
		assert.Equal(t, tt.display, target.Display, tt.raw)
		assert.NotContains(t, target.Display, "services", "the display never carries the path")
	}
}

func TestParseTargetRefusesWhatCouldReachTheWrongPlace(t *testing.T) {
	tests := map[string]error{
		"":                                         notifications.ErrURLInvalid,
		"hooks.example.com/x":                      notifications.ErrURLScheme,
		"http://hooks.example.com/x":               notifications.ErrURLScheme,
		"ftp://hooks.example.com/x":                notifications.ErrURLScheme,
		"https://user:pw@hooks.example.com":        notifications.ErrURLUserinfo,
		"https://user@hooks.example.com":           notifications.ErrURLUserinfo,
		"https://hooks.example.com@evil.example/x": notifications.ErrURLUserinfo,
		"https:///x":                               notifications.ErrURLHost,
		"https://.example.com/x":                   notifications.ErrURLHost,
		"https://a..example.com/x":                 notifications.ErrURLHost,
		"https://hooks.example.com:0/x":            notifications.ErrURLHost,
		"https://hooks.example.com:99999/x":        notifications.ErrURLHost,
		"https://fe80::1%25eth0/x":                 notifications.ErrURLInvalid,
		"https://2130706433/x":                     notifications.ErrURLNumeric,
		"https://0x7f000001/x":                     notifications.ErrURLNumeric,
		"https://0x7f.1/x":                         notifications.ErrURLNumeric,
		"https://127.1/x":                          notifications.ErrURLNumeric,
		"https://017700000001/x":                   notifications.ErrURLNumeric,
		"https://example.com/x#frag":               notifications.ErrURLInvalid,
		"https://example.com/ x":                   notifications.ErrURLInvalid,
		"https://example.com/\nx":                  notifications.ErrURLInvalid,
		"https://exämple.com/x":                    notifications.ErrURLInvalid,
		"mailto:a@example.com":                     notifications.ErrURLScheme,
		"https:example.com":                        notifications.ErrURLInvalid,
	}
	for raw, want := range tests {
		_, err := notifications.ParseTarget(raw, false)
		assert.ErrorIs(t, err, want, "%q", raw)
	}
	_, err := notifications.ParseTarget("https://example.com/"+strings.Repeat("a", 3000), false)
	assert.ErrorIs(t, err, notifications.ErrURLTooLong)
}

func TestParseTargetAllowsPlainHTTPOnlyWhenTheServerRunsInDevelopment(t *testing.T) {
	_, err := notifications.ParseTarget("http://hooks.example.com/x", true)
	require.NoError(t, err)
	_, err = notifications.ParseTarget("http://hooks.example.com/x", false)
	assert.ErrorIs(t, err, notifications.ErrURLScheme)
}

func TestParseTargetKeepsIPLiteralsForTheNetworkGuard(t *testing.T) {
	target, err := notifications.ParseTarget("https://[::ffff:127.0.0.1]/x", false)
	require.NoError(t, err)
	assert.Equal(t, netip.MustParseAddr("127.0.0.1"), target.Literal, "an IPv4-mapped address is unmapped, so the guard sees IPv4")
}
