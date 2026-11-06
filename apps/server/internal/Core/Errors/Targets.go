package apperrors

import "net/http"

const (
	CodeTargetExists          = "TARGET_EXISTS"
	CodeTargetDuplicate       = "TARGET_DUPLICATE"
	CodeTargetVersionConflict = "TARGET_VERSION_CONFLICT"
	CodeTargetHostBlocked     = "TARGET_HOST_BLOCKED"
	CodeTargetHostUnreachable = "TARGET_HOST_UNREACHABLE"
	CodeTargetKeyNotFound     = "TARGET_KEY_NOT_FOUND"
	CodeDeployPathInvalid     = "DEPLOY_PATH_INVALID"
	CodeTargetKeyNameTaken    = "TARGET_KEY_NAME_TAKEN"
	KeyTargetExists           = "errors.targetExists"
	KeyTargetDuplicate        = "errors.targetDuplicate"
	KeyTargetVersionConflict  = "errors.targetVersionConflict"
	KeyTargetHostBlocked      = "errors.targetHostBlocked"
	KeyTargetHostUnreachable  = "errors.targetHostUnreachable"
	KeyTargetKeyNotFound      = "errors.targetKeyNotFound"
	KeyDeployPathInvalid      = "errors.deployPathInvalid"
	KeyTargetKeyNameTaken     = "errors.targetKeyNameTaken"
)

func TargetExists() *AppError {
	return New(http.StatusConflict, CodeTargetExists, KeyTargetExists)
}

func TargetDuplicate() *AppError {
	return New(http.StatusConflict, CodeTargetDuplicate, KeyTargetDuplicate)
}

func TargetVersionConflict() *AppError {
	return New(http.StatusConflict, CodeTargetVersionConflict, KeyTargetVersionConflict)
}

func TargetHostBlocked(details ...IErrorDetail) *AppError {
	e := New(http.StatusUnprocessableEntity, CodeTargetHostBlocked, KeyTargetHostBlocked)
	e.Details = details
	return e
}

func TargetHostUnreachable(err error) *AppError {
	e := New(http.StatusBadGateway, CodeTargetHostUnreachable, KeyTargetHostUnreachable)
	e.Err = err
	return e
}

func TargetKeyNotFound(details ...IErrorDetail) *AppError {
	e := New(http.StatusUnprocessableEntity, CodeTargetKeyNotFound, KeyTargetKeyNotFound)
	e.Details = details
	return e
}

func DeployPathInvalid(details ...IErrorDetail) *AppError {
	e := New(http.StatusUnprocessableEntity, CodeDeployPathInvalid, KeyDeployPathInvalid)
	e.Details = details
	return e
}

func TargetKeyNameTaken() *AppError {
	return New(http.StatusConflict, CodeTargetKeyNameTaken, KeyTargetKeyNameTaken)
}

const (
	CodeTargetCheckUnavailable = "TARGET_CHECK_UNAVAILABLE"
	KeyTargetCheckUnavailable  = "errors.targetCheckUnavailable"
)

func TargetCheckUnavailable() *AppError {
	return New(http.StatusUnprocessableEntity, CodeTargetCheckUnavailable, KeyTargetCheckUnavailable)
}

const (
	CodeCheckRunNotRerunnable = "CHECK_RUN_NOT_RERUNNABLE"
	KeyCheckRunNotRerunnable  = "errors.checkRunNotRerunnable"
)

func CheckRunNotRerunnable() *AppError {
	return New(http.StatusConflict, CodeCheckRunNotRerunnable, KeyCheckRunNotRerunnable)
}
