package apperrors

import "net/http"

const (
	CodeSecretTooShort                 = "SECRET_TOO_SHORT"
	CodeSecretTooLarge                 = "SECRET_TOO_LARGE"
	CodeSecretNameReserved             = "SECRET_NAME_RESERVED"
	CodeSecretLimitReached             = "SECRET_LIMIT_REACHED"
	CodeSecretVersionErased            = "SECRET_VERSION_ERASED"
	CodeVariableLimitReached           = "VARIABLE_LIMIT_REACHED"
	CodeRegistryCredentialExists       = "REGISTRY_CREDENTIAL_EXISTS"        //nolint:gosec // An error code, not a credential.
	CodeRegistryCredentialLimitReached = "REGISTRY_CREDENTIAL_LIMIT_REACHED" //nolint:gosec // An error code, not a credential.
	KeySecretTooShort                  = "errors.secretTooShort"
	KeySecretTooLarge                  = "errors.secretTooLarge"
	KeySecretNameReserved              = "errors.secretNameReserved"
	KeySecretLimitReached              = "errors.secretLimitReached"
	KeySecretVersionErased             = "errors.secretVersionErased"
	KeyVariableLimitReached            = "errors.variableLimitReached"
	KeyRegistryCredentialExists        = "errors.registryCredentialExists"
	KeyRegistryCredentialLimitReached  = "errors.registryCredentialLimitReached"
)

func SecretTooShort() *AppError {
	return New(http.StatusUnprocessableEntity, CodeSecretTooShort, KeySecretTooShort)
}

func SecretTooLarge() *AppError {
	return New(http.StatusUnprocessableEntity, CodeSecretTooLarge, KeySecretTooLarge)
}

func SecretNameReserved() *AppError {
	return New(http.StatusUnprocessableEntity, CodeSecretNameReserved, KeySecretNameReserved)
}

func SecretLimitReached() *AppError {
	return New(http.StatusConflict, CodeSecretLimitReached, KeySecretLimitReached)
}

func SecretVersionErased() *AppError {
	return New(http.StatusConflict, CodeSecretVersionErased, KeySecretVersionErased)
}

func VariableLimitReached() *AppError {
	return New(http.StatusConflict, CodeVariableLimitReached, KeyVariableLimitReached)
}

func RegistryCredentialExists() *AppError {
	return New(http.StatusConflict, CodeRegistryCredentialExists, KeyRegistryCredentialExists)
}

func RegistryCredentialLimitReached() *AppError {
	return New(http.StatusConflict, CodeRegistryCredentialLimitReached, KeyRegistryCredentialLimitReached)
}
