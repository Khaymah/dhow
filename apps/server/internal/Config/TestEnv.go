package config

import "os"

const (
	EnvTestDatabaseURL = "DHOW_TEST_DATABASE_URL"
	EnvTestPgDump      = "DHOW_TEST_PG_DUMP"
	envCI              = "CI"
	defaultPgDump      = "pg_dump"
)

func TestDatabaseURL() string { return os.Getenv(EnvTestDatabaseURL) }

func TestPgDump() string {
	if v := os.Getenv(EnvTestPgDump); v != "" {
		return v
	}
	return defaultPgDump
}

func RunningInCI() bool { return os.Getenv(envCI) != "" }

const (
	EnvLiveSlackWebhook   = "SLACK_WEBHOOK_URL"
	EnvLiveDiscordWebhook = "DISCORD_WEBHOOK_URL"
	EnvLiveTelegramBot    = "TELEGRAM_BOT_TOKEN"
	EnvLiveTelegramChat   = "TELEGRAM_MY_ID"
)

type LiveNotify struct {
	Slack         string
	Discord       string
	TelegramToken string
	TelegramChat  string
}

func LiveNotifyFromEnv() LiveNotify {
	return LiveNotify{
		Slack: os.Getenv(EnvLiveSlackWebhook), Discord: os.Getenv(EnvLiveDiscordWebhook),
		TelegramToken: os.Getenv(EnvLiveTelegramBot), TelegramChat: os.Getenv(EnvLiveTelegramChat),
	}
}
