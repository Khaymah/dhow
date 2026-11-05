package apperrors

import "net/http"

const (
	CodeEnvironmentExists          = "ENVIRONMENT_EXISTS"
	CodeEnvironmentVersionConflict = "ENVIRONMENT_VERSION_CONFLICT"
	CodeReviewerNotEligible        = "REVIEWER_NOT_ELIGIBLE"
	KeyEnvironmentExists           = "errors.environmentExists"
	KeyEnvironmentVersionConflict  = "errors.environmentVersionConflict"
	KeyReviewerNotEligible         = "errors.reviewerNotEligible"

	CodeSelfReviewNotAllowed      = "SELF_REVIEW_NOT_ALLOWED"
	CodeReviewerNeedsSecondFactor = "REVIEWER_NEEDS_SECOND_FACTOR"
	CodeApprovalNotPending        = "APPROVAL_NOT_PENDING"
	CodeBypassNotAllowed          = "BYPASS_NOT_ALLOWED"
	KeySelfReviewNotAllowed       = "errors.selfReviewNotAllowed"
	KeyReviewerNeedsSecondFactor  = "errors.reviewerNeedsSecondFactor"
	KeyApprovalNotPending         = "errors.approvalNotPending"
	KeyBypassNotAllowed           = "errors.bypassNotAllowed"
)

func SelfReviewNotAllowed() *AppError {
	return New(http.StatusForbidden, CodeSelfReviewNotAllowed, KeySelfReviewNotAllowed)
}

func ReviewerNeedsSecondFactor() *AppError {
	return New(http.StatusForbidden, CodeReviewerNeedsSecondFactor, KeyReviewerNeedsSecondFactor)
}

func ApprovalNotPending() *AppError {
	return New(http.StatusConflict, CodeApprovalNotPending, KeyApprovalNotPending)
}

func BypassNotAllowed() *AppError {
	return New(http.StatusForbidden, CodeBypassNotAllowed, KeyBypassNotAllowed)
}

func EnvironmentExists() *AppError {
	return New(http.StatusConflict, CodeEnvironmentExists, KeyEnvironmentExists)
}

func EnvironmentVersionConflict() *AppError {
	return New(http.StatusConflict, CodeEnvironmentVersionConflict, KeyEnvironmentVersionConflict)
}

func ReviewerNotEligible(details ...IErrorDetail) *AppError {
	e := New(http.StatusUnprocessableEntity, CodeReviewerNotEligible, KeyReviewerNotEligible)
	e.Details = details
	return e
}
