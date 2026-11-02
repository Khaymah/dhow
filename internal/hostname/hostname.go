package hostname

import (
	"errors"
	"net/netip"
	"strings"
)

const (
	MaxLength      = 253
	maxLabelLength = 63
)

var (
	ErrInvalid = errors.New("a host must be lowercase ASCII labels (punycode for other scripts)")
	ErrIP      = errors.New("a host must be a name, not an IP address")
)

func Check(host string) error {
	if host == "" || len(host) > MaxLength {
		return ErrInvalid
	}
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return ErrIP
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if !validLabel(label) {
			return ErrInvalid
		}
	}
	last := labels[len(labels)-1]
	if allDigits(last) || strings.HasPrefix(last, "0x") {
		return ErrIP
	}
	return nil
}

func validLabel(label string) bool {
	if label == "" || len(label) > maxLabelLength || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}
