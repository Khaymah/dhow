package apperrors

import "net/http"

const (
	CodeArtifactGone = "ARTIFACT_GONE"
	KeyArtifactGone  = "errors.artifactGone"
)

func ArtifactGone() *AppError {
	return New(http.StatusGone, CodeArtifactGone, KeyArtifactGone)
}
