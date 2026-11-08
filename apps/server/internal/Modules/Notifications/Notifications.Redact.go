package notifications

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

const (
	redacted     = "[redacted]"
	minSecretLen = 6
)

var (
	addressPattern  = regexp.MustCompile(`https?://[^\s"']+`)
	botTokenPattern = regexp.MustCompile(`[0-9]{3,20}(?::|%3[Aa])[A-Za-z0-9_-]{20,}`)
)

func RedactError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	for {
		var ue *url.Error
		if !errors.As(err, &ue) || ue.Err == nil {
			break
		}
		err = ue.Err
	}
	msg := err.Error()
	for _, s := range secrets {
		if len(s) >= minSecretLen {
			msg = strings.ReplaceAll(msg, s, redacted)
		}
	}
	msg = addressPattern.ReplaceAllString(msg, redacted)
	return botTokenPattern.ReplaceAllString(msg, redacted)
}

func (c channelConfig) sensitive() []string {
	out := []string{c.URL, c.Secret, c.BotToken, c.ChatID}
	if u, err := url.Parse(c.URL); err == nil {
		out = append(out, u.Path, strings.TrimPrefix(u.Path, "/"))
	}
	if _, secret, ok := strings.Cut(c.BotToken, ":"); ok {
		out = append(out, secret)
	}
	return out
}
