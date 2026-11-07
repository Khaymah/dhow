package notifications

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

var (
	ErrURLInvalid  = errors.New("the address is not a valid webhook address")
	ErrURLScheme   = errors.New("the address must start with https://")
	ErrURLUserinfo = errors.New("the address must not carry a user name or a password")
	ErrURLHost     = errors.New("the address names no usable host")
	ErrURLNumeric  = errors.New("the host is written as a number, not as a name or a plain IP address")
	ErrURLTooLong  = errors.New("the address is too long")
)

type Target struct {
	URL      *url.URL
	Hostname string
	Display  string
	Literal  netip.Addr
}

func ParseTarget(raw string, allowHTTP bool) (Target, error) {
	if len(raw) == 0 {
		return Target{}, ErrURLInvalid
	}
	if len(raw) > config.NotifyURLMax {
		return Target{}, ErrURLTooLong
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r > unicode.MaxASCII {
			return Target{}, ErrURLInvalid
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, ErrURLInvalid
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return Target{}, ErrURLScheme
	}
	if u.User != nil || strings.Contains(u.Host, "@") {
		return Target{}, ErrURLUserinfo
	}
	if u.Opaque != "" || u.Fragment != "" || u.RawFragment != "" {
		return Target{}, ErrURLInvalid
	}
	host := u.Hostname()
	if host == "" || strings.ContainsAny(host, "%\\") || strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return Target{}, ErrURLHost
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return Target{}, ErrURLHost
		}
	}
	target := Target{URL: u, Hostname: strings.TrimSuffix(strings.ToLower(host), "."), Display: displayOf(u)}
	if addr, err := netip.ParseAddr(target.Hostname); err == nil {
		target.Literal = addr.Unmap()
		return target, nil
	}
	if numericHost(target.Hostname) {
		return Target{}, ErrURLNumeric
	}
	return target, nil
}

func displayOf(u *url.URL) string {
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" && !((u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80")) {
		return host + ":" + port
	}
	return host
}

func numericHost(host string) bool {
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	last := labels[len(labels)-1]
	if last == "" {
		return true
	}
	if strings.HasPrefix(last, "0x") || strings.HasPrefix(last, "0X") {
		return true
	}
	for _, r := range last {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
