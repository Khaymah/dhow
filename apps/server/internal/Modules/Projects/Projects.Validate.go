package projects

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Projects/Models"
)

const (
	maxSlugLen         = 63
	maxNameLen         = 100
	maxPipelinePathLen = 255

	fieldName         = "Name"
	fieldSlug         = "Slug"
	fieldPipelinePath = "PipelinePath"
	fieldForkPolicy   = "ForkPolicy"
	fieldForkCache    = "ForkCacheRestore"

	messageInvalidValue = "errors.validationFailed"
	messageSlugTaken    = "errors.slugTaken"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func invalid(field string) apperrors.IErrorDetail {
	return apperrors.IErrorDetail{Field: field, Message: messageInvalidValue}
}

func ValidSlug(v string) bool { return len(v) <= maxSlugLen && slugPattern.MatchString(v) }

func ValidPipelinePath(v string) bool {
	if v == "" || len(v) > maxPipelinePathLen || !utf8.ValidString(v) {
		return false
	}
	if strings.ContainsFunc(v, unsafePathRune) || strings.HasPrefix(v, ":") {
		return false
	}
	if !strings.HasSuffix(v, ".yml") && !strings.HasSuffix(v, ".yaml") {
		return false
	}
	for segment := range strings.SplitSeq(v, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, "-") {
			return false
		}
	}
	return true
}

func unsafePathRune(r rune) bool {
	return r == '\\' || unicode.IsControl(r) || unicode.IsSpace(r) || unicode.In(r, unicode.Cf, unicode.Co, unicode.Cs, unicode.Zl, unicode.Zp) ||
		r == utf8.RuneError
}

func ValidForkPolicy(v string) bool {
	return v == models.ForkPolicyApproval || v == models.ForkPolicyAutomatic
}

func validateCreate(in models.CreateInput) (models.CreateInput, *apperrors.AppError) {
	var details []apperrors.IErrorDetail
	if !auth.ValidDisplayName(in.Name, maxNameLen) {
		details = append(details, invalid(fieldName))
	}
	if !ValidSlug(in.Slug) {
		details = append(details, invalid(fieldSlug))
	}
	path, policy := models.DefaultPipelinePath, models.ForkPolicyApproval
	if in.PipelinePath != nil {
		path = *in.PipelinePath
	}
	if in.ForkPolicy != nil {
		policy = *in.ForkPolicy
	}
	if !ValidPipelinePath(path) {
		details = append(details, invalid(fieldPipelinePath))
	}
	if !ValidForkPolicy(policy) {
		details = append(details, invalid(fieldForkPolicy))
	}
	if len(details) > 0 {
		return in, apperrors.ValidationFailed(details...)
	}
	in.PipelinePath, in.ForkPolicy = &path, &policy
	return in, nil
}

func validateUpdate(in models.UpdateInput) *apperrors.AppError {
	if in.Name == nil && in.PipelinePath == nil && in.ForkPolicy == nil && in.ForkCache == nil {
		return apperrors.BadRequest()
	}
	var details []apperrors.IErrorDetail
	if in.Name != nil && !auth.ValidDisplayName(*in.Name, maxNameLen) {
		details = append(details, invalid(fieldName))
	}
	if in.PipelinePath != nil && !ValidPipelinePath(*in.PipelinePath) {
		details = append(details, invalid(fieldPipelinePath))
	}
	if in.ForkPolicy != nil && !ValidForkPolicy(*in.ForkPolicy) {
		details = append(details, invalid(fieldForkPolicy))
	}
	if in.ForkCache != nil && *in.ForkCache != models.ForkCacheNone && *in.ForkCache != models.ForkCacheDefault {
		details = append(details, invalid(fieldForkCache))
	}
	if len(details) > 0 {
		return apperrors.ValidationFailed(details...)
	}
	return nil
}
