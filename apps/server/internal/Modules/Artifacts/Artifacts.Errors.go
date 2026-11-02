package artifacts

import (
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect"
)

const (
	CodeLeaseLost         = "LEASE_LOST"
	CodeGrantRequired     = "DOWNLOAD_GRANT_REQUIRED"
	CodeBlobTooLarge      = "BLOB_TOO_LARGE"
	CodeLengthRequired    = "LENGTH_REQUIRED"
	CodeDigestMismatch    = "DIGEST_MISMATCH"
	CodeStorageFull       = "STORAGE_FULL"
	CodeBlobNotFound      = "BLOB_NOT_FOUND"
	CodeNotDeclared       = "NOT_DECLARED"
	CodeNotPipelineJob    = "NOT_A_PIPELINE_JOB"
	CodeCacheReadOnly     = "CACHE_READ_ONLY"
	CodeArtifactNameTaken = "ARTIFACT_NAME_TAKEN"
	CodeValidationFailed  = "VALIDATION_FAILED"
	CodeInternal          = "INTERNAL"
	CodeUnauthenticated   = "UNAUTHENTICATED"
)

type rejection struct {
	status int
	rpc    connect.Code
	code   string
}

func (r *rejection) Error() string { return r.code }

func reject(status int, rpc connect.Code, code string) *rejection {
	return &rejection{status: status, rpc: rpc, code: code}
}

func errLeaseLost() *rejection {
	return reject(http.StatusConflict, connect.CodeFailedPrecondition, CodeLeaseLost)
}

func errUnauthenticated() *rejection {
	return reject(http.StatusUnauthorized, connect.CodeUnauthenticated, CodeUnauthenticated)
}

func errGrant() *rejection {
	return reject(http.StatusForbidden, connect.CodePermissionDenied, CodeGrantRequired)
}

func errTooLarge() *rejection {
	return reject(http.StatusRequestEntityTooLarge, connect.CodeResourceExhausted, CodeBlobTooLarge)
}

func errDigest() *rejection {
	return reject(http.StatusUnprocessableEntity, connect.CodeInvalidArgument, CodeDigestMismatch)
}

func errFull() *rejection {
	return reject(http.StatusInsufficientStorage, connect.CodeResourceExhausted, CodeStorageFull)
}

func errBlobMissing() *rejection {
	return reject(http.StatusNotFound, connect.CodeNotFound, CodeBlobNotFound)
}

func errNotDeclared() *rejection {
	return reject(http.StatusForbidden, connect.CodePermissionDenied, CodeNotDeclared)
}

func errNotPipeline() *rejection {
	return reject(http.StatusForbidden, connect.CodeFailedPrecondition, CodeNotPipelineJob)
}

func errReadOnly() *rejection {
	return reject(http.StatusForbidden, connect.CodePermissionDenied, CodeCacheReadOnly)
}

func errNameTaken() *rejection {
	return reject(http.StatusConflict, connect.CodeAlreadyExists, CodeArtifactNameTaken)
}

func errInvalid(field string) *rejection {
	return reject(http.StatusBadRequest, connect.CodeInvalidArgument, CodeValidationFailed+": "+field)
}

func errLength() *rejection {
	return reject(http.StatusLengthRequired, connect.CodeInvalidArgument, CodeLengthRequired)
}

func rpcError(err error) error {
	var r *rejection
	if errors.As(err, &r) {
		return connect.NewError(r.rpc, errors.New(r.code))
	}
	return err
}

type httpError struct {
	Code string `json:"Code"`
}

func writeRejection(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(httpError{Code: code})
}
