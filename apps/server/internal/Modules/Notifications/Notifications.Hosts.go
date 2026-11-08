package notifications

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

const (
	HostSlack         = "hooks.slack.com"
	HostDiscord       = "discord.com"
	HostDiscordLegacy = "discordapp.com"
	HostTelegram      = "api.telegram.org"

	slackPathPrefix   = "/services/"
	slackPathParts    = 3
	discordPathPrefix = "/api/webhooks/"
	discordPathParts  = 2
	httpsPort         = "443"
	telegramPathHead  = "/bot"
	telegramPathTail  = "/sendMessage"
)

var (
	ErrHostNotAllowed = errors.New("the address is not an address of this service")

	pathPart     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	botTokenForm = regexp.MustCompile(`^[0-9]{3,20}:[A-Za-z0-9_-]{20,64}$`)
	chatIDForm   = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z][A-Za-z0-9_]{4,31})$`)
)

func ChatHosts(kind string) []string {
	switch kind {
	case models.KindSlack:
		return []string{HostSlack}
	case models.KindDiscord:
		return []string{HostDiscord, HostDiscordLegacy}
	case models.KindTelegram:
		return []string{HostTelegram}
	default:
		return nil
	}
}

func ValidBotToken(v string) bool { return botTokenForm.MatchString(v) }

func ValidChatID(v string) bool { return chatIDForm.MatchString(v) }

func ChatURL(kind string, t Target) (*url.URL, error) {
	hosts := ChatHosts(kind)
	if kind == models.KindTelegram || len(hosts) == 0 || t.URL == nil {
		return nil, ErrHostNotAllowed
	}
	u := t.URL
	if !slices.Contains(hosts, t.Hostname) || (u.Port() != "" && u.Port() != httpsPort) {
		return nil, ErrHostNotAllowed
	}
	if u.RawQuery != "" || u.ForceQuery || u.RawPath != "" || path.Clean(u.Path) != u.Path {
		return nil, ErrHostNotAllowed
	}
	prefix, parts := slackPathPrefix, slackPathParts
	if kind == models.KindDiscord {
		prefix, parts = discordPathPrefix, discordPathParts
	}
	rest, ok := strings.CutPrefix(u.Path, prefix)
	if !ok {
		return nil, ErrHostNotAllowed
	}
	segments := strings.Split(rest, "/")
	if len(segments) != parts {
		return nil, ErrHostNotAllowed
	}
	for _, s := range segments {
		if !pathPart.MatchString(s) {
			return nil, ErrHostNotAllowed
		}
	}
	return &url.URL{Scheme: "https", Host: t.Hostname, Path: u.Path}, nil
}

func TelegramURL(token string) string {
	return "https://" + HostTelegram + telegramPathHead + token + telegramPathTail
}
