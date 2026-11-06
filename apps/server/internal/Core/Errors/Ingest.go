package apperrors

import (
	"net/http"
	"time"
)

const (
	CodeIngestTokenInvalid = "INGEST_TOKEN_INVALID"    //nolint:gosec // A public error code, not a credential.
	CodeIngestTokenRevoked = "INGEST_TOKEN_REVOKED"    //nolint:gosec // A public error code, not a credential.
	CodeIngestTokenExpired = "INGEST_TOKEN_EXPIRED"    //nolint:gosec // A public error code, not a credential.
	CodeIngestTokenLimit   = "INGEST_TOKEN_LIMIT"      //nolint:gosec // A public error code, not a credential.
	CodeIngestTokenName    = "INGEST_TOKEN_NAME_TAKEN" //nolint:gosec // A public error code, not a credential.
	CodeRefNotAllowed      = "REF_NOT_ALLOWED_FOR_TOKEN"
	CodeDigestMismatch     = "DIGEST_MISMATCH"
	CodeArchiveUnsafe      = "ARCHIVE_UNSAFE"
	CodeArtifactTooLarge   = "ARTIFACT_TOO_LARGE"
	CodeArtifactQuota      = "ARTIFACT_QUOTA_EXCEEDED"
	CodeLengthRequired     = "LENGTH_REQUIRED"
	CodeStorageFull        = "STORAGE_FULL"
	CodeUploadTooSlow      = "UPLOAD_TOO_SLOW"
	CodeIngestTokenQuota   = "INGEST_TOKEN_QUOTA"             //nolint:gosec // A public error code, not a credential.
	CodeReachUnconfirmed   = "INGEST_TOKEN_REACH_UNCONFIRMED" //nolint:gosec // A public error code, not a credential.

	KeyIngestTokenInvalid = "errors.ingestTokenInvalid"
	KeyIngestTokenRevoked = "errors.ingestTokenRevoked"
	KeyIngestTokenExpired = "errors.ingestTokenExpired"
	KeyIngestTokenLimit   = "errors.ingestTokenLimit"
	KeyIngestTokenName    = "errors.ingestTokenNameTaken"
	KeyRefNotAllowed      = "errors.refNotAllowedForToken"
	KeyDigestMismatch     = "errors.ingestDigestMismatch"
	KeyArchiveUnsafe      = "errors.ingestArchiveUnsafe"
	KeyArtifactTooLarge   = "errors.ingestTooLarge"
	KeyArtifactQuota      = "errors.ingestQuotaExceeded"
	KeyLengthRequired     = "errors.ingestLengthRequired"
	KeyStorageFull        = "errors.ingestStorageFull"
	KeyIngestBusy         = "errors.ingestBusy"
	KeyUploadTooSlow      = "errors.ingestTooSlow"
	KeyIngestTokenQuota   = "errors.ingestTokenQuota"
	KeyReachUnconfirmed   = "errors.ingestReachUnconfirmed"
)

func IngestTokenInvalid() *AppError {
	return New(http.StatusUnauthorized, CodeIngestTokenInvalid, KeyIngestTokenInvalid)
}

func IngestTokenRevoked() *AppError {
	return New(http.StatusUnauthorized, CodeIngestTokenRevoked, KeyIngestTokenRevoked)
}

func IngestTokenExpired() *AppError {
	return New(http.StatusUnauthorized, CodeIngestTokenExpired, KeyIngestTokenExpired)
}

func IngestTokenLimit() *AppError {
	return New(http.StatusConflict, CodeIngestTokenLimit, KeyIngestTokenLimit)
}

func IngestTokenNameTaken() *AppError {
	return New(http.StatusConflict, CodeIngestTokenName, KeyIngestTokenName)
}

func RefNotAllowedForToken() *AppError {
	return New(http.StatusForbidden, CodeRefNotAllowed, KeyRefNotAllowed)
}

func DigestMismatch() *AppError {
	return New(http.StatusUnprocessableEntity, CodeDigestMismatch, KeyDigestMismatch)
}

func ArchiveUnsafe() *AppError {
	return New(http.StatusUnprocessableEntity, CodeArchiveUnsafe, KeyArchiveUnsafe)
}

func ArtifactTooLarge() *AppError {
	return New(http.StatusRequestEntityTooLarge, CodeArtifactTooLarge, KeyArtifactTooLarge)
}

func ArtifactQuotaExceeded() *AppError {
	return New(http.StatusRequestEntityTooLarge, CodeArtifactQuota, KeyArtifactQuota)
}

func LengthRequired() *AppError {
	return New(http.StatusLengthRequired, CodeLengthRequired, KeyLengthRequired)
}

func StorageFull() *AppError {
	return New(http.StatusInsufficientStorage, CodeStorageFull, KeyStorageFull)
}

func IngestBusy(retryAfter time.Duration) *AppError {
	e := New(http.StatusServiceUnavailable, CodeServiceUnavailable, KeyIngestBusy)
	e.RetryAfter = retryAfter
	return e
}

func UploadTooSlow() *AppError {
	return New(http.StatusRequestTimeout, CodeUploadTooSlow, KeyUploadTooSlow)
}

func IngestTokenQuota() *AppError {
	return New(http.StatusRequestEntityTooLarge, CodeIngestTokenQuota, KeyIngestTokenQuota)
}

func IngestReachUnconfirmed(details ...IErrorDetail) *AppError {
	e := New(http.StatusUnprocessableEntity, CodeReachUnconfirmed, KeyReachUnconfirmed)
	e.Details = details
	return e
}
