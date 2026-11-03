package config

import (
	"errors"
	"regexp"
)

const (
	EnvOnIncompatible  = "DHOW_ON_INCOMPATIBLE"
	EnvInstanceID      = "DHOW_INSTANCE_ID"
	OnIncompatibleExit = "exit"
	OnIncompatibleWait = "wait"
)

var reInstanceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func parseOnIncompatible(v string) (string, error) {
	switch v {
	case "", OnIncompatibleExit:
		return OnIncompatibleExit, nil
	case OnIncompatibleWait:
		return OnIncompatibleWait, nil
	}
	return "", errors.New("must be exit or wait")
}

func parseInstanceID(v string) (string, error) {
	if v == "" || reInstanceID.MatchString(v) {
		return v, nil
	}
	return "", errors.New("must be 1 to 128 letters, digits, dots, dashes or underscores, starting with a letter or digit")
}
