package notifications

import (
	"context"
	"errors"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Models"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	fieldBotToken = "BotToken"
	fieldChatID   = "ChatId"
	fieldRotate   = "RotateSecret"
)

type channelConfig struct {
	URL      string `json:"Url,omitempty"`
	Secret   string `json:"Secret,omitempty"`
	BotToken string `json:"BotToken,omitempty"`
	ChatID   string `json:"ChatId,omitempty"`
}

type configChange struct {
	cfg     channelConfig
	host    string
	secret  string
	changed bool
}

func usesURL(kind string) bool {
	return kind == models.KindWebhook || kind == models.KindSlack || kind == models.KindDiscord
}

func validConfig(kind string, cfg channelConfig) bool {
	switch kind {
	case models.KindWebhook:
		return cfg.URL != "" && ValidSecret(cfg.Secret)
	case models.KindSlack, models.KindDiscord:
		return cfg.URL != ""
	case models.KindTelegram:
		return cfg.BotToken != "" && cfg.ChatID != ""
	default:
		return false
	}
}

func validInputs(kind, address, token, chat string) []apperrors.IErrorDetail {
	var details []apperrors.IErrorDetail
	if usesURL(kind) == (address == "") {
		details = append(details, invalid(fieldURL))
	}
	telegram := kind == models.KindTelegram
	if telegram == (token == "") || (telegram && !ValidBotToken(token)) {
		details = append(details, invalid(fieldBotToken))
	}
	if telegram == (chat == "") || (telegram && !ValidChatID(chat)) {
		details = append(details, invalid(fieldChatID))
	}
	return details
}

func (s *Service) resolveHost(ctx context.Context, hostname string) error {
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	if _, err := s.dialer().Resolve(ctx, hostname); err != nil {
		if errors.Is(err, netguard.ErrBlocked) {
			return apperrors.ChannelHostBlocked()
		}
		return apperrors.ValidationFailed(invalid(fieldURL))
	}
	return nil
}

func (s *Service) checkTarget(ctx context.Context, kind, raw string) (Target, error) {
	target, err := ParseTarget(raw, false)
	switch {
	case errors.Is(err, ErrURLScheme), errors.Is(err, ErrURLUserinfo), errors.Is(err, ErrURLHost), errors.Is(err, ErrURLNumeric):
		return Target{}, apperrors.ChannelHostNotAllowed()
	case err != nil:
		return Target{}, apperrors.ValidationFailed(invalid(fieldURL))
	}
	if models.Chat(kind) {
		u, err := ChatURL(kind, target)
		if err != nil {
			return Target{}, apperrors.ChannelHostNotAllowed()
		}
		target.URL, target.Display = u, target.Hostname
	}
	if err := s.resolveHost(ctx, target.Hostname); err != nil {
		return Target{}, err
	}
	return target, nil
}

func (s *Service) newConfig(ctx context.Context, in models.CreateChannelInput) (configChange, error) {
	switch {
	case in.Kind == models.KindTelegram:
		if err := s.resolveHost(ctx, HostTelegram); err != nil {
			return configChange{}, err
		}
		return configChange{cfg: channelConfig{BotToken: in.BotToken, ChatID: in.ChatID}, host: HostTelegram, changed: true}, nil
	case usesURL(in.Kind):
		target, err := s.checkTarget(ctx, in.Kind, in.URL)
		if err != nil {
			return configChange{}, err
		}
		out := configChange{cfg: channelConfig{URL: target.URL.String()}, host: target.Display, changed: true}
		if in.Kind == models.KindWebhook {
			if out.secret, err = NewSecret(); err != nil {
				return configChange{}, err
			}
			out.cfg.Secret = out.secret
		}
		return out, nil
	default:
		return configChange{}, nil
	}
}

func (s *Service) planConfig(ctx context.Context, row sqlc.NotificationChannel, in models.UpdateChannelInput) (configChange, error) {
	touches := in.URL != nil || in.BotToken != nil || in.ChatID != nil || in.RotateSecret
	if !touches {
		return configChange{}, nil
	}
	if details := misplacedFields(row.Kind, in); len(details) > 0 {
		return configChange{}, apperrors.ValidationFailed(details...)
	}
	switch row.Kind {
	case models.KindWebhook:
		return s.planWebhook(ctx, row, in)
	case models.KindTelegram:
		return s.planTelegram(ctx, row, in)
	default:
		target, err := s.checkTarget(ctx, row.Kind, *in.URL)
		if err != nil {
			return configChange{}, err
		}
		return configChange{cfg: channelConfig{URL: target.URL.String()}, host: target.Display, changed: true}, nil
	}
}

func misplacedFields(kind string, in models.UpdateChannelInput) []apperrors.IErrorDetail {
	var details []apperrors.IErrorDetail
	if in.URL != nil && !usesURL(kind) {
		details = append(details, invalid(fieldURL))
	}
	if (in.BotToken != nil || in.ChatID != nil) && kind != models.KindTelegram {
		details = append(details, invalid(fieldBotToken))
	}
	if in.RotateSecret && kind != models.KindWebhook {
		details = append(details, invalid(fieldRotate))
	}
	return details
}

func (s *Service) planWebhook(ctx context.Context, row sqlc.NotificationChannel, in models.UpdateChannelInput) (configChange, error) {
	cfg, err := s.openConfig(ctx, row)
	if err != nil && in.URL == nil {
		return configChange{}, err
	}
	out := configChange{cfg: cfg, host: row.Host, changed: true}
	if in.URL != nil {
		target, err := s.checkTarget(ctx, row.Kind, *in.URL)
		if err != nil {
			return configChange{}, err
		}
		out.cfg.URL, out.host = target.URL.String(), target.Display
	}
	if in.RotateSecret || !ValidSecret(out.cfg.Secret) {
		if out.cfg.Secret, err = NewSecret(); err != nil {
			return configChange{}, err
		}
		out.secret = out.cfg.Secret
	}
	return out, nil
}

func (s *Service) planTelegram(ctx context.Context, row sqlc.NotificationChannel, in models.UpdateChannelInput) (configChange, error) {
	var cfg channelConfig
	if in.BotToken == nil || in.ChatID == nil {
		current, err := s.openConfig(ctx, row)
		if err != nil {
			return configChange{}, err
		}
		cfg = current
	}
	var details []apperrors.IErrorDetail
	if in.BotToken != nil {
		cfg.BotToken = *in.BotToken
	}
	if in.ChatID != nil {
		cfg.ChatID = *in.ChatID
	}
	if !ValidBotToken(cfg.BotToken) {
		details = append(details, invalid(fieldBotToken))
	}
	if !ValidChatID(cfg.ChatID) {
		details = append(details, invalid(fieldChatID))
	}
	if len(details) > 0 {
		return configChange{}, apperrors.ValidationFailed(details...)
	}
	return configChange{cfg: cfg, host: HostTelegram, changed: true}, nil
}
