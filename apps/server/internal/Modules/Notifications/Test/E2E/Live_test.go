//go:build live

package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

type liveTarget struct {
	kind   string
	body   func(live config.LiveNotify) map[string]any
	status int
}

func liveTargets() []liveTarget {
	return []liveTarget{
		{"slack", func(l config.LiveNotify) map[string]any { return map[string]any{"Url": l.Slack} }, http.StatusOK},
		{"discord", func(l config.LiveNotify) map[string]any { return map[string]any{"Url": l.Discord} }, http.StatusNoContent},
		{"telegram", func(l config.LiveNotify) map[string]any {
			return map[string]any{"BotToken": l.TelegramToken, "ChatId": l.TelegramChat}
		}, http.StatusOK},
	}
}

func hasLive(kind string, l config.LiveNotify) bool {
	switch kind {
	case "slack":
		return l.Slack != ""
	case "discord":
		return l.Discord != ""
	default:
		return l.TelegramToken != "" && l.TelegramChat != ""
	}
}

func TestLiveEachServiceTakesAnArabicAndAnEnglishMessage(t *testing.T) {
	live := config.LiveNotifyFromEnv()
	if !hasLive("slack", live) && !hasLive("discord", live) && !hasLive("telegram", live) {
		t.Skip("set SLACK_WEBHOOK_URL, DISCORD_WEBHOOK_URL, TELEGRAM_BOT_TOKEN and TELEGRAM_MY_ID to send to the real services")
	}
	w := &world{t: t, mail: &apptest.FakeMailer{}}
	w.boot(apptest.Options{Mailer: w.mail})

	want := map[string]int{}
	for _, target := range liveTargets() {
		if !hasLive(target.kind, live) {
			t.Logf("%s: skipped, its variables are not set", target.kind)
			continue
		}
		for _, locale := range []string{"ar", "en"} {
			body := target.body(live)
			body["Kind"], body["Name"], body["Locale"], body["CurrentFactor"] = target.kind, "live-"+target.kind+"-"+locale, locale, w.factor()
			res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/notification-channels"), Body: body})
			code := ""
			if res.Error != nil {
				code = res.Error.Code
			}
			require.Equal(t, http.StatusCreated, res.Status, "%s %s: create answered with code %q", target.kind, locale, code)
			want[target.kind+" "+locale] = target.status
		}
	}

	_, err := w.env.Pool.Exec(t.Context(), `UPDATE "PipelineRuns" SET "Ref" = $1 WHERE "Id" = $2`, "refs/heads/<!channel> @everyone @here", w.run)
	require.NoError(t, err)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()

	rows, err := w.env.Pool.Query(t.Context(), `SELECT c."Kind", c."Locale", o."State", o."LastError", COALESCE(o."LastStatus", 0)
		FROM "NotificationOutbox" o JOIN "NotificationChannels" c ON c."Id" = o."ChannelId" ORDER BY c."Kind", c."Locale"`)
	require.NoError(t, err)
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var kind, locale, state, lastError string
		var status int
		require.NoError(t, rows.Scan(&kind, &locale, &state, &lastError, &status))
		t.Logf("%s %s: state=%s http=%d error=%q", kind, locale, state, status, lastError)
		assert.Equal(t, "sent", state, "%s %s", kind, locale)
		assert.Equal(t, want[kind+" "+locale], status, "%s %s", kind, locale)
		seen++
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, len(want), seen, "every channel got its message")
}
