package edge_test

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
	"github.com/Khaymah/dhow/internal/netguard"
)

func FuzzNotifyURL(f *testing.F) {
	for _, seed := range []string{
		"https://hooks.example.com/services/T/B/X", "https://127.0.0.1/x", "https://[::1]/x", "https://[::ffff:10.0.0.1]/x",
		"https://169.254.169.254/latest/meta-data", "https://2130706433/", "https://0x7f.0.0.1/", "https://0177.0.0.1/", "https://127.1/",
		"https://user:pw@example.com/", "https://example.com@127.0.0.1/", "https://example.com:0/", "https://example.com#frag",
		"http://example.com/", "https://[fe80::1%25eth0]/", "https://localhost./", "https://exämple.com/", "https://a..b/", "",
		"https://\x00example.com/", "https://example.com/\r\nHost: evil", strings.Repeat("https://a.b/", 300),
	} {
		f.Add(seed)
	}
	policy := netguard.Policy{}
	f.Fuzz(func(t *testing.T, raw string) {
		target, err := notifications.ParseTarget(raw, false)
		if err != nil {
			return
		}
		require.LessOrEqual(t, len(raw), config.NotifyURLMax)
		require.Equal(t, "https", target.URL.Scheme)
		require.Empty(t, target.URL.User)
		require.NotEmpty(t, target.Hostname)
		require.Empty(t, target.URL.Fragment)
		require.NotContains(t, target.Display, "/")
		require.NotContains(t, target.Display, "?")
		require.NotContains(t, target.Display, "@")
		for _, r := range target.URL.String() {
			require.Greater(t, r, rune(' '))
			require.Less(t, r, rune(0x7f))
		}
		if target.Literal.IsValid() {
			require.False(t, target.Literal.Is4In6(), "a mapped address is unmapped, so the guard checks IPv4")
			if addr, ok := netip.AddrFromSlice(target.Literal.AsSlice()); ok && addr == netip.MustParseAddr("169.254.169.254") {
				require.Error(t, policy.Check(addr))
			}
			return
		}
		require.False(t, strings.ContainsAny(target.Hostname, "%\\ "), "a host name is a plain name")
		if _, err := netip.ParseAddr(target.Hostname); err == nil {
			t.Fatalf("an IP address was taken for a name: %q", target.Hostname)
		}
	})
}

func FuzzChatURL(f *testing.F) {
	for _, seed := range []string{
		"https://hooks.slack.com/services/T/B/X", "https://discord.com/api/webhooks/1/a", "https://discordapp.com/api/webhooks/1/a",
		"https://hooks.slack.com.evil.example/services/T/B/X", "https://hooks.slack.com@evil.example/services/T/B/X",
		"https://hooks.slack.com/services/../x/y", "https://discord.com/api/webhooks/1/a?wait=true", "https://api.telegram.org/bot1:a/sendMessage",
		"https://hooks.slack.com:8443/services/T/B/X", "https://hooks.slack.com/services/%2e%2e/B/X", "",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		target, err := notifications.ParseTarget(raw, false)
		if err != nil {
			return
		}
		for _, kind := range []string{models.KindSlack, models.KindDiscord, models.KindTelegram, models.KindWebhook} {
			u, err := notifications.ChatURL(kind, target)
			if err != nil {
				continue
			}
			require.Contains(t, notifications.ChatHosts(kind), u.Host, "the address is on a host of the service")
			require.Equal(t, "https", u.Scheme)
			require.Empty(t, u.RawQuery)
			require.Empty(t, u.Fragment)
			require.Empty(t, u.User)
			require.NotContains(t, u.Path, "..")
			require.NotContains(t, u.Path, "//")
			require.Empty(t, u.Port())
		}
	})
}
