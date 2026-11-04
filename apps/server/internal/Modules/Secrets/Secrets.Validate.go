package secrets

import (
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	fieldName                    = "Name"
	fieldValue                   = "Value"
	fieldDescription             = "Description"
	fieldVisibility              = "Visibility"
	fieldProjectIDs              = "ProjectIds"
	fieldAvailableToPullRequests = "AvailableToPullRequests"
	fieldVersion                 = "Version"
	fieldHost                    = "Host"
	fieldUsername                = "Username"
	fieldPassword                = "Password"

	messageInvalid      = "errors.validationFailed"
	messageValue        = "errors.secretValueInvalid"
	messageRegistryHost = "errors.registryHostInvalid"

	reservedPrefix = "DHOW_"
	dockerHubHost  = "registry-1.docker.io"
)

var (
	namePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	dockerAlias  = []string{"docker.io", "index.docker.io"}
	maxPortValue = 65535
)

func ValidName(name string) bool { return namePattern.MatchString(name) }

func Reserved(name string) bool { return strings.HasPrefix(strings.ToUpper(name), reservedPrefix) }

func invalid(field, message string) *apperrors.AppError {
	return apperrors.ValidationFailed(apperrors.IErrorDetail{Field: field, Message: message})
}

func checkName(name string) error {
	if !ValidName(name) {
		return invalid(fieldName, messageInvalid)
	}
	if Reserved(name) {
		return apperrors.SecretNameReserved()
	}
	return nil
}

func plainText(v string) bool { return utf8.ValidString(v) && !strings.ContainsRune(v, 0) }

func checkSecretValue(v string) error {
	if !plainText(v) {
		return invalid(fieldValue, messageValue)
	}
	if len(v) < config.SecretMinBytes {
		return apperrors.SecretTooShort()
	}
	if len(v) > config.SecretMaxBytes {
		return apperrors.SecretTooLarge()
	}
	return nil
}

func checkVariableValue(v string) error {
	if !plainText(v) || len(v) > config.VariableMaxBytes {
		return invalid(fieldValue, messageValue)
	}
	return nil
}

func visibleText(v string, limit int) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > limit {
		return false
	}
	for _, r := range v {
		if unicode.Is(unicode.Cf, r) || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return false
		}
	}
	return true
}

func checkSecretInput(scope string, in models.PutSecretInput) error {
	if err := checkSecretValue(in.Value); err != nil {
		return err
	}
	var details []apperrors.IErrorDetail
	add := func(field string) {
		details = append(details, apperrors.IErrorDetail{Field: field, Message: messageInvalid})
	}
	if in.Description != nil && !visibleText(*in.Description, config.SecretDescriptionMax) {
		add(fieldDescription)
	}
	if scope == models.ScopeEnvironment && in.AvailableToPullRequests != nil {
		add(fieldAvailableToPullRequests)
	}
	if scope != models.ScopeOrganization && (in.Visibility != nil || in.ProjectIDs != nil) {
		add(fieldVisibility)
	}
	if in.Visibility != nil && *in.Visibility != models.VisibilityAll && *in.Visibility != models.VisibilitySelected {
		add(fieldVisibility)
	}
	if in.ProjectIDs != nil && !validProjects(*in.ProjectIDs, in.Visibility) {
		add(fieldProjectIDs)
	}
	if len(details) > 0 {
		return apperrors.ValidationFailed(details...)
	}
	return nil
}

func validProjects(ids []uuid.UUID, visibility *string) bool {
	if visibility == nil || *visibility != models.VisibilitySelected || len(ids) > config.SecretProjectsMax {
		return false
	}
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return len(slices.Compact(sorted)) == len(ids) && !slices.Contains(ids, uuid.Nil)
}

func NormalizeHost(raw string) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(raw))
	if h == "" || len(h) > config.RegistryHostMax || strings.ContainsAny(h, "/@?#*[] ") {
		return "", false
	}
	name, port, hasPort := strings.Cut(h, ":")
	if net.ParseIP(name) != nil {
		return "", false
	}
	rule, err := jobspec.ParseEgress(h)
	if err != nil || rule.Wildcard {
		return "", false
	}
	if hasPort {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > maxPortValue || strconv.Itoa(n) != port {
			return "", false
		}
	}
	if !hasPort && slices.Contains(dockerAlias, name) {
		return dockerHubHost, true
	}
	return h, true
}

func checkRegistryInput(in models.RegistryInput, withHost bool) (string, error) {
	var details []apperrors.IErrorDetail
	host := ""
	if withHost {
		h, ok := NormalizeHost(in.Host)
		if !ok {
			details = append(details, apperrors.IErrorDetail{Field: fieldHost, Message: messageRegistryHost})
		}
		host = h
	}
	if in.Username == "" || !visibleText(in.Username, config.RegistryUsernameMax) || strings.ContainsAny(in.Username, "\n\t:") {
		details = append(details, apperrors.IErrorDetail{Field: fieldUsername, Message: messageInvalid})
	}
	if in.Password == "" || len(in.Password) > config.RegistryPasswordMax || !plainText(in.Password) {
		details = append(details, apperrors.IErrorDetail{Field: fieldPassword, Message: messageValue})
	}
	if len(details) > 0 {
		return "", apperrors.ValidationFailed(details...)
	}
	return host, nil
}
