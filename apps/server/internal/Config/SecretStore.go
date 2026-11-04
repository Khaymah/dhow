package config

import "time"

const (
	SecretSealLabel           = "dhow/secret"
	RegistryCredentialLabel   = "dhow/registry-credential"
	SecretMinBytes            = 8
	SecretMaxBytes            = 32 * 1024
	SecretSealedMax           = 33024
	SecretDescriptionMax      = 256
	SecretsPerOrganization    = 500
	SecretsPerProject         = 200
	SecretsPerEnvironment     = 100
	SecretVersionsKept        = 5
	SecretVersionRetention    = 30 * 24 * time.Hour
	SecretsPerJob             = 50
	SecretBytesPerJob         = 256 * 1024
	SecretProjectsMax         = 500
	VariableMaxBytes          = 4096
	VariablesPerScope         = 256
	RegistryCredentialsMax    = 20
	RegistryUsernameMax       = 256
	RegistryPasswordMax       = 4096
	RegistryPasswordSealedMax = 4352
	RegistryHostMax           = 259
)
