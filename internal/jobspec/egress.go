package jobspec

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Khaymah/dhow/internal/hostname"
)

const (
	DefaultTLSPort  = 443
	DefaultHTTPPort = 80
	wildcardPrefix  = "*."
)

var (
	ErrEgressEmpty    = errors.New("an egress entry is empty")
	ErrEgressTooLong  = errors.New("an egress entry is too long")
	ErrEgressPort     = errors.New("an egress port must be a number from 1 to 65535 without leading zeros")
	ErrEgressIP       = errors.New("an egress entry must name a host, not an IP address")
	ErrEgressHostname = errors.New("an egress host must be lowercase ASCII labels (punycode for other scripts)")
	ErrEgressSingle   = errors.New("an egress host needs at least two labels, so a bare name or a whole top-level domain is refused")
)

type EgressRule struct {
	Host     string
	Wildcard bool
	Port     int
}

func ParseEgress(entry string) (EgressRule, error) {
	if entry == "" {
		return EgressRule{}, ErrEgressEmpty
	}
	if len(entry) > MaxEgressEntryLength {
		return EgressRule{}, ErrEgressTooLong
	}
	host, portText, hasPort := strings.Cut(entry, ":")
	rule := EgressRule{}
	if hasPort {
		port, ok := parsePort(portText)
		if !ok {
			return EgressRule{}, ErrEgressPort
		}
		rule.Port = port
	}
	if strings.HasPrefix(host, wildcardPrefix) {
		rule.Wildcard = true
		host = strings.TrimPrefix(host, wildcardPrefix)
	}
	if err := checkHostname(host); err != nil {
		return EgressRule{}, err
	}
	if !strings.Contains(host, ".") {
		return EgressRule{}, ErrEgressSingle
	}
	rule.Host = host
	return rule, nil
}

func (r EgressRule) Allows(host string, port int, tunnel bool) bool {
	want := r.Port
	if want == 0 {
		want = DefaultHTTPPort
		if tunnel {
			want = DefaultTLSPort
		}
	}
	if port != want {
		return false
	}
	if r.Wildcard {
		return strings.HasSuffix(host, "."+r.Host)
	}
	return host == r.Host
}

func NormalizeHost(host string) (string, error) {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	if err := checkHostname(h); err != nil {
		return "", err
	}
	return h, nil
}

func checkHostname(host string) error {
	switch err := hostname.Check(host); {
	case errors.Is(err, hostname.ErrIP):
		return ErrEgressIP
	case err != nil:
		return ErrEgressHostname
	default:
		return nil
	}
}

func parsePort(text string) (int, bool) {
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > maxPort || strconv.Itoa(n) != text {
		return 0, false
	}
	return n, true
}
