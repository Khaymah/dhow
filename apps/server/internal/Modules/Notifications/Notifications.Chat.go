package notifications

import (
	"bytes"
	"context"
	"net/http"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
)

type chatDeliverer struct {
	s    *Service
	kind string
}

func (d chatDeliverer) Deliver(ctx context.Context, item outbox.Item) outbox.Result {
	s := d.s
	ch, res, ok := s.liveChannel(ctx, item)
	if !ok {
		return res
	}
	if ch.Kind != d.kind {
		return outbox.Result{Code: CodeConfig, Permanent: true}
	}
	cfg, err := s.openConfig(ctx, ch)
	if err != nil {
		s.log.WarnContext(ctx, "notification channel settings cannot be read", "channel_id", ch.Id.String())
		return outbox.Result{Code: CodeConfig, Permanent: true}
	}
	p, err := DecodePayload(item.Payload)
	if err != nil {
		return outbox.Result{Code: CodeConfig, Permanent: true}
	}
	req, err := chatRequest(ctx, d.kind, cfg, i18n.Locale(ch.Locale), p)
	if err != nil {
		return outbox.Result{Code: CodeBlocked, Permanent: true}
	}
	return s.send(ctx, ch.Id.String(), ch.Kind, item, req, cfg.sensitive())
}

func chatRequest(ctx context.Context, kind string, cfg channelConfig, locale i18n.Locale, p Payload) (*http.Request, error) {
	var address string
	var body []byte
	switch kind {
	case models.KindSlack, models.KindDiscord:
		target, err := ParseTarget(cfg.URL, false)
		if err != nil {
			return nil, err
		}
		u, err := ChatURL(kind, target)
		if err != nil {
			return nil, err
		}
		address = u.String()
		if kind == models.KindSlack {
			body, err = RenderSlack(locale, p)
		} else {
			body, err = RenderDiscord(locale, p)
		}
		if err != nil {
			return nil, err
		}
	case models.KindTelegram:
		if !ValidBotToken(cfg.BotToken) || !ValidChatID(cfg.ChatID) {
			return nil, ErrHostNotAllowed
		}
		var err error
		if body, err = RenderTelegram(locale, cfg.ChatID, p); err != nil {
			return nil, err
		}
		address = TelegramURL(cfg.BotToken)
	default:
		return nil, ErrHostNotAllowed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, ErrHostNotAllowed
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}
