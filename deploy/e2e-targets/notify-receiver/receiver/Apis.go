package receiver

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf16"
)

const (
	slackTextMax    = 40000
	discordTextMax  = 2000
	telegramTextMax = 4096
	jsonType        = "application/json"
)

var (
	slackPath    = regexp.MustCompile(`^/services/[A-Za-z0-9]+/[A-Za-z0-9]+/[A-Za-z0-9]+$`)
	discordPath  = regexp.MustCompile(`^/api/webhooks/[0-9]+/[A-Za-z0-9_-]+$`)
	telegramPath = regexp.MustCompile(`^/bot[0-9]+:[A-Za-z0-9_-]{20,}/([A-Za-z]+)$`)
	telegramTag  = regexp.MustCompile(`^</?(b|strong|i|em|u|ins|s|strike|del|code|pre|tg-spoiler)>`)
	telegramLink = regexp.MustCompile(`^<a href="[^"<>]*">`)
	htmlEntity   = regexp.MustCompile(`^&(amp|lt|gt|quot|#[0-9]{1,7}|#x[0-9A-Fa-f]{1,6});`)
	mentionKinds = map[string]bool{"roles": true, "users": true, "everyone": true}
)

func utf16Len(v string) int { return len(utf16.Encode([]rune(v))) }

type slackAPI struct{}

func fields(body []byte) (map[string]json.RawMessage, bool) {
	var out map[string]json.RawMessage
	if err := json.Unmarshal(body, &out); err != nil || out == nil {
		return nil, false
	}
	return out, true
}

func textOf(f map[string]json.RawMessage, key string) string {
	var v string
	if raw, ok := f[key]; ok {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

func (slackAPI) Accept(req *http.Request, body []byte) (Message, reply) {
	if !slackPath.MatchString(req.URL.Path) {
		return Message{}, reply{Status: http.StatusNotFound, Body: "no_service", Type: "text/plain"}
	}
	f, ok := fields(body)
	if !ok {
		return Message{}, reply{Status: http.StatusBadRequest, Body: "invalid_payload", Type: "text/plain"}
	}
	text := textOf(f, "text")
	if _, blocks := f["blocks"]; text == "" && !blocks {
		return Message{}, reply{Status: http.StatusBadRequest, Body: "no_text", Type: "text/plain"}
	}
	if utf16Len(text) > slackTextMax {
		return Message{}, reply{Status: http.StatusBadRequest, Body: "msg_too_long", Type: "text/plain"}
	}
	return Message{Text: text}, reply{Status: http.StatusOK, Body: "ok", Type: "text/plain"}
}

type discordAPI struct{}

func discordError(status int, code int, message string) reply {
	raw, _ := json.Marshal(map[string]any{"code": code, "message": message})
	return reply{Status: status, Body: string(raw), Type: jsonType}
}

func (discordAPI) Accept(req *http.Request, body []byte) (Message, reply) {
	if !discordPath.MatchString(req.URL.Path) {
		return Message{}, discordError(http.StatusNotFound, 10015, "Unknown Webhook")
	}
	f, ok := fields(body)
	if !ok {
		return Message{}, discordError(http.StatusBadRequest, 50109, "The request body contains invalid JSON.")
	}
	content := textOf(f, "content")
	if content == "" {
		return Message{}, discordError(http.StatusBadRequest, 50006, "Cannot send an empty message")
	}
	if utf16Len(content) > discordTextMax {
		return Message{}, discordError(http.StatusBadRequest, 50035, "Invalid Form Body: content: Must be 2000 or fewer in length.")
	}
	msg := Message{Text: content}
	if raw, ok := f["allowed_mentions"]; ok {
		var policy map[string][]string
		if err := json.Unmarshal(raw, &policy); err != nil {
			return Message{}, discordError(http.StatusBadRequest, 50035, "Invalid Form Body: allowed_mentions: invalid")
		}
		msg.HasMentions, msg.MentionsParse = true, append([]string{}, policy["parse"]...)
		for _, kind := range msg.MentionsParse {
			if !mentionKinds[kind] {
				return Message{}, discordError(http.StatusBadRequest, 50035, "Invalid Form Body: allowed_mentions.parse: unknown value")
			}
		}
	}
	if req.URL.Query().Get("wait") == "true" {
		return msg, reply{Status: http.StatusOK, Body: `{"id":"1","type":0}`, Type: jsonType}
	}
	return msg, reply{Status: http.StatusNoContent}
}

type telegramAPI struct{}

func telegramError(status int, description string) reply {
	raw, _ := json.Marshal(map[string]any{"ok": false, "error_code": status, "description": description})
	return reply{Status: status, Body: string(raw), Type: jsonType}
}

func (telegramAPI) Accept(req *http.Request, body []byte) (Message, reply) {
	m := telegramPath.FindStringSubmatch(req.URL.Path)
	if m == nil {
		return Message{}, telegramError(http.StatusNotFound, "Not Found")
	}
	if m[1] != "sendMessage" {
		return Message{}, telegramError(http.StatusNotFound, "Not Found")
	}
	f, ok := fields(body)
	if !ok {
		return Message{}, telegramError(http.StatusBadRequest, "Bad Request: can't parse JSON")
	}
	chat := strings.Trim(string(f["chat_id"]), `"`)
	if chat == "" || chat == "null" {
		return Message{}, telegramError(http.StatusBadRequest, "Bad Request: chat_id is empty")
	}
	text, mode := textOf(f, "text"), textOf(f, "parse_mode")
	if text == "" {
		return Message{}, telegramError(http.StatusBadRequest, "Bad Request: message text is empty")
	}
	if utf16Len(text) > telegramTextMax {
		return Message{}, telegramError(http.StatusBadRequest, "Bad Request: message is too long")
	}
	switch mode {
	case "":
	case "HTML":
		if !validTelegramHTML(text) {
			return Message{}, telegramError(http.StatusBadRequest, "Bad Request: can't parse entities: unsupported start tag or unescaped character")
		}
	default:
		return Message{}, telegramError(http.StatusBadRequest, "Bad Request: unsupported parse_mode")
	}
	return Message{Text: text, ChatID: chat, ParseMode: mode}, reply{
		Status: http.StatusOK, Type: jsonType,
		Body: `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":0},"text":"ok"}}`,
	}
}

func validTelegramHTML(text string) bool {
	for i := 0; i < len(text); i++ {
		rest := text[i:]
		switch text[i] {
		case '<':
			if loc := telegramTag.FindString(rest); loc != "" {
				i += len(loc) - 1
			} else if loc := telegramLink.FindString(rest); loc != "" {
				i += len(loc) - 1
			} else if strings.HasPrefix(rest, "</a>") {
				i += len("</a>") - 1
			} else {
				return false
			}
		case '>':
			return false
		case '&':
			loc := htmlEntity.FindString(rest)
			if loc == "" {
				return false
			}
			i += len(loc) - 1
		}
	}
	return true
}
