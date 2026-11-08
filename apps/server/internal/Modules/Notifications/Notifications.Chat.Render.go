package notifications

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
)

const (
	discordContentMax  = 1900
	telegramContentMax = 3900
	slackContentMax    = 3000
	senderName         = "Dhow"
	discordSuppress    = 4
	parseHTML          = "HTML"
	bidiOverrideFrom   = 0x202A
	bidiOverrideTo     = 0x202E
	bidiIsolateFrom    = 0x2066
	bidiIsolateTo      = 0x2069
	arabicLetterMark   = 0x061C
)

var (
	slackEscaper    = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	telegramEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	discordEscaper  = strings.NewReplacer(
		"\\", "\\\\", "*", "\\*", "_", "\\_", "~", "\\~", "|", "\\|", "`", "\\`", "[", "\\[", "]", "\\]", "<", "<\u200b",
	)
)

func EscapeSlack(v string) string { return slackEscaper.Replace(v) }

func EscapeTelegram(v string) string { return telegramEscaper.Replace(v) }

func EscapeDiscord(v string) string { return discordEscaper.Replace(v) }

func plain(v string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r < ' ' || r == 0x7f:
			return ' '
		case r >= bidiOverrideFrom && r <= bidiOverrideTo, r >= bidiIsolateFrom && r <= bidiIsolateTo, r == arabicLetterMark:
			return -1
		default:
			return r
		}
	}, v)
}

func fit(v string, limit int) string {
	if utf8.RuneCountInString(v) <= limit {
		return v
	}
	return string([]rune(v)[:limit])
}

func compose(locale i18n.Locale, p Payload, bold func(string) string, escape func(string) string) string {
	title, lines := Parts(locale, p)
	var b strings.Builder
	b.WriteString(bold(escape(plain(title))) + "\n")
	for _, l := range lines {
		b.WriteString(escape(plain(l.Label)) + ": " + escape(plain(l.Value)) + "\n")
	}
	if p.URL != "" {
		b.WriteString("\n" + escape(plain(p.URL)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func RenderSlack(locale i18n.Locale, p Payload) ([]byte, error) {
	text := compose(locale, p, func(v string) string { return "*" + v + "*" }, EscapeSlack)
	return encodeMessage(map[string]any{"text": fit(text, slackContentMax), "unfurl_links": false, "unfurl_media": false})
}

func RenderDiscord(locale i18n.Locale, p Payload) ([]byte, error) {
	text := compose(locale, p, func(v string) string { return "**" + v + "**" }, EscapeDiscord)
	return encodeMessage(map[string]any{
		"username": senderName, "content": fit(text, discordContentMax), "allowed_mentions": map[string][]string{"parse": {}}, "flags": discordSuppress,
	})
}

func RenderTelegram(locale i18n.Locale, chatID string, p Payload) ([]byte, error) {
	text := compose(locale, p, func(v string) string { return "<b>" + v + "</b>" }, EscapeTelegram)
	return encodeMessage(map[string]any{
		"chat_id": chatID, "text": fitHTML(text, telegramContentMax), "parse_mode": parseHTML,
		"link_preview_options": map[string]bool{"is_disabled": true},
	})
}

func fitHTML(v string, limit int) string {
	if utf8.RuneCountInString(v) <= limit {
		return v
	}
	cut := string([]rune(v)[:limit])
	if i := strings.LastIndex(cut, "&"); i >= 0 && !strings.Contains(cut[i:], ";") {
		cut = cut[:i]
	}
	return cut
}

func encodeMessage(fields map[string]any) ([]byte, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode a chat message: %w", err)
	}
	return raw, nil
}
