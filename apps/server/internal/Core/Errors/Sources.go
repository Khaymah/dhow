package apperrors

import "net/http"

const (
	CodeRepositoryNotLinked     = "REPOSITORY_NOT_LINKED"
	CodeGitHostBlocked          = "GIT_HOST_BLOCKED"
	CodeGitHostUnreachable      = "GIT_HOST_UNREACHABLE"
	CodeHostKeyNotPending       = "HOST_KEY_NOT_PENDING"
	CodeHostKeyMismatch         = "HOST_KEY_FINGERPRINT_MISMATCH"
	CodeDeployKeyNotApplicable  = "DEPLOY_KEY_NOT_APPLICABLE"
	CodeWebhookRejected         = "WEBHOOK_REJECTED"
	KeyRepositoryNotLinked      = "errors.repositoryNotLinked"
	KeyGitHostBlocked           = "errors.gitHostBlocked"
	KeyGitHostUnreachable       = "errors.gitHostUnreachable"
	KeyHostKeyNotPending        = "errors.hostKeyNotPending"
	KeyHostKeyMismatch          = "errors.hostKeyFingerprintMismatch"
	KeyDeployKeyNotApplicable   = "errors.deployKeyNotApplicable"
	KeyWebhookRejected          = "errors.webhookRejected"
	KeyWebhookDeliveryIDMissing = "errors.webhookDeliveryIdMissing"

	CodeRunNotRerunnable       = "RUN_NOT_RERUNNABLE"
	CodeRunNotAwaitingApproval = "RUN_NOT_AWAITING_APPROVAL"
	KeyRunNotRerunnable        = "errors.runNotRerunnable"
	KeyRunNotAwaitingApproval  = "errors.runNotAwaitingApproval"
)

func RunNotRerunnable() *AppError {
	return New(http.StatusConflict, CodeRunNotRerunnable, KeyRunNotRerunnable)
}

func RunNotAwaitingApproval() *AppError {
	return New(http.StatusConflict, CodeRunNotAwaitingApproval, KeyRunNotAwaitingApproval)
}

func RepositoryNotLinked() *AppError {
	return New(http.StatusNotFound, CodeRepositoryNotLinked, KeyRepositoryNotLinked)
}

func GitHostBlocked() *AppError {
	return New(http.StatusUnprocessableEntity, CodeGitHostBlocked, KeyGitHostBlocked)
}

func GitHostUnreachable(err error) *AppError {
	e := New(http.StatusBadGateway, CodeGitHostUnreachable, KeyGitHostUnreachable)
	e.Err = err
	return e
}

func HostKeyNotPending() *AppError {
	return New(http.StatusConflict, CodeHostKeyNotPending, KeyHostKeyNotPending)
}

func HostKeyMismatch() *AppError {
	return New(http.StatusConflict, CodeHostKeyMismatch, KeyHostKeyMismatch)
}

func DeployKeyNotApplicable() *AppError {
	return New(http.StatusConflict, CodeDeployKeyNotApplicable, KeyDeployKeyNotApplicable)
}

func WebhookRejected() *AppError {
	return New(http.StatusUnauthorized, CodeWebhookRejected, KeyWebhookRejected)
}
