package environments

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/pathglob"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	fieldName        = "Name"
	fieldDescription = "Description"
	fieldReason      = "Reason"
	fieldRules       = "Rules."

	ruleAllowedRefs          = "AllowedRefs"
	ruleAllowPullRequests    = "AllowPullRequests"
	ruleReviewers            = "Reviewers"
	ruleRequiredApprovals    = "RequiredApprovals"
	ruleAllowSelfApproval    = "AllowSelfApproval"
	ruleWaitMinutes          = "WaitMinutes"
	ruleApprovalTimeoutHours = "ApprovalTimeoutHours"
	ruleRequirePinnedImages  = "RequirePinnedImages"
	ruleAllowedPlugins       = "AllowedPlugins"
	ruleRequiredIsolation    = "RequiredIsolation"
	ruleAllowAdminBypass     = "AllowAdminBypass"
	ruleSupersedePending     = "SupersedePending"

	messageInvalidValue = "errors.validationFailed"
	messageReviewerUser = "errors.reviewerNotEligibleUser"
	messageReviewerTeam = "errors.reviewerNotEligibleTeam"
)

func invalid(field string) apperrors.IErrorDetail {
	return apperrors.IErrorDetail{Field: field, Message: messageInvalidValue}
}

func ValidName(name string) bool { return pipelinespec.ValidEnvironmentName(name) }

func ValidRefPattern(p string) bool {
	if !pathglob.ValidPattern(p) {
		return false
	}
	if strings.Contains(p, "..") || strings.Contains(p, "//") || strings.Contains(p, "@{") {
		return false
	}
	if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") || strings.HasPrefix(p, "-") {
		return false
	}
	return !strings.ContainsFunc(p, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("~^:", r) })
}

func ValidPluginPrefix(p string) bool {
	if p == "" || len(p) > models.MaxPluginPrefix || !utf8.ValidString(p) || strings.Contains(p, "..") {
		return false
	}
	return !strings.ContainsFunc(p, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '\\' })
}

func validText(v string, maxRunes int, required bool) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > maxRunes || (required && strings.TrimSpace(v) == "") {
		return false
	}
	return !strings.ContainsFunc(v, func(r rune) bool { return r != '\n' && unicode.IsControl(r) })
}

func DefaultRules(defaultBranch string) models.Rules {
	refs := []models.Ref{}
	if defaultBranch == "" {
		defaultBranch = models.DefaultBranch
	}
	if ValidRefPattern(defaultBranch) {
		refs = append(refs, models.Ref{Kind: models.RefBranch, Pattern: defaultBranch})
	}
	return models.Rules{
		AllowedRefs: refs, Reviewers: models.Reviewers{UserIDs: []uuid.UUID{}, TeamIDs: []uuid.UUID{}},
		RequiredApprovals: models.RequiredApprovals, ApprovalTimeoutHours: models.DefaultApprovalTimeoutHours,
		AllowedPlugins: []string{}, RequiredIsolation: models.IsolationContainer, SupersedePending: true,
	}
}

