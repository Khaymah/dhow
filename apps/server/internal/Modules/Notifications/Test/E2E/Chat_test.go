package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	notifyreceiver "github.com/Khaymah/dhow/deploy/e2e-targets/notify-receiver/receiver"
)

type chatCase struct {
	kind, locale, service, title string
}

func chatCases() []chatCase {
	return []chatCase{
		{"slack", "ar", notifyreceiver.ServiceSlack, "فشل النشر"},
		{"slack", "en", notifyreceiver.ServiceSlack, "Deployment failed"},
		{"discord", "ar", notifyreceiver.ServiceDiscord, "فشل النشر"},
		{"discord", "en", notifyreceiver.ServiceDiscord, "Deployment failed"},
		{"telegram", "ar", notifyreceiver.ServiceTelegram, "فشل النشر"},
		{"telegram", "en", notifyreceiver.ServiceTelegram, "Deployment failed"},
	}
}

func TestAFailedDeploymentReachesEachChatServiceInTheLanguageOfTheChannel(t *testing.T) {
	for _, tc := range chatCases() {
		t.Run(tc.kind+"-"+tc.locale, func(t *testing.T) {
			c := newChatWorld(t)
			created := c.channel(tc.kind, "chat", tc.locale)
			assert.True(t, created.Channel.HasSecret)
			assert.Empty(t, created.Secret, "a chat channel has no signing secret")
			c.hostileRun()

			c.deploymentEvent(eventbus.DeploymentFinished, "failed")
			c.drain()

			msgs := c.rec.Messages()
			require.Len(t, msgs, 1)
			m := msgs[0]
			assert.Equal(t, tc.service, m.Service)
			assert.Contains(t, m.Text, tc.title)
			assert.Contains(t, m.Text, "production")
			assert.Contains(t, m.Text, "@everyone", "the word stays visible as text")
			assert.Contains(t, m.Text, "@here")
			assert.Contains(t, m.Text, apptest.PublicOrigin+"/projects/"+c.project.String()+"/runs/1")
			assert.NotContains(t, m.Text, "<!channel>", "no live broadcast token")
			assert.NotContains(t, m.Text, "<@123456789>")

			switch tc.kind {
			case "slack":
				assert.Contains(t, m.Text, "&lt;!channel&gt;")
				assert.Contains(t, m.Text, "&lt;@123456789&gt;")
			case "discord":
				assert.True(t, m.HasMentions)
				assert.Empty(t, m.MentionsParse, "allowed_mentions.parse is empty, so nobody is pinged")
				assert.Contains(t, m.Text, `\*x\*`)
				assert.Contains(t, m.Text, `\[y\]`)
			case "telegram":
				assert.Equal(t, "HTML", m.ParseMode)
				assert.Equal(t, telegramChat, m.ChatID)
				assert.Contains(t, m.Text, "&lt;!channel&gt;")
			}

			list := c.deliveries(c.projectPath("/notification-deliveries"))
			require.Len(t, list, 1)
			assert.Equal(t, "sent", list[0].State)
			require.NotNil(t, list[0].LastStatus)
			assert.Less(t, *list[0].LastStatus, 300)
		})
	}
}

func TestATestMessageReachesAChatServiceInTheChannelLanguage(t *testing.T) {
	c := newChatWorld(t)
	created := c.channel("telegram", "chat", "en")
	res := c.owner.Do(apptest.Request{Method: http.MethodPost, Path: c.projectPath("/notification-channels/" + created.Channel.Id + "/test")})
	require.Equal(t, http.StatusAccepted, res.Status, string(res.Raw))
	c.drain()
	msgs := c.rec.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Text, "Test message from Dhow")

	updated := c.put(c.projectPath("/notification-channels/"+created.Channel.Id), map[string]any{
		"Name": "chat", "Locale": "ar", "Version": created.Channel.Version,
	})
	require.Equal(t, http.StatusOK, updated.Status, string(updated.Raw))
	c.owner.Do(apptest.Request{Method: http.MethodPost, Path: c.projectPath("/notification-channels/" + created.Channel.Id + "/test")})
	c.drain()
	msgs = c.rec.Messages()
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[1].Text, "رسالة اختبار من داو")
}

func TestAChatServiceThatAnswersWithAnErrorIsRetriedOrStoppedByItsStatus(t *testing.T) {
	c := newChatWorld(t)
	c.channel("telegram", "chat", "en")
	c.rec.Script(notifyreceiver.ServiceTelegram, http.StatusTooManyRequests)
	c.deploymentEvent(eventbus.DeploymentFinished, "failed")
	c.drain()
	list := c.deliveries(c.projectPath("/notification-deliveries"))
	require.Len(t, list, 1)
	assert.Equal(t, "pending", list[0].State, "429 is worth another try")
	assert.Equal(t, "http_status", list[0].LastError)
	c.env.Clock.Advance(time.Minute)
	c.drain()
	assert.Equal(t, "sent", c.deliveries(c.projectPath("/notification-deliveries"))[0].State)
	assert.Len(t, c.rec.Messages(), 1)

	c.rec.Script(notifyreceiver.ServiceTelegram, http.StatusUnauthorized)
	c.deploymentEvent(eventbus.DeploymentFinished, "failed")
	c.drain()
	attempts := c.rec.Attempts(notifyreceiver.ServiceTelegram)
	c.env.Clock.Advance(3 * time.Hour)
	c.drain()
	assert.Equal(t, attempts, c.rec.Attempts(notifyreceiver.ServiceTelegram), "a revoked token is final: no second attempt")
}

func TestAChannelOfEachKindListsItsServiceHostAndNothingSecret(t *testing.T) {
	c := newChatWorld(t)
	c.channel("slack", "s", "en")
	c.channel("discord", "d", "en")
	c.channel("telegram", "t", "en")
	res := c.owner.Get(c.projectPath("/notification-channels"))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var list []channelView
	res.Decode(t, &list)
	hosts := map[string]string{}
	for _, ch := range list {
		hosts[ch.Kind] = ch.Host
		assert.True(t, ch.HasSecret, ch.Kind)
	}
	assert.Equal(t, map[string]string{"slack": "hooks.slack.com", "discord": "discord.com", "telegram": "api.telegram.org"}, hosts)
	for _, secret := range []string{slackPath, discordPath, telegramToken, "xoxb", "DISCORDTOKEN", "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"} {
		assert.False(t, strings.Contains(string(res.Raw), secret), "the list carries %q", secret)
	}
}
