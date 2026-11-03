package keys

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ExitKeyring       = 79
	exitOther         = 1
	keyringCodePrefix = "KEYRING_"
)

const (
	CodeKeyringMissing        = "KEYRING_MISSING"
	CodeKeyringInsecure       = "KEYRING_INSECURE"
	CodeKeyringMalformed      = "KEYRING_MALFORMED"
	CodeKeyringMissingVersion = "KEYRING_MISSING_VERSION"
	CodeKeyringMismatch       = "KEYRING_MISMATCH"
	CodeKeyringFull           = "KEYRING_FULL"
	CodeKeyringRetired        = "KEYRING_RETIRED"
	CodeKekNotEverywhere      = "KEK_NOT_EVERYWHERE"
	CodeKekNotFound           = "KEK_NOT_FOUND"
	CodeKekRetired            = "KEK_RETIRED"
	CodeKekAlreadyActive      = "KEK_ALREADY_ACTIVE"
	CodeKekNoActive           = "KEK_NO_ACTIVE"
	CodeKeysInUse             = "KEYS_IN_USE"
)

var (
	ErrSealed      = errors.New("sealed value is malformed or was not sealed for this owner, purpose and row")
	ErrShredded    = fmt.Errorf("%w: its data key was destroyed", ErrSealed)
	ErrTooLarge    = errors.New("plaintext is larger than 1 MiB")
	ErrBadOwner    = errors.New("the owner of a data key is not valid")
	ErrBadLabel    = errors.New("the purpose label must be 1 to 255 bytes")
	ErrBadContext  = errors.New("the row context must not be empty")
	ErrNoDataKey   = errors.New("no active data key for this owner")
	ErrUnknownKek  = errors.New("the key provider has no such KEK version")
	ErrWrappedSize = errors.New("a wrapped data key has the wrong size")
)

type Error struct {
	Code    string
	Version int32
	Line    int
	Detail  string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Code)
	if e.Version > 0 {
		fmt.Fprintf(&b, ": KEK version %d", e.Version)
	}
	if e.Line > 0 {
		fmt.Fprintf(&b, ": line %d", e.Line)
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	return b.String()
}

func (e *Error) ExitCode() int {
	if strings.HasPrefix(e.Code, keyringCodePrefix) {
		return ExitKeyring
	}
	return exitOther
}

func newError(code string, version int32, detail string) *Error {
	return &Error{Code: code, Version: version, Detail: detail}
}

func CodeOf(err error) string {
	var ke *Error
	if errors.As(err, &ke) {
		return ke.Code
	}
	var ne *NotEverywhereError
	if errors.As(err, &ne) {
		return CodeKekNotEverywhere
	}
	return ""
}

type NotEverywhereError struct {
	Version   int32
	Instances []string
}

func (e *NotEverywhereError) Error() string {
	return fmt.Sprintf("%s: KEK version %d is not loaded on these running servers: %s; copy the keyring file to them and restart them first",
		CodeKekNotEverywhere, e.Version, strings.Join(e.Instances, ", "))
}
