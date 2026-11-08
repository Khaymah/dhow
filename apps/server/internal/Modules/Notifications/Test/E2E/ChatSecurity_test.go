package e2e_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

func (c *chatWorld) createRaw(body map[string]any, language string) apptest.Response {
	c.t.Helper()
	body["CurrentFactor"] = c.factor()
	return c.owner.Do(apptest.Request{
		Method: http.MethodPost, Path: c.projectPath("/notification-channels"), Body: body, Headers: map[string]string{"Accept-Language": language},
	})
}

func TestAChatChannelAcceptsOnlyTheHostsOfItsOwnService(t *testing.T) {
	c := newChatWorld(t)
	notAllowed := map[string]map[string]any{
		"slack on another host":        {"Kind": "slack", "Name": "a", "Url": "https://evil.example/services/T01/B02/xoxbSLACKTOKEN0123456789ab"},
		"slack on a lookalike host":    {"Kind": "slack", "Name": "b", "Url": "https://hooks.slack.com.evil.example/services/T01/B02/abc"},
		"slack on a subdomain":         {"Kind": "slack", "Name": "c", "Url": "https://x.hooks.slack.com/services/T01/B02/abc"},
		"slack with a user name":       {"Kind": "slack", "Name": "d", "Url": "https://hooks.slack.com@evil.example/services/T01/B02/abc"},
		"slack over plain http":        {"Kind": "slack", "Name": "e", "Url": "http://hooks.slack.com/services/T01/B02/abc"},
		"slack on a port":              {"Kind": "slack", "Name": "f", "Url": "https://hooks.slack.com:8443/services/T01/B02/abc"},
		"slack with another path":      {"Kind": "slack", "Name": "g", "Url": "https://hooks.slack.com/api/chat.postMessage"},
		"slack with a dot segment":     {"Kind": "slack", "Name": "h", "Url": "https://hooks.slack.com/services/../api/x/y"},
		"slack with a query":           {"Kind": "slack", "Name": "i", "Url": slackURL + "?x=1"},
		"slack with an encoded path":   {"Kind": "slack", "Name": "j", "Url": "https://hooks.slack.com/services/T01/B02/%61bc"},
		"slack with too few parts":     {"Kind": "slack", "Name": "k", "Url": "https://hooks.slack.com/services/T01/B02"},
		"slack with a discord host":    {"Kind": "slack", "Name": "l", "Url": discordURL},
		"discord on a slack host":      {"Kind": "discord", "Name": "m", "Url": slackURL},
		"discord on another host":      {"Kind": "discord", "Name": "n", "Url": "https://evil.example" + discordPath},
		"discord on a lookalike host":  {"Kind": "discord", "Name": "o", "Url": "https://discord.com.evil.example" + discordPath},
		"discord with another path":    {"Kind": "discord", "Name": "p", "Url": "https://discord.com/api/v10/users/@me"},
		"discord with an ip address":   {"Kind": "discord", "Name": "q", "Url": "https://162.159.135.232" + discordPath},
		"discord with a trailing part": {"Kind": "discord", "Name": "r", "Url": discordURL + "/slack"},
	}
	for name, body := range notAllowed {
		res := c.createRaw(body, "en")
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s: %s", name, res.Raw)
		require.NotNil(t, res.Error, name)
		assert.Equal(t, "CHANNEL_HOST_NOT_ALLOWED", res.Error.Code, name)
	}
	assert.Zero(t, c.count(`SELECT count(*) FROM "NotificationChannels" WHERE "Kind" IN ('slack', 'discord')`), "nothing was saved")

	for name, url := range map[string]string{
		"discord.com": discordURL, "discordapp.com": "https://discordapp.com" + discordPath,
	} {
		res := c.createRaw(map[string]any{"Kind": "discord", "Name": name, "Url": url}, "en")
		assert.Equal(t, http.StatusCreated, res.Status, "%s: %s", name, res.Raw)
	}
}

