package apperrors

import "net/http"

const (
	CodeEnvironmentExists          = "ENVIRONMENT_EXISTS"
	CodeEnvironmentVersionConflict = "ENVIRONMENT_VERSION_CONFLICT"
	CodeReviewerNotEligible        = "REVIEWER_NOT_ELIGIBLE"
	KeyEnvironmentExists           = "errors.environmentExists"
	KeyEnvironmentVersionConflict  = "errors.environmentVersionConflict"
	KeyReviewerNotEligible         = "errors.reviewerNotEligible"
)

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
