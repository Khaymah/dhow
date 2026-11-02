package gitsource

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/Khaymah/dhow/internal/hostname"
)

const (
	MaxURLBytes      = 2048
	maxUserBytes     = 64
	DefaultSSHPort   = 22
	DefaultHTTPSPort = 443
	maxPort          = 65535
	schemeHTTPS      = "https://"
	schemeSSH        = "ssh://"
)

type Transport string

const (
	TransportHTTPS Transport = "https"
	TransportSSH   Transport = "ssh"
)

var (
	ErrEmpty     = errors.New("the repository address is empty")
	ErrTooLong   = errors.New("the repository address is longer than 2048 bytes")
	ErrScheme    = errors.New("the repository address must start with https:// or ssh://, or be user@host:path")
	ErrCharacter = errors.New("the repository address has a control, space, non-ASCII or special character")
	ErrUser      = errors.New("the repository address has a password, or a user that is not a plain name")
	ErrHost      = errors.New("the repository host must be a DNS name of lowercase ASCII labels, not an IP address")
	ErrPort      = errors.New("the repository port must be a number from 1 to 65535")
	ErrPath      = errors.New("the repository path is empty, starts with a dash, or has .., a query or a fragment")
)

type URL struct {
	Transport Transport
	User      string
	Host      string
	Port      int
	Path      string
}

func Parse(raw string) (URL, error) {
	if raw == "" {
		return URL{}, ErrEmpty
	}
	if len(raw) > MaxURLBytes {
		return URL{}, ErrTooLong
	}
	if err := checkCharacters(raw); err != nil {
		return URL{}, err
	}
	switch {
	case strings.HasPrefix(raw, schemeHTTPS):
		return parseURL(TransportHTTPS, raw[len(schemeHTTPS):])
	case strings.HasPrefix(raw, schemeSSH):
		return parseURL(TransportSSH, raw[len(schemeSSH):])
	case strings.Contains(raw, "://") || strings.Contains(raw, "::") || strings.HasPrefix(raw, "-"):
		return URL{}, ErrScheme
	default:
		return parseSCP(raw)
	}
}

func checkCharacters(raw string) error {
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c <= ' ' || c >= 0x7f || c == '\\' || c == '"' || c == '\'' || c == '`' || c == '<' || c == '>' {
			return ErrCharacter
		}
	}
	return nil
}

func parseURL(transport Transport, rest string) (URL, error) {
	authority, path, ok := strings.Cut(rest, "/")
	if !ok || authority == "" {
		return URL{}, ErrPath
	}
	u := URL{Transport: transport}
	hostPort := authority
	if userInfo, host, hasUser := cutLast(authority, "@"); hasUser {
		if transport == TransportHTTPS {
			return URL{}, ErrUser
		}
		if err := checkUser(userInfo); err != nil {
			return URL{}, err
		}
		u.User, hostPort = userInfo, host
	}
	host, port, err := splitHostPort(hostPort)
	if err != nil {
		return URL{}, err
	}
	u.Host, u.Port = host, port
	if u.Path, err = cleanPath("/" + path); err != nil {
		return URL{}, err
	}
	return u, nil
}

func parseSCP(raw string) (URL, error) {
	userInfo, rest, ok := strings.Cut(raw, "@")
	if !ok {
		return URL{}, ErrScheme
	}
	host, path, ok := strings.Cut(rest, ":")
	if !ok || strings.Contains(host, "/") {
		return URL{}, ErrScheme
	}
	if err := checkUser(userInfo); err != nil {
		return URL{}, err
	}
	name, err := checkHost(host)
	if err != nil {
		return URL{}, err
	}
	clean, err := cleanPath("/" + strings.TrimPrefix(path, "/"))
	if err != nil {
		return URL{}, err
	}
	return URL{Transport: TransportSSH, User: userInfo, Host: name, Path: clean}, nil
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return "", s, false
	}
	return s[:i], s[i+len(sep):], true
}

func checkUser(user string) error {
	if user == "" || len(user) > maxUserBytes || user[0] == '-' || user[0] == '.' || strings.Contains(user, "..") {
		return ErrUser
	}
	for i := 0; i < len(user); i++ {
		c := user[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' && c != '.' {
			return ErrUser
		}
	}
	return nil
}

func splitHostPort(hostPort string) (string, int, error) {
	if strings.HasPrefix(hostPort, "[") {
		return "", 0, ErrHost
	}
	host, portText, hasPort := strings.Cut(hostPort, ":")
	name, err := checkHost(host)
	if err != nil {
		return "", 0, err
	}
	if !hasPort {
		return name, 0, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > maxPort || strconv.Itoa(port) != portText {
		return "", 0, ErrPort
	}
	return name, port, nil
}

func checkHost(host string) (string, error) {
	if host == "" || strings.HasPrefix(host, "-") || strings.HasPrefix(host, "*") {
		return "", ErrHost
	}
	h := strings.ToLower(host)
	if hostname.Check(h) != nil || !strings.Contains(h, ".") {
		return "", ErrHost
	}
	return h, nil
}

func cleanPath(path string) (string, error) {
	if path == "/" || strings.ContainsAny(path, "?#") {
		return "", ErrPath
	}
	decoded, err := decodePercent(path)
	if err != nil {
		return "", err
	}
	if strings.Contains(decoded, "..") {
		return "", ErrPath
	}
	for _, part := range strings.Split(strings.Trim(decoded, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, "-") {
			return "", ErrPath
		}
	}
	for i := 0; i < len(path); i++ {
		c := path[i]
		if !pathByte(c) {
			return "", ErrCharacter
		}
	}
	return path, nil
}

func pathByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.IndexByte("/-._~+@%", c) >= 0
}

func decodePercent(path string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '%' {
			b.WriteByte(path[i])
			continue
		}
		if i+2 >= len(path) {
			return "", ErrCharacter
		}
		v, err := strconv.ParseUint(path[i+1:i+3], 16, 8)
		if err != nil || v <= ' ' || v >= 0x7f || v == '%' || v == '\\' {
			return "", ErrCharacter
		}
		b.WriteByte(byte(v))
		i += 2
	}
	return b.String(), nil
}

func (u URL) EffectivePort() int {
	if u.Port != 0 {
		return u.Port
	}
	if u.Transport == TransportSSH {
		return DefaultSSHPort
	}
	return DefaultHTTPSPort
}

func (u URL) HostPort() string {
	return net.JoinHostPort(u.Host, strconv.Itoa(u.EffectivePort()))
}

func (u URL) String() string {
	var b strings.Builder
	b.WriteString(string(u.Transport))
	b.WriteString("://")
	if u.User != "" {
		b.WriteString(u.User)
		b.WriteByte('@')
	}
	b.WriteString(u.Host)
	if u.Port != 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(u.Port))
	}
	b.WriteString(u.Path)
	return b.String()
}

func (u URL) FullName() string {
	return u.Host + strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
}

func (u URL) KnownHostsPattern() string {
	if u.Transport != TransportSSH {
		return ""
	}
	if u.EffectivePort() == DefaultSSHPort {
		return u.Host
	}
	return "[" + u.Host + "]:" + strconv.Itoa(u.EffectivePort())
}
