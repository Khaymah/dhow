package unit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
	"github.com/Khaymah/dhow/internal/leaksweep"
)

func sweepPattern(t *testing.T, name string) leaksweep.Pattern {
	t.Helper()
	for _, p := range leaksweep.NotifyPatterns() {
		if p.Name == name {
			return p
		}
	}
	require.FailNow(t, "the sweep has no pattern named "+name)
	return leaksweep.Pattern{}
}

func TestTheSweepRecognisesEverySecretShapeTheServerMakes(t *testing.T) {
	for range 50 {
		secret, err := notifications.NewSecret()
		require.NoError(t, err)
		assert.True(t, sweepPattern(t, leaksweep.NameWebhookSecret).Re.MatchString(secret), "a signing secret the server made must match the sweep")
		assert.True(t, notifications.ValidSecret(secret))
	}
}

func TestTheSweepRecognisesAnAddressAndATokenTheServerAccepts(t *testing.T) {
	slack := "https://hooks.slack.com/services/" + "T0DEMO0001/B0DEMO0001/" + "Xk3mP9qR2sT5vW8yZ1aB4cD7"
	target, err := notifications.ParseTarget(slack, false)
	require.NoError(t, err)
	_, err = notifications.ChatURL(models.KindSlack, target)
	require.NoError(t, err, "the server accepts this address")
	assert.True(t, sweepPattern(t, leaksweep.NameSlackWebhook).Re.MatchString(slack))

	discord := "https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz0123456789ABCD_-"
	target, err = notifications.ParseTarget(discord, false)
	require.NoError(t, err)
	_, err = notifications.ChatURL(models.KindDiscord, target)
	require.NoError(t, err, "the server accepts this address")
	assert.True(t, sweepPattern(t, leaksweep.NameDiscordWebhook).Re.MatchString(discord))

	token := "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw0"
	require.True(t, notifications.ValidBotToken(token), "the server accepts this token")
	assert.True(t, sweepPattern(t, leaksweep.NameTelegramToken).Re.MatchString("/bot"+token+"/sendMessage"))
}