func TestTheRefusalOfAForeignHostIsTranslatedAndTellsHowToFixIt(t *testing.T) {
	c := newChatWorld(t)
	for language, want := range map[string]string{"en": "channel address is not accepted", "ar": "عنوان القناة غير مقبول"} {
		res := c.createRaw(map[string]any{"Kind": "slack", "Name": "x", "Url": "https://evil.example/services/a/b/c"}, language)
		require.Equal(t, http.StatusUnprocessableEntity, res.Status, string(res.Raw))
		var body struct {
			Error struct{ Code, Message, Fix string }
		}
		require.NoError(t, json.Unmarshal(res.Raw, &body))
		assert.Equal(t, "CHANNEL_HOST_NOT_ALLOWED", body.Error.Code)
		assert.Contains(t, body.Error.Message, want, language)
		assert.NotEmpty(t, body.Error.Fix, language)
		assert.Contains(t, body.Error.Fix, "hooks.slack.com", language)
	}
}

func TestEachKindRefusesTheFieldsOfAnotherKind(t *testing.T) {
	c := newChatWorld(t)
	bad := map[string]map[string]any{
		"telegram without a token":   {"Kind": "telegram", "Name": "a", "ChatId": telegramChat},
		"telegram without a chat":    {"Kind": "telegram", "Name": "b", "BotToken": telegramToken},
		"telegram with a bad token":  {"Kind": "telegram", "Name": "c", "BotToken": "not-a-token", "ChatId": telegramChat},
		"telegram with a bad chat":   {"Kind": "telegram", "Name": "d", "BotToken": telegramToken, "ChatId": "../x"},
		"telegram with an address":   {"Kind": "telegram", "Name": "e", "BotToken": telegramToken, "ChatId": telegramChat, "Url": slackURL},
		"slack with a token":         {"Kind": "slack", "Name": "f", "Url": slackURL, "BotToken": telegramToken},
		"slack with a chat":          {"Kind": "slack", "Name": "g", "Url": slackURL, "ChatId": telegramChat},
		"slack without an address":   {"Kind": "slack", "Name": "h"},
		"email with a token":         {"Kind": "email", "Name": "i", "BotToken": telegramToken},
		"webhook with a chat":        {"Kind": "webhook", "Name": "j", "Url": hookURL, "ChatId": telegramChat},
		"discord without an address": {"Kind": "discord", "Name": "k"},
	}
	for name, body := range bad {
		res := c.createRaw(body, "en")
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s: %s", name, res.Raw)
		require.NotNil(t, res.Error, name)
		assert.Equal(t, "VALIDATION_FAILED", res.Error.Code, name)
	}
	for _, token := range []string{telegramToken, "not-a-token"} {
		for name, body := range bad {
			res := c.createRaw(body, "en")
			assert.NotContains(t, string(res.Raw), token, "%s echoes a token back", name)
		}
	}
}

func TestAChannelIsChangedOnlyWithFieldsOfItsKind(t *testing.T) {
	c := newChatWorld(t)
	slack := c.channel("slack", "s", "en")
	telegram := c.channel("telegram", "t", "en")
	slackChannel := c.projectPath("/notification-channels/" + slack.Channel.Id)
	telegramChannel := c.projectPath("/notification-channels/" + telegram.Channel.Id)

	for name, tc := range map[string]struct {
		path string
		body map[string]any
	}{
		"slack rotates a secret":     {slackChannel, map[string]any{"RotateSecret": true}},
		"slack takes a token":        {slackChannel, map[string]any{"BotToken": telegramToken}},
		"slack takes a foreign url":  {slackChannel, map[string]any{"Url": "https://evil.example/services/a/b/c"}},
		"telegram takes an address":  {telegramChannel, map[string]any{"Url": slackURL}},
		"telegram rotates":           {telegramChannel, map[string]any{"RotateSecret": true}},
		"telegram takes a bad chat":  {telegramChannel, map[string]any{"ChatId": "a b"}},
		"telegram takes a bad token": {telegramChannel, map[string]any{"BotToken": "x"}},
	} {
		body := tc.body
		body["Name"], body["Locale"], body["Version"] = "x", "en", 1
		res := c.put(tc.path, body)
		assert.GreaterOrEqual(t, res.Status, 400, "%s: %s", name, res.Raw)
		assert.Less(t, res.Status, 500, name)
	}

	other := "987654321:BBHdqTcvCH1vGWJxfSeofSAs0K5PALDsbx"
	res := c.put(telegramChannel, map[string]any{"Name": "t", "Locale": "en", "Version": telegram.Channel.Version, "BotToken": other})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.NotContains(t, string(res.Raw), other)
	c.deploymentEvent(eventbus.DeploymentFinished, "failed")
	c.drain()
	require.NotEmpty(t, c.rec.Messages())
	for _, m := range c.rec.Messages() {
		if m.Service == "telegram" {
			assert.Equal(t, telegramChat, m.ChatID, "the chat stayed the same when only the token changed")
		}
	}
}

