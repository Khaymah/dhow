package apperrors

import (
	"net/http"
	"time"
)

const (
	CodeChannelExists          = "CHANNEL_EXISTS"
	CodeChannelHostNotAllowed  = "CHANNEL_HOST_NOT_ALLOWED"
	CodeChannelHostBlocked     = "CHANNEL_HOST_BLOCKED"
	CodeChannelSecretInvalid   = "CHANNEL_SECRET_INVALID" //nolint:gosec // A public error code, not a credential. allow-secret
	CodeChannelVersionConflict = "CHANNEL_VERSION_CONFLICT"
	CodeChannelDisabled        = "CHANNEL_DISABLED"
	CodeDeliveryNotRetryable   = "DELIVERY_NOT_RETRYABLE"
	CodeTestRateLimited        = "TEST_RATE_LIMITED"

	KeyChannelExists          = "errors.channelExists"
	KeyChannelHostNotAllowed  = "errors.channelHostNotAllowed"
	KeyChannelHostBlocked     = "errors.channelHostBlocked"
	KeyChannelSecretInvalid   = "errors.channelSecretInvalid" //nolint:gosec // An i18n key, not a credential. allow-secret
	KeyChannelVersionConflict = "errors.channelVersionConflict"
	KeyChannelDisabled        = "errors.channelDisabled"
	KeyDeliveryNotRetryable   = "errors.deliveryNotRetryable"
	KeyTestRateLimited        = "errors.testRateLimited"
)

func ChannelExists() *AppError {
	return New(http.StatusConflict, CodeChannelExists, KeyChannelExists)
}

func ChannelHostNotAllowed() *AppError {
	return New(http.StatusUnprocessableEntity, CodeChannelHostNotAllowed, KeyChannelHostNotAllowed)
}

func ChannelHostBlocked() *AppError {
	return New(http.StatusUnprocessableEntity, CodeChannelHostBlocked, KeyChannelHostBlocked)
}

func ChannelSecretInvalid() *AppError {
	return New(http.StatusUnprocessableEntity, CodeChannelSecretInvalid, KeyChannelSecretInvalid)
}

func ChannelVersionConflict() *AppError {
	return New(http.StatusConflict, CodeChannelVersionConflict, KeyChannelVersionConflict)
}

func ChannelDisabled() *AppError {
	return New(http.StatusConflict, CodeChannelDisabled, KeyChannelDisabled)
}

func DeliveryNotRetryable() *AppError {
	return New(http.StatusConflict, CodeDeliveryNotRetryable, KeyDeliveryNotRetryable)
}

func TestRateLimited(retryAfter time.Duration) *AppError {
	e := New(http.StatusTooManyRequests, CodeTestRateLimited, KeyTestRateLimited)
	e.RetryAfter = retryAfter
	return e
}
