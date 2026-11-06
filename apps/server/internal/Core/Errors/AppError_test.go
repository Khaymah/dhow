package apperrors_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
)

func TestConstructors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		wantStatus int
		wantCode   string
		wantKey    string
	}{
		{"bad request", apperrors.BadRequest(), http.StatusBadRequest, "BAD_REQUEST", "errors.badRequest"},
		{"validation failed", apperrors.ValidationFailed(), http.StatusUnprocessableEntity, "VALIDATION_FAILED", "errors.validationFailed"},
		{"not found", apperrors.NotFound(), http.StatusNotFound, "NOT_FOUND", "errors.notFound"},
		{"method not allowed", apperrors.MethodNotAllowed(), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "errors.methodNotAllowed"},
		{"payload too large", apperrors.PayloadTooLarge(), http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "errors.payloadTooLarge"},
		{"service unavailable", apperrors.ServiceUnavailable(), http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "errors.serviceUnavailable"},
		{"internal", apperrors.Internal(errors.New("boom")), http.StatusInternalServerError, "INTERNAL", "errors.internal"},
		{"unauthenticated", apperrors.Unauthenticated(), http.StatusUnauthorized, "UNAUTHENTICATED", "errors.unauthenticated"},
		{"forbidden", apperrors.Forbidden(), http.StatusForbidden, "FORBIDDEN", "errors.forbidden"},
		{"csrf failed", apperrors.CSRFFailed(), http.StatusForbidden, "CSRF_FAILED", "errors.csrfFailed"},
		{"invalid credentials", apperrors.InvalidCredentials(0), http.StatusUnauthorized, "INVALID_CREDENTIALS", "errors.invalidCredentials"},
		{"invalid current password", apperrors.InvalidCurrentPassword(0), http.StatusForbidden, "INVALID_CURRENT_PASSWORD", "errors.invalidCurrentPassword"},
		{"password breached", apperrors.PasswordBreached(), http.StatusUnprocessableEntity, "PASSWORD_BREACHED", "errors.passwordBreached"},
		{"setup completed", apperrors.SetupCompleted(), http.StatusConflict, "SETUP_COMPLETED", "errors.setupCompleted"},
		{"setup token invalid", apperrors.SetupTokenInvalid(), http.StatusForbidden, "SETUP_TOKEN_INVALID", "errors.setupTokenInvalid"},
		{"second factor required", apperrors.SecondFactorRequired(), http.StatusForbidden, "SECOND_FACTOR_REQUIRED", "errors.secondFactorRequired"},
		{"target key needs factor", apperrors.TargetKeyNeedsFactor(), http.StatusForbidden, "TARGET_KEY_NEEDS_FACTOR", "errors.targetKeyNeedsFactor"},
		{"conflict", apperrors.Conflict(), http.StatusConflict, "CONFLICT", "errors.conflict"},
		{"mfa challenge expired", apperrors.MfaChallengeExpired(), http.StatusUnauthorized, "MFA_CHALLENGE_EXPIRED", "errors.mfaChallengeExpired"},
		{"invalid mfa code", apperrors.InvalidMfaCode(0), http.StatusUnauthorized, "INVALID_MFA_CODE", "errors.invalidMfaCode"},
		{"invalid current factor", apperrors.InvalidCurrentFactor(0), http.StatusForbidden, "INVALID_CURRENT_FACTOR", "errors.invalidCurrentFactor"},
		{"enrollment expired", apperrors.EnrollmentExpired(), http.StatusConflict, "ENROLLMENT_EXPIRED", "errors.enrollmentExpired"},
		{"passkey rejected", apperrors.PasskeyRejected(), http.StatusUnprocessableEntity, "PASSKEY_REJECTED", "errors.passkeyRejected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantStatus, tt.err.Status)
			assert.Equal(t, tt.wantCode, tt.err.Code)
			assert.Equal(t, tt.wantKey, tt.err.Message)
		})
	}
}

func TestErrorAndUnwrap(t *testing.T) {
	cause := errors.New("relation does not exist")
	ae := apperrors.Internal(cause)
	assert.ErrorIs(t, ae, cause)
	assert.Contains(t, ae.Error(), "relation does not exist")
	assert.Equal(t, "NOT_FOUND: errors.notFound", apperrors.NotFound().Error())
}

func TestJSONOmitsInternals(t *testing.T) {
	ae := apperrors.ValidationFailed(apperrors.IErrorDetail{Field: "Name", Message: "required"})
	ae.Err = errors.New("secret internal text")
	raw, err := json.Marshal(ae)
	require.NoError(t, err)
	assert.JSONEq(t, `{"Code":"VALIDATION_FAILED","Message":"errors.validationFailed","Details":[{"Field":"Name","Message":"required"}]}`, string(raw))

	raw, err = json.Marshal(apperrors.NotFound())
	require.NoError(t, err)
	assert.JSONEq(t, `{"Code":"NOT_FOUND","Message":"errors.notFound"}`, string(raw))
}

func TestFrom(t *testing.T) {
	notFound := apperrors.NotFound()
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"app error", notFound, "NOT_FOUND"},
		{"wrapped app error", fmt.Errorf("load: %w", notFound), "NOT_FOUND"},
		{"plain error", errors.New("boom"), "INTERNAL"},
		{"nil", nil, "INTERNAL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, apperrors.From(tt.err).Code)
		})
	}
}
