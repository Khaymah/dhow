package keys

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
)

const (
	agePublicPrefix  = "age1"
	ageSecretPrefix  = "AGE-SECRET-KEY-" //nolint:gosec // The prefix that marks an age private key, so it is refused.
	maxRecipientSize = 4096
)

var (
	ErrRecipientSecret  = errors.New("this is an age private key; pass the public key (age1...) and keep the private key off this server")
	ErrRecipientInvalid = errors.New("the recipient must be one age public key (age1...)")
)

func ParseRecipient(s string) (age.Recipient, error) {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s)), ageSecretPrefix) {
		return nil, ErrRecipientSecret
	}
	if len(s) > maxRecipientSize || !strings.HasPrefix(s, agePublicPrefix) || strings.ContainsAny(s, " \t\r\n#") {
		return nil, ErrRecipientInvalid
	}
	recipients, err := age.ParseRecipients(strings.NewReader(s))
	if err != nil || len(recipients) != 1 {
		return nil, ErrRecipientInvalid
	}
	return recipients[0], nil
}

func Export(dst io.Writer, ring *Keyring, recipient age.Recipient) error {
	w, err := age.Encrypt(dst, recipient)
	if err != nil {
		return fmt.Errorf("encrypt the keyring: %w", err)
	}
	data := ring.Marshal()
	defer clear(data)
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("encrypt the keyring: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("encrypt the keyring: %w", err)
	}
	return nil
}
