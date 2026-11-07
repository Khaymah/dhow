package alerts

import (
	alert "github.com/Khaymah/dhow/apps/server/internal/Core/Alert"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
)

const (
	ownerKey       = "Owner"
	changeMfaReset = alert.ChangeMfaReset
)

func changeOf(event string) (string, bool) {
	switch event {
	case eventbus.AuthPasswordChanged:
		return "password_changed", true
	case eventbus.AuthTotpEnabled:
		return "totp_enabled", true
	case eventbus.AuthTotpDisabled:
		return "totp_disabled", true
	case eventbus.AuthPasskeyAdded:
		return "passkey_added", true
	case eventbus.AuthPasskeyRemoved:
		return "passkey_removed", true
	case eventbus.AuthRecoveryCodesRegenerated:
		return "recovery_codes_regenerated", true
	case eventbus.AuthIdentityLinked:
		return "identity_linked", true
	case eventbus.AuthIdentityUnlinked:
		return "identity_unlinked", true
	case eventbus.AuthApiTokenCreated:
		return "api_token_created", true
	case eventbus.IdentityMemberMfaReset:
		return changeMfaReset, true
	default:
		return "", false
	}
}

func Events() []string {
	return []string{
		eventbus.AuthPasswordChanged,
		eventbus.AuthTotpEnabled,
		eventbus.AuthTotpDisabled,
		eventbus.AuthPasskeyAdded,
		eventbus.AuthPasskeyRemoved,
		eventbus.AuthRecoveryCodesRegenerated,
		eventbus.AuthIdentityLinked,
		eventbus.AuthIdentityUnlinked,
		eventbus.AuthApiTokenCreated,
		eventbus.IdentityMemberMfaReset,
	}
}

func Changes() []string {
	out := make([]string, 0, len(Events()))
	for _, name := range Events() {
		if c, ok := changeOf(name); ok {
			out = append(out, c)
		}
	}
	return out
}

type Details = alert.Details

func Message(locale i18n.Locale, d Details) mailer.Message { return alert.Message(locale, d) }

func ApproximateIP(raw string) string { return alert.ApproximateIP(raw) }

func Browser(ua string) string { return alert.Browser(ua) }
