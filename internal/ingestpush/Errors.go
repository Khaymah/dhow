package ingestpush

import (
	"errors"
	"fmt"
	"time"
)

type Class int

const (
	ClassInput Class = iota + 1
	ClassRefused
	ClassToken
	ClassTemporary
	ClassUnexpected
)

const (
	CodeIngestTokenInvalid = "INGEST_TOKEN_INVALID" //nolint:gosec // An error code, not a credential. allow-secret
	CodeIngestTokenRevoked = "INGEST_TOKEN_REVOKED" //nolint:gosec // An error code, not a credential. allow-secret
	CodeIngestTokenExpired = "INGEST_TOKEN_EXPIRED" //nolint:gosec // An error code, not a credential. allow-secret
	CodeRefNotAllowed      = "REF_NOT_ALLOWED_FOR_TOKEN"
	CodeDigestMismatch     = "DIGEST_MISMATCH"
	CodeArchiveUnsafe      = "ARCHIVE_UNSAFE"
	CodeArtifactTooLarge   = "ARTIFACT_TOO_LARGE"
	CodeArtifactQuota      = "ARTIFACT_QUOTA_EXCEEDED"
	CodeLengthRequired     = "LENGTH_REQUIRED"
	CodeUploadTooSlow      = "UPLOAD_TOO_SLOW"
	CodeIngestTokenQuota   = "INGEST_TOKEN_QUOTA" //nolint:gosec // An error code, not a credential. allow-secret
	CodeRateLimited        = "RATE_LIMITED"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeValidationFailed   = "VALIDATION_FAILED"
	CodeProjectArchived    = "PROJECT_ARCHIVED"

	CodePackHasGit   = "PACK_HAS_GIT_DIR"
	CodeInput        = "INPUT"
	CodeNetwork      = "NETWORK"
	CodeTimeout      = "TIMEOUT"
	CodeRedirect     = "REDIRECT"
	CodeBadResponse  = "BAD_RESPONSE"
	CodeDigestDiffer = "RESPONSE_DIGEST_DIFFERS"
	CodeStatus4xx    = "HTTP_4XX"
	CodeStatus5xx    = "HTTP_5XX"
	CodeStatusOther  = "HTTP_UNEXPECTED"
	CodeProxyLimit   = "HTTP_413"
)

var (
	ErrServer       = errors.New("the server URL must be https, or http on this machine, and carry no credentials, query or fragment")
	ErrToken        = errors.New("the ingest token is missing or not shaped like dhow_ing_ and 43 characters")
	ErrName         = errors.New("the artifact name is not valid")
	ErrRef          = errors.New("the ref is not valid")
	ErrRefKind      = errors.New("the ref kind is not branch or tag")
	ErrSha          = errors.New("the commit is not a lowercase hex SHA-1 or SHA-256")
	ErrProvenance   = errors.New("the provenance is not valid")
	ErrCAFile       = errors.New("the CA bundle cannot be read or holds no certificate")
	ErrSource       = errors.New("the path is not a folder or a tar.gz file that can be sent")
	ErrNotGzip      = errors.New("the file is not a gzip stream")
	ErrHasGit       = errors.New("the folder has a .git entry, and packing it would send the history and maybe credentials")
	ErrTooBig       = errors.New("the archive is larger than the limit")
	ErrNotLoopback  = errors.New("the http server name does not resolve to a loopback address")
	ErrEmpty        = errors.New("the folder or file is empty")
	ErrRetries      = errors.New("the number of retries is out of range")
	ErrBadResponse  = errors.New("the server answered with something that is not a Dhow ingest reply")
	ErrDigestDiffer = errors.New("the server stored a different digest from the one sent")
)

type Error struct {
	Class      Class
	Status     int
	Code       string
	RetryAfter time.Duration
	Attempts   int
	Err        error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

func inputError(err error) *Error {
	return &Error{Class: ClassInput, Code: CodeInput, Err: err}
}

func (e *Error) retryable() bool { return e.Class == ClassTemporary }
