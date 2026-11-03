package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	EnvKeyringFile    = "DHOW_KEYRING_FILE"
	CredentialKeyring = "dhow-keyring"
	envKeyringValue   = "DHOW_KEYRING"
	KeyRewrapEvery    = 30 * time.Second
	KekActiveMaxAge   = 365 * 24 * time.Hour
	BackfillEvery     = 30 * time.Second
	BackfillPause     = 100 * time.Millisecond
	BackfillBatch     = 1000
	KeyRekeyGrace     = 10 * time.Second
)

type Keyring struct {
	Path       string
	Credential bool
}

func LoadKeyringFromEnv() (Keyring, error) { return LoadKeyring(os.Getenv) }

func LoadKeyring(getenv func(string) string) (Keyring, error) {
	if getenv(envKeyringValue) != "" {
		return Keyring{}, fmt.Errorf("%s is set, but the keyring is never read from an environment variable, which docker inspect "+
			"and the process environment reveal; put it in a file and set %s or the systemd credential %s", envKeyringValue, EnvKeyringFile, CredentialKeyring)
	}
	file := getenv(EnvKeyringFile)
	cred := credentialPath(getenv(EnvCredentialsDirectory), CredentialKeyring)
	switch {
	case file != "" && cred != "":
		return Keyring{}, fmt.Errorf("%s: is set more than once (%s, the systemd credential %s); keep exactly one", EnvKeyringFile, EnvKeyringFile, CredentialKeyring)
	case cred != "":
		return Keyring{Path: cred, Credential: true}, nil
	case file == "":
		return Keyring{}, fmt.Errorf("%s: is required; create the keyring with `dhow-server keys init --file <path>` and point %s or the systemd credential %s at it",
			EnvKeyringFile, EnvKeyringFile, CredentialKeyring)
	case !filepath.IsAbs(file) || filepath.Clean(file) != file:
		return Keyring{}, fmt.Errorf("%s: must be an absolute, clean path", EnvKeyringFile)
	}
	return Keyring{Path: file}, nil
}