func TestNoTokenOrWebhookAddressIsAnywhereButInTheSealedColumn(t *testing.T) {
	c := newChatWorld(t)
	slack := c.channel("slack", "s", "en")
	discord := c.channel("discord", "d", "ar")
	telegram := c.channel("telegram", "t", "en")
	c.hostileRun()
	responses := [][]byte{}
	for _, id := range []string{slack.Channel.Id, discord.Channel.Id, telegram.Channel.Id} {
		responses = append(responses, c.owner.Get(c.projectPath("/notification-channels/"+id)).Raw)
		responses = append(responses, c.owner.Do(apptest.Request{Method: http.MethodPost, Path: c.projectPath("/notification-channels/" + id + "/test")}).Raw)
	}
	responses = append(responses, c.owner.Get(c.projectPath("/notification-channels")).Raw, c.owner.Get(c.projectPath("/notification-rules")).Raw)
	c.rec.Script("telegram", 500, 500)
	c.deploymentEvent(eventbus.DeploymentFinished, "failed")
	c.drain()
	c.env.Clock.Advance(time.Minute)
	c.drain()
	responses = append(responses, c.owner.Get(c.projectPath("/notification-deliveries")).Raw)

	needles := map[string][]byte{
		"slack address": []byte(slackURL), "slack path": []byte(slackPath), "slack token": []byte("xoxbSLACKTOKEN0123456789ab"),
		"discord address": []byte(discordURL), "discord token": []byte("DISCORDTOKEN_abc-123_xyz"),
		"bot token": []byte(telegramToken), "bot secret": []byte("AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"), "bot id": []byte("123456789:"),
	}
	for _, raw := range responses {
		for name, needle := range needles {
			assert.False(t, bytes.Contains(raw, needle), "a response carries the %s", name)
		}
	}
	for table, content := range dumpEverything(t, c.world) {
		for name, needle := range needles {
			assert.False(t, bytes.Contains(content, needle), "%s holds the %s in clear text", table, name)
			assert.False(t, bytes.Contains(content, []byte(hex.EncodeToString(needle))), "%s holds the %s as hex", table, name)
		}
	}
}

func TestAConnectionErrorNeverCarriesTheBotTokenOutOfTheDeliverer(t *testing.T) {
	c := newChatWorld(t)
	c.channel("telegram", "t", "en")
	c.channel("slack", "s", "en")
	c.channel("discord", "d", "en")
	c.srv.Close()
	c.deploymentEvent(eventbus.DeploymentFinished, "failed")
	c.drain()

	list := c.deliveries(c.projectPath("/notification-deliveries"))
	require.Len(t, list, 3)
	for _, d := range list {
		assert.Equal(t, "connect", d.LastError, d.ChannelName)
		assert.Equal(t, "pending", d.State, "a connection error is worth another try")
	}
	logs := c.env.Logs.String()
	assert.Contains(t, logs, "notification not delivered", "the failure is logged")
	for name, needle := range map[string]string{
		"bot token": telegramToken, "bot secret": "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", "bot path": "/bot" + telegramToken, "slack path": slackPath,
		"slack token": "xoxbSLACKTOKEN0123456789ab", "discord path": discordPath, "discord token": "DISCORDTOKEN_abc-123_xyz",
		"sendMessage address": "https://api.telegram.org/bot",
	} {
		assert.NotContains(t, logs, needle, "the log carries the %s", name)
	}
	for table, content := range dumpEverything(t, c.world) {
		assert.False(t, bytes.Contains(content, []byte("AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw")), table)
	}
}
