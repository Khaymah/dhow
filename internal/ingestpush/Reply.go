package ingestpush

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxURLBytes = 2048

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
)

type Result struct {
	Name      string
	Digest    string
	SizeBytes int64
	Created   bool
	Triggered bool
	Reason    string
	RunID     string
	RunNumber int64
	RunURL    string
	Attempts  int
}

type replyData struct {
	Name      string `json:"Name"`
	Digest    string `json:"Digest"`
	SizeBytes int64  `json:"SizeBytes"`
	Created   bool   `json:"Created"`
	Triggered bool   `json:"Triggered"`
	Reason    string `json:"Reason"`
	RunID     string `json:"RunId"`
	RunNumber int64  `json:"RunNumber"`
	RunURL    string `json:"RunUrl"`
}

type replyError struct {
	Code string `json:"Code"`
}

type reply struct {
	Success bool        `json:"Success"`
	Data    *replyData  `json:"Data"`
	Error   *replyError `json:"Error"`
}

var codeClass = map[string]Class{
	CodeIngestTokenInvalid: ClassToken,
	CodeIngestTokenRevoked: ClassToken,
	CodeIngestTokenExpired: ClassToken,
	CodeRefNotAllowed:      ClassToken,
	CodeDigestMismatch:     ClassRefused,
	CodeArchiveUnsafe:      ClassRefused,
	CodeArtifactTooLarge:   ClassRefused,
	CodeArtifactQuota:      ClassRefused,
	CodeLengthRequired:     ClassRefused,
	CodeIngestTokenQuota:   ClassRefused,
	CodeValidationFailed:   ClassRefused,
	CodeProjectArchived:    ClassRefused,
	CodeUploadTooSlow:      ClassTemporary,
	CodeRateLimited:        ClassTemporary,
	CodeServiceUnavailable: ClassTemporary,
}

func readReply(resp *http.Response, digest string) (Result, *Error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
	if err != nil {
		return Result{}, &Error{Class: ClassTemporary, Code: CodeNetwork, Status: resp.StatusCode, Err: err}
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return accepted(resp.StatusCode, body, digest)
	}
	return refusal(resp, body)
}

func accepted(status int, body []byte, digest string) (Result, *Error) {
	var r reply
	if err := json.Unmarshal(body, &r); err != nil || !r.Success || r.Data == nil {
		return Result{}, &Error{Class: ClassUnexpected, Code: CodeBadResponse, Status: status, Err: ErrBadResponse}
	}
	d := r.Data
	if d.Digest != digest {
		return Result{}, &Error{Class: ClassUnexpected, Code: CodeDigestDiffer, Status: status, Err: ErrDigestDiffer}
	}
	if !safeReply(d) {
		return Result{}, &Error{Class: ClassUnexpected, Code: CodeBadResponse, Status: status, Err: ErrBadResponse}
	}
	return Result{
		Name: d.Name, Digest: d.Digest, SizeBytes: d.SizeBytes, Created: status == http.StatusCreated, Triggered: d.Triggered,
		Reason: d.Reason, RunID: d.RunID, RunNumber: d.RunNumber, RunURL: d.RunURL,
	}, nil
}

func safeReply(d *replyData) bool {
	if d.SizeBytes < 0 || d.RunNumber < 0 || !plainText(d.Name, 64) || !plainText(d.Reason, 64) {
		return false
	}
	if d.RunID != "" && !uuidPattern.MatchString(d.RunID) {
		return false
	}
	if d.RunURL == "" {
		return true
	}
	u, err := url.Parse(d.RunURL)
	return err == nil && plainText(d.RunURL, maxURLBytes) && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func refusal(resp *http.Response, body []byte) (Result, *Error) {
	fail := &Error{Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	var r reply
	if json.Unmarshal(body, &r) == nil && r.Error != nil && codePattern.MatchString(r.Error.Code) {
		fail.Code = r.Error.Code
		if class, ok := codeClass[r.Error.Code]; ok {
			fail.Class = class
			return Result{}, fail
		}
	}
	classifyStatus(fail)
	return Result{}, fail
}

func classifyStatus(fail *Error) {
	status := fail.Status
	switch {
	case status >= 300 && status < 400:
		fail.Class, fail.Code = ClassUnexpected, CodeRedirect
	case status == http.StatusRequestEntityTooLarge:
		fail.Class, fail.Code = ClassRefused, CodeProxyLimit
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		fail.Class = ClassToken
		fail.Code = orCode(fail.Code, CodeStatus4xx)
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests:
		fail.Class = ClassTemporary
		fail.Code = orCode(fail.Code, CodeStatus4xx)
	case status >= 400 && status < 500:
		fail.Class = ClassRefused
		fail.Code = orCode(fail.Code, CodeStatus4xx)
	case status >= 500 && status < 600:
		fail.Class = ClassTemporary
		fail.Code = orCode(fail.Code, CodeStatus5xx)
	default:
		fail.Class, fail.Code = ClassUnexpected, CodeStatusOther
	}
}

func orCode(code, fallback string) string {
	if code != "" {
		return code
	}
	return fallback
}

func retryAfter(v string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}
