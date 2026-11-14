package leaksweep_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/leaksweep"
)

const (
	slackPath   = "T0DEMO0001/B0DEMO0001/" + "Xk3mP9qR2sT5vW8yZ1aB4cD7"
	slackURL    = "https://hooks.slack.com/services/" + slackPath
	discordURL  = "https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz0123456789ABCD_-"
	botToken    = "123456789" + ":AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw0"
	signingText = "dhow_whsec_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-AbCdE"
)

func patternByName(t *testing.T, name string) leaksweep.Pattern {
	t.Helper()
	for _, p := range leaksweep.NotifyPatterns() {
		if p.Name == name {
			return p
		}
	}
	require.FailNow(t, "no pattern named "+name)
	return leaksweep.Pattern{}
}

func TestEachNotificationPatternMatchesItsOwnShapeInEveryPlaceItCanAppear(t *testing.T) {
	for _, tc := range []struct {
		name  string
		texts []string
	}{
		{leaksweep.NameSlackWebhook, []string{slackURL, `{"url":"` + slackURL + `"}`, "hooks.slack.com/services/" + slackPath, `https:\/\/hooks.slack.com\/services\/` + strings.ReplaceAll(slackPath, "/", `\/`)}},
		{leaksweep.NameDiscordWebhook, []string{discordURL, "https://discordapp.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz0123456789ABCD_-"}},
		{leaksweep.NameTelegramBot, []string{botToken, "POST /bot" + botToken + "/sendMessage", `{"token":"` + botToken + `"}`, "token=" + botToken}},
		{leaksweep.NameWebhookSecret, []string{signingText, `{"Secret":"` + signingText + `"}`, "Dhow-Secret: " + signingText}},
	} {
		p := patternByName(t, tc.name)
		for _, text := range tc.texts {
			assert.True(t, p.Re.MatchString(text), "%s in %q", tc.name, text)
		}
	}
}

func TestTheNotificationPatternsLeaveOrdinaryTextAlone(t *testing.T) {
	ordinary := []string{
		"hooks.slack.com",
		"https://hooks.slack.com/services/",
		"https://hooks.slack.com/services/T1/B2/short",
		"https://discord.com/api/webhooks/1/short",
		"12345:short",
		"1234567890:" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"dhow_whsec_",
		"dhow_whsec_tooshort",
		"dhow_ing_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-AbCdE",
		"checksum sha256:3a4264520e121176619f54ff867d12774bc81e95e7b84db58d63bccdb6144b81",
		"line 49999\nline 50000\n<img src=x onerror=alert(1)>\n\x1b[31mred\x1b[0m",
	}
	for _, p := range leaksweep.NotifyPatterns() {
		for _, text := range ordinary {
			assert.False(t, p.Re.MatchString(text), "%s must not match %q", p.Name, text)
		}
	}
}

func TestAChannelOfEachKindGivesNeedlesForItsSecretsOnly(t *testing.T) {
	cases := []struct {
		name  string
		plain string
		want  []string
	}{
		{"webhook", `{"Url":"https://webhook.notify.test/hooks/demo","Secret":"` + signingText + `"}`, []string{"webhook secret", "webhook address"}},
		{"slack", `{"Url":"` + slackURL + `"}`, []string{"slack address", "slack address path"}},
		{"telegram", `{"BotToken":"` + botToken + `","ChatId":"-1001234567890"}`, []string{"telegram bot token"}},
		{"empty", `{}`, nil},
	}
	for _, tc := range cases {
		needles, err := leaksweep.ChannelNeedles(tc.name, []byte(tc.plain))
		require.NoError(t, err, tc.name)
		var names []string
		for _, n := range needles {
			names = append(names, n.Name)
			assert.Empty(t, n.Allowed, "no place may hold a channel secret in clear")
		}
		assert.ElementsMatch(t, tc.want, names, tc.name)
	}
	_, err := leaksweep.ChannelNeedles("bad", []byte("not json"))
	require.Error(t, err)
}

func TestAChannelSecretPlantedInAnyPlaceIsFoundAndNothingElseIs(t *testing.T) {
	needles, err := leaksweep.ChannelNeedles("channel", []byte(`{"Url":"`+slackURL+`","BotToken":"`+botToken+`","Secret":"`+signingText+`"}`))
	require.NoError(t, err)
	places := map[string]string{
		"logs/server.log":                    `{"msg":"sent","url":"` + slackURL + `"}`,
		"responses/channels.json":            `{"BotToken":"` + botToken + `"}`,
		"events/stream.txt":                  "event: change\ndata: " + signingText + "\n",
		"pg_dump/NotificationDeliveries.sql": slackURL,
		"inbox/inbox.json":                   `{"Text":"` + botToken + `"}`,
	}
	clean := map[string]string{
		"logs/clean.log":       `{"msg":"delivered","channel_id":"0198f5d2-0000-7000-8000-000000000001","kind":"slack","host":"hooks.slack.com"}`,
		"responses/rules.json": `{"Events":["run.finished"],"HasSecret":true}`,
		"events/clean.txt":     "event: change\ndata: {\"Seq\":7,\"Type\":\"release.succeeded\"}\n",
		"inbox/clean.json":     `{"Text":"Deployment failed","Url":"/projects/1/runs/2"}`,
	}
	dir := t.TempDir()
	for name, text := range places {
		plant(t, dir, name, text)
	}
	for name, text := range clean {
		plant(t, dir, name, text)
	}
	hits, _, err := leaksweep.Sweep([]string{dir}, needles, leaksweep.NotifyPatterns(), leaksweep.DefaultLimits())
	require.NoError(t, err)
	found := map[string]bool{}
	for _, h := range hits {
		rel, err := filepath.Rel(dir, h.File)
		require.NoError(t, err)
		found[rel] = true
	}
	want := map[string]bool{}
	for name := range places {
		want[name] = true
	}
	assert.Equal(t, want, found, "exactly the planted files, and no clean file")
}

func plant(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}
