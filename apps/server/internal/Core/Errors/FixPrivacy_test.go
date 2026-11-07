package apperrors_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
)

var accountStateCodes = []string{
	"INVALID_CREDENTIALS",
	"INVALID_MFA_CODE",
	"INVALID_CURRENT_PASSWORD",
	"INVALID_CURRENT_FACTOR",
	"MFA_CHALLENGE_EXPIRED",
	"INVITATION_INVALID",
	"INVITATION_EMAIL_MISMATCH",
	"OIDC_NOT_LINKED",
}

var accountStateTerms = map[i18n.Locale][]string{
	i18n.En: {"exist", "registered", "no account", "locked", "disabled", "not found", "unknown", "wrong email", "wrong password", "enabled"},
	i18n.Ar: {"غير موجود", "غير مسجل", "مقفل", "معطل", "غير معروف", "مفعّل"},
}

var approvedSignInFixes = map[i18n.Locale]string{
	i18n.En: "Check the email and the password. If you forgot the password, ask your server admin for help.",
	i18n.Ar: "راجع البريد وكلمة السر. إذا نسيت كلمة السر، اطلب المساعدة من مسؤول السيرفر.",
}

func TestSignInFixesDoNotRevealAccountState(t *testing.T) {
	for _, locale := range []i18n.Locale{i18n.Ar, i18n.En} {
		assert.Equal(t, approvedSignInFixes[locale], i18n.Fix(locale, "INVALID_CREDENTIALS"), "%s sign-in fix must stay generic", locale)
		for _, code := range accountStateCodes {
			fix := strings.ToLower(i18n.Fix(locale, code))
			assert.NotEmpty(t, fix, "%s %s", locale, code)
			for _, term := range accountStateTerms[locale] {
				assert.NotContains(t, fix, term, "%s %s must not reveal account state", locale, code)
			}
		}
	}
}
