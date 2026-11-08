package preferences

import (
	"regexp"
	"slices"
	"strconv"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Preferences/Models"
)

const (
	fieldLocale       = "Locale"
	fieldThemeID      = "ThemeId"
	fieldCustomThemes = "CustomThemes"
	fieldOrganization = "OrganizationId"
	fieldVersion      = "Version"

	messageTheme        = "errors.preferenceTheme"
	messageThemes       = "errors.preferenceThemes"
	messageOrganization = "errors.preferenceOrganization"
	messageLocale       = "errors.validationFailed"
	messageVersion      = "errors.releaseVersion"
)

var (
	themeIDPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	colorPattern   = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	versionPattern = regexp.MustCompile(`^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}(-rc\.[0-9]{1,6})?$`)
	locales        = []string{"ar", "en"}
)

func SystemThemes() []string { return []string{"sea", "teak", "dusk", "indigo"} }

func ThemeTokens() []string {
	return []string{
		"canvas", "rail", "input", "inset", "surface", "raised", "top", "track",
		"t1", "t2", "t3", "t4", "a1", "a2", "a3", "a4", "a5", "a6",
		"edge-warm", "edge-cool", "green", "blue", "amber", "coral", "violet-soft",
	}
}

func validTheme(t models.CustomTheme) bool {
	if !themeIDPattern.MatchString(t.Id) || slices.Contains(SystemThemes(), t.Id) || !auth.ValidDisplayName(t.Name, config.ThemeNameMax) {
		return false
	}
	tokens := ThemeTokens()
	if len(t.Colors) != len(tokens) {
		return false
	}
	for _, token := range tokens {
		if !colorPattern.MatchString(t.Colors[token]) {
			return false
		}
	}
	return true
}

func validate(p models.Preferences) *apperrors.AppError {
	var details []apperrors.IErrorDetail
	if !slices.Contains(locales, p.Locale) {
		details = append(details, apperrors.IErrorDetail{Field: fieldLocale, Message: messageLocale})
	}
	ids := make([]string, 0, len(p.CustomThemes))
	if len(p.CustomThemes) > config.MaxCustomThemes {
		details = append(details, apperrors.IErrorDetail{Field: fieldCustomThemes, Message: messageThemes})
	} else {
		for i, t := range p.CustomThemes {
			if !validTheme(t) || slices.Contains(ids, t.Id) {
				details = append(details, apperrors.IErrorDetail{Field: fieldCustomThemes + "[" + strconv.Itoa(i) + "]", Message: messageThemes})
				break
			}
			ids = append(ids, t.Id)
		}
	}
	if !slices.Contains(SystemThemes(), p.ThemeID) && !slices.Contains(ids, p.ThemeID) {
		details = append(details, apperrors.IErrorDetail{Field: fieldThemeID, Message: messageTheme})
	}
	if len(details) > 0 {
		return apperrors.ValidationFailed(details...)
	}
	return nil
}