func setIf[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

func Apply(base models.Rules, in *models.RulesInput) models.Rules {
	if in == nil {
		return Normalize(base)
	}
	out := base
	setIf(&out.AllowedRefs, in.AllowedRefs)
	setIf(&out.AllowPullRequests, in.AllowPullRequests)
	setIf(&out.Reviewers, in.Reviewers)
	setIf(&out.RequiredApprovals, in.RequiredApprovals)
	setIf(&out.AllowSelfApproval, in.AllowSelfApproval)
	setIf(&out.WaitMinutes, in.WaitMinutes)
	setIf(&out.ApprovalTimeoutHours, in.ApprovalTimeoutHours)
	setIf(&out.RequirePinnedImages, in.RequirePinnedImages)
	setIf(&out.AllowedPlugins, in.AllowedPlugins)
	setIf(&out.RequiredIsolation, in.RequiredIsolation)
	setIf(&out.AllowAdminBypass, in.AllowAdminBypass)
	setIf(&out.SupersedePending, in.SupersedePending)
	return Normalize(out)
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	out = slices.Compact(out)
	if out == nil {
		return []uuid.UUID{}
	}
	return out
}

func uniqueInOrder[T comparable](in []T) []T {
	out := make([]T, 0, len(in))
	for _, v := range in {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func Normalize(r models.Rules) models.Rules {
	r.AllowedRefs = uniqueInOrder(r.AllowedRefs)
	r.AllowedPlugins = uniqueInOrder(r.AllowedPlugins)
	r.Reviewers = models.Reviewers{UserIDs: uniqueIDs(r.Reviewers.UserIDs), TeamIDs: uniqueIDs(r.Reviewers.TeamIDs)}
	return r
}

func indexed(rule string, i int, sub string) string {
	return fieldRules + rule + "[" + strconv.Itoa(i) + "]" + sub
}

func validateRefs(refs []models.Ref) []apperrors.IErrorDetail {
	if len(refs) > models.MaxAllowedRefs {
		return []apperrors.IErrorDetail{invalid(fieldRules + ruleAllowedRefs)}
	}
	var details []apperrors.IErrorDetail
	for i, ref := range refs {
		if ref.Kind != models.RefBranch && ref.Kind != models.RefTag {
			details = append(details, invalid(indexed(ruleAllowedRefs, i, ".Kind")))
		}
		if !ValidRefPattern(ref.Pattern) {
			details = append(details, invalid(indexed(ruleAllowedRefs, i, ".Pattern")))
		}
	}
	return details
}

func validatePlugins(plugins []string) []apperrors.IErrorDetail {
	if len(plugins) > models.MaxAllowedPlugins {
		return []apperrors.IErrorDetail{invalid(fieldRules + ruleAllowedPlugins)}
	}
	var details []apperrors.IErrorDetail
	for i, p := range plugins {
		if !ValidPluginPrefix(p) {
			details = append(details, invalid(indexed(ruleAllowedPlugins, i, "")))
		}
	}
	return details
}

func ValidateRules(r models.Rules) []apperrors.IErrorDetail {
	details := validateRefs(r.AllowedRefs)
	details = append(details, validatePlugins(r.AllowedPlugins)...)
	if len(r.Reviewers.UserIDs)+len(r.Reviewers.TeamIDs) > models.MaxReviewers ||
		slices.Contains(r.Reviewers.UserIDs, uuid.Nil) || slices.Contains(r.Reviewers.TeamIDs, uuid.Nil) {
		details = append(details, invalid(fieldRules+ruleReviewers))
	}
	if r.RequiredApprovals != models.RequiredApprovals {
		details = append(details, invalid(fieldRules+ruleRequiredApprovals))
	}
	if r.WaitMinutes < 0 || r.WaitMinutes > models.MaxWaitMinutes {
		details = append(details, invalid(fieldRules+ruleWaitMinutes))
	}
	if r.ApprovalTimeoutHours < models.MinApprovalTimeout || r.ApprovalTimeoutHours > models.MaxApprovalTimeout {
		details = append(details, invalid(fieldRules+ruleApprovalTimeoutHours))
	}
	if r.RequiredIsolation != models.IsolationContainer {
		details = append(details, invalid(fieldRules+ruleRequiredIsolation))
	}
	return details
}

func validateInput(name *string, description *string, rules *models.RulesInput) *apperrors.AppError {
	var details []apperrors.IErrorDetail
	if name != nil && !ValidName(*name) {
		details = append(details, invalid(fieldName))
	}
	if description != nil && !validText(*description, models.MaxDescription, false) {
		details = append(details, invalid(fieldDescription))
	}
	details = append(details, ValidateRules(Apply(DefaultRules(models.DefaultBranch), rules))...)
	if len(details) > 0 {
		return apperrors.ValidationFailed(details...)
	}
	return nil
}

func validateReason(reason string) *apperrors.AppError {
	if !validText(reason, models.MaxPauseReason, true) {
		return apperrors.ValidationFailed(invalid(fieldReason))
	}
	return nil
}
