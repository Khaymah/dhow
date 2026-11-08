package unit_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

const (
	botToken = "123456789" + ":AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
	botPath  = "/bot" + botToken + "/sendMessage"
)

func hostile() notifications.Payload {
	return notifications.Payload{
		Event: notifications.EventDeploymentFinished, EventID: "e1", Time: "2026-11-07T10:00:00Z", Status: notifications.StatusFailed,
		Project: "shop <b>&</b>", Environment: "production", Ref: "<!channel> @everyone @here <@U123> <https://evil.example|click>",
		Run: 7, URL: "https://dhow.example.com/projects/p/runs/7",
	}
}

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func TestSlackEscapesTheThreeControlCharactersAndNeverSendsABroadcast(t *testing.T) {
	assert.Equal(t, "&amp;&lt;&gt;", notifications.EscapeSlack("&<>"))
	raw, err := notifications.RenderSlack(i18n.En, hostile())
	require.NoError(t, err)
	m := decode(t, raw)
	text, _ := m["text"].(string)
	for _, bad := range []string{"<!channel>", "<!here>", "<@U123>", "<https://evil.example|click>", "<b>"} {
		assert.NotContains(t, text, bad)
	}
	assert.Contains(t, text, "&lt;!channel&gt; @everyone @here &lt;@U123&gt;")
	assert.Contains(t, text, "shop &lt;b&gt;&amp;&lt;/b&gt;")
	assert.Contains(t, text, "*Deployment failed*")
	assert.Equal(t, false, m["unfurl_links"])
}

