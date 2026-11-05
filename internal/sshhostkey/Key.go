package sshhostkey

import (
	"errors"
	"slices"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	MaxPasteBytes = 8 << 10
	maxPasteLines = 16
)

var (
	ErrNoKey       = errors.New("no public key was found in the text")
	ErrTooLarge    = errors.New("the pasted text is too large")
	ErrKeyType     = errors.New("the key type is not one of ssh-ed25519, ecdsa-sha2-nistp256, ecdsa-sha2-nistp384, ecdsa-sha2-nistp521 or ssh-rsa")
	preferredTypes = []string{ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA521, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA256, ssh.KeyAlgoRSA}
)

func AuthorizedLine(key ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

func Fingerprint(authorized string) string {
	if authorized == "" {
		return ""
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorized))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}

func Parse(pasted string) (ssh.PublicKey, error) {
	if len(pasted) > MaxPasteBytes {
		return nil, ErrTooLarge
	}
	lines := strings.Split(strings.ReplaceAll(pasted, "\r", ""), "\n")
	if len(lines) > maxPasteLines {
		return nil, ErrTooLarge
	}
	var found []ssh.PublicKey
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, err := parseLine(line)
		if err != nil {
			return nil, err
		}
		found = append(found, key)
	}
	if len(found) == 0 {
		return nil, ErrNoKey
	}
	return best(found), nil
}

func parseLine(line string) (ssh.PublicKey, error) {
	fields := strings.Fields(line)
	if len(fields) > 0 && !isKeyType(fields[0]) && !strings.HasPrefix(fields[0], "@") {
		fields = fields[1:]
	}
	if len(fields) < 2 || !isKeyType(fields[0]) {
		return nil, ErrKeyType
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.Join(fields[:2], " ")))
	if err != nil {
		return nil, ErrNoKey
	}
	if !slices.Contains(preferredTypes, key.Type()) {
		return nil, ErrKeyType
	}
	return key, nil
}

func isKeyType(v string) bool { return slices.Contains(preferredTypes, v) }

func best(keys []ssh.PublicKey) ssh.PublicKey {
	for _, t := range preferredTypes {
		for _, k := range keys {
			if k.Type() == t {
				return k
			}
		}
	}
	return keys[0]
}
