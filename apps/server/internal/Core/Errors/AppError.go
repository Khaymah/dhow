package apperrors

import (
	"errors"
	"fmt"
	"time"
)

type IErrorDetail struct {
	Field   string `json:"Field"`
	Message string `json:"Message"`
}

type AppError struct {
	Status   int            `json:"-"`
	Code     string         `json:"Code"`
	Message  string         `json:"Message"`
	Fix      string         `json:"Fix,omitempty"`
	DocsPath string         `json:"DocsPath,omitempty"`
	Details  []IErrorDetail `json:"Details,omitempty"`
	Err      error          `json:"-"`

	RetryAfter time.Duration `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func From(err error) *AppError {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return Internal(err)
}