func TestDiscordParsesNoMentionAndMakesFormattingLiteral(t *testing.T) {
	raw, err := notifications.RenderDiscord(i18n.En, hostile())
	require.NoError(t, err)
	m := decode(t, raw)
	content, _ := m["content"].(string)
	mentions, ok := m["allowed_mentions"].(map[string]any)
	require.True(t, ok, "the mention policy is always sent")
	parse, ok := mentions["parse"].([]any)
	require.True(t, ok, "an empty list, not a missing key, so Discord parses nothing")
	assert.Empty(t, parse)
	assert.Contains(t, string(raw), `"allowed_mentions":{"parse":[]}`)
	assert.Contains(t, content, "@everyone @here")
	assert.NotContains(t, content, "<@U123>")
	assert.NotContains(t, content, "<!channel>")
	assert.Equal(t, `a\_b\*c\~d\|e\`+"`"+`f\[g\]\\`, notifications.EscapeDiscord("a_b*c~d|e`f[g]\\"))
	assert.LessOrEqual(t, len([]rune(content)), 2000)
}

func TestTelegramSendsEscapedHTMLAndEveryUserValueIsInertText(t *testing.T) {
	assert.Equal(t, "&amp;&lt;&gt;\"'", notifications.EscapeTelegram("&<>\"'"))
	raw, err := notifications.RenderTelegram(i18n.Ar, "-100123", hostile())
	require.NoError(t, err)
	m := decode(t, raw)
	text, _ := m["text"].(string)
	assert.Equal(t, "HTML", m["parse_mode"])
	assert.Equal(t, "-100123", m["chat_id"])
	assert.True(t, strings.HasPrefix(text, "<b>فشل النشر</b>"), text)
	withoutOwnTags := strings.NewReplacer("<b>", "", "</b>", "").Replace(text)
	assert.NotContains(t, withoutOwnTags, "<", "the only tag in the message is the bold title")
	assert.NotContains(t, withoutOwnTags, ">")
}

func TestAMessageDropsControlAndBidiOverrideCharactersFromUserValues(t *testing.T) {
	p := hostile()
	p.Ref = "main\u202Egpj.exe\u2066x\u2069\x00y"
	for name, render := range map[string]func() ([]byte, error){
		"slack":    func() ([]byte, error) { return notifications.RenderSlack(i18n.En, p) },
		"discord":  func() ([]byte, error) { return notifications.RenderDiscord(i18n.En, p) },
		"telegram": func() ([]byte, error) { return notifications.RenderTelegram(i18n.En, "1", p) },
	} {
		raw, err := render()
		require.NoError(t, err, name)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		text := fmt.Sprint(m["text"], m["content"])
		for _, r := range []string{"\u202E", "\u2066", "\u2069", "\x00"} {
			assert.NotContains(t, text, r, name)
		}
		assert.Contains(t, text, "maingpj.exe")
	}
}

func TestTheMessageCarriesTheTitleOfTheEventInTheLanguageOfTheChannel(t *testing.T) {
	p := hostile()
	p.Event = notifications.EventApprovalRequested
	p.Status = notifications.StatusAwaitingApprove
	ar, err := notifications.RenderSlack(i18n.Ar, p)
	require.NoError(t, err)
	en, err := notifications.RenderSlack(i18n.En, p)
	require.NoError(t, err)
	assert.Contains(t, string(ar), "نشر ينتظر موافقتك")
	assert.Contains(t, string(en), "A deployment waits for your approval")
}

func parse(t *testing.T, raw string) notifications.Target {
	t.Helper()
	target, err := notifications.ParseTarget(raw, false)
	require.NoError(t, err, raw)
	return target
}

func TestChatURLAcceptsTheCanonicalAddressOfEachServiceOnly(t *testing.T) {
	good := map[string]string{
		models.KindSlack:   "https://hooks.slack.com/services/T1/B2/abc123",
		models.KindDiscord: "https://discord.com/api/webhooks/123456789012345678/abc_DEF-123",
	}
	for kind, raw := range good {
		u, err := notifications.ChatURL(kind, parse(t, raw))
		require.NoError(t, err, kind)
		assert.Equal(t, raw, u.String())
	}
	u, err := notifications.ChatURL(models.KindDiscord, parse(t, "https://DiscordApp.com./api/webhooks/1/a"))
	require.NoError(t, err)
	assert.Equal(t, "https://discordapp.com/api/webhooks/1/a", u.String(), "host is lower case and has no trailing dot")
	u, err = notifications.ChatURL(models.KindSlack, parse(t, "https://hooks.slack.com:443/services/a/b/c"))
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/a/b/c", u.String())

	for kind, raws := range map[string][]string{
		models.KindSlack: {
			"https://hooks.slack.com/", "https://hooks.slack.com/services/", "https://hooks.slack.com/services/a/b", "https://hooks.slack.com/services/a/b/c/d",
			"https://hooks.slack.com/services/a/b/c/", "https://hooks.slack.com/services/a//c", "https://hooks.slack.com/services/a/b/c?x=1",
			"https://hooks.slack.com/services/a/b/c?", "https://hooks.slack.com/services/a/b/%63", "https://hooks.slack.com/services/a/./c",
			"https://hooks.slack.com/services/a/b/c.d", "https://hooks.slack.com:444/services/a/b/c", "https://slack.com/services/a/b/c",
			"https://discord.com/api/webhooks/1/a", "https://hooks.slack.com.evil.example/services/a/b/c",
		},
		models.KindDiscord: {
			"https://discord.com/api/webhooks/1", "https://discord.com/api/webhooks/1/a/b", "https://discord.com/api/v10/webhooks/1/a",
			"https://hooks.slack.com/services/a/b/c", "https://cdn.discordapp.com/api/webhooks/1/a", "https://discord.com/api/webhooks/1/a?wait=true",
		},
	} {
		for _, raw := range raws {
			target, err := notifications.ParseTarget(raw, false)
			if err != nil {
				continue
			}
			_, err = notifications.ChatURL(kind, target)
			assert.ErrorIs(t, err, notifications.ErrHostNotAllowed, "%s %s", kind, raw)
		}
	}
	_, err = notifications.ChatURL(models.KindTelegram, parse(t, "https://api.telegram.org/bot1:abc/sendMessage"))
	assert.ErrorIs(t, err, notifications.ErrHostNotAllowed, "a Telegram channel has no address of its own")
	_, err = notifications.ChatURL(models.KindWebhook, parse(t, "https://hooks.slack.com/services/a/b/c"))
	assert.ErrorIs(t, err, notifications.ErrHostNotAllowed)
	_, err = notifications.ChatURL(models.KindSlack, notifications.Target{})
	assert.ErrorIs(t, err, notifications.ErrHostNotAllowed)
}

func TestBotTokenAndChatIdAreCheckedForShape(t *testing.T) {
	for _, ok := range []string{botToken, "1234:ABCDEFGHIJKLMNOPQRSTUVWX", "5555555555:abc-DEF_ghi-JKL_mno-PQR_stu"} {
		assert.True(t, notifications.ValidBotToken(ok), ok)
	}
	badTokens := []string{
		"", "abc", "123:short", "123456789:AAH dqTcvCH1vGWJxfSeofSAs0K5PALDsaw", "x123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw",
		botToken + "/../x", botToken + "\n", "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw?x=1",
	}
	for _, bad := range badTokens {
		assert.False(t, notifications.ValidBotToken(bad), "%q", bad)
	}
	for _, ok := range []string{"123", "-1001234567890", "@my_channel", "@Dhow_alerts"} {
		assert.True(t, notifications.ValidChatID(ok), ok)
	}
	for _, bad := range []string{"", "abc", "1 2", "@a", "@1abcde", "12/3", "-", "--1", "../x", "1\n"} {
		assert.False(t, notifications.ValidChatID(bad), "%q", bad)
	}
	assert.Equal(t, "https://api.telegram.org"+botPath, notifications.TelegramURL(botToken))
}

func TestRedactErrorStripsTheAddressAndEveryKnownSecretFromATransportError(t *testing.T) {
	address := "https://api.telegram.org" + botPath
	inner := errors.New("dial tcp 10.0.0.1:443: connect: connection refused")
	wrapped := &url.Error{Op: "Post", URL: address, Err: inner}
	twice := &url.Error{Op: "Post", URL: address, Err: fmt.Errorf("round trip: %w", wrapped)}

	for name, err := range map[string]error{"once": wrapped, "twice": twice, "inside a wrapper": fmt.Errorf("send: %w", wrapped)} {
		msg := notifications.RedactError(err, address, botToken)
		assert.NotContains(t, msg, botToken, name)
		assert.NotContains(t, msg, "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", name)
		assert.NotContains(t, msg, "api.telegram.org/bot", name)
		assert.Contains(t, msg, "connection refused", name)
	}

	leaky := fmt.Errorf("could not reach %s%s", "https://api.telegram.org", botPath)
	msg := notifications.RedactError(leaky)
	assert.NotContains(t, msg, botToken, "an address in a plain error text is removed even when no secret was given")
	leaky = errors.New("bad token 123456789%3AAAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw seen")
	assert.NotContains(t, notifications.RedactError(leaky), "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", "a token in escaped form is removed")
	leaky = errors.New("hook token xoxbSLACKTOKEN0123456789ab failed")
	assert.NotContains(t, notifications.RedactError(leaky, "xoxbSLACKTOKEN0123456789ab"), "xoxbSLACKTOKEN")
	assert.Empty(t, notifications.RedactError(nil))
	assert.Equal(t, "short", notifications.RedactError(errors.New("short"), "ab"), "a secret too short to be one is not replaced everywhere")
}
