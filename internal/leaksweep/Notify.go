package leaksweep

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	NameSlackWebhook    = "Slack incoming webhook URL"
	NameDiscordWebhook  = "Discord webhook URL"
	NameTelegramToken   = "Telegram bot token"
	NameWebhookSecret   = "webhook signing secret"
	webhookSecretPrefix = "dhow_whsec_"
	webhookSecretBody   = 43
	slash               = `\\?/`
	tokenSet            = `[A-Za-z0-9_-]`
)

var (
	slackWebhook = regexp.MustCompile(`hooks\.slack\.com` + slash + `services` + slash + `T[A-Z0-9]{8,}` + slash + `B[A-Z0-9]{8,}` + slash + `[A-Za-z0-9]{20,}`)
	discordHook  = regexp.MustCompile(`discord(?:app)?\.com` + slash + `api` + slash + `webhooks` + slash + `[0-9]{15,}` + slash + tokenSet + `{40,}`)
	telegramBot  = regexp.MustCompile(`(?:^|[^0-9])[0-9]{8,10}:` + tokenSet + `{35}(?:$|[^A-Za-z0-9_-])`)
	signingToken = regexp.MustCompile(webhookSecretPrefix + tokenSet + `{` + fmt.Sprint(webhookSecretBody) + `}`)
)

type ChannelSecrets struct {
	URL      string `json:"Url"`
	Secret   string `json:"Secret"`
	BotToken string `json:"BotToken"`
}

func NotifyPatterns() []Pattern {
	return []Pattern{
		{Name: NameSlackWebhook, Re: slackWebhook},
		{Name: NameDiscordWebhook, Re: discordHook},
		{Name: NameTelegramToken, Re: telegramBot},
		{Name: NameWebhookSecret, Re: signingToken},
	}
}

func ChannelNeedles(name string, plain []byte) ([]Needle, error) {
	var cfg ChannelSecrets
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, fmt.Errorf("%s is not the settings of a notification channel: %w", name, err)
	}
	var out []Needle
	add := func(label, value string) error {
		if len(value) < MinValueBytes {
			return nil
		}
		n, err := Encoded(name+" "+label, []byte(value))
		if err != nil {
			return err
		}
		out = append(out, n)
		return nil
	}
	if err := add("secret", cfg.Secret); err != nil {
		return nil, err
	}
	if err := add("bot token", cfg.BotToken); err != nil {
		return nil, err
	}
	if err := add("address", cfg.URL); err != nil {
		return nil, err
	}
	if u, err := url.Parse(cfg.URL); err == nil && strings.Count(u.Path, "/") >= 2 {
		if err := add("address path", u.Path); err != nil {
			return nil, err
		}
	}
	return out, nil
}
