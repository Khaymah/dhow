package jobspec

import (
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Khaymah/dhow/internal/netguard"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	EnvTargetName        = "DHOW_TARGET_NAME"
	EnvTargetKind        = "DHOW_TARGET_KIND"
	EnvTargetUser        = "DHOW_TARGET_USER"
	EnvTargetDeployPath  = "DHOW_TARGET_DEPLOY_PATH"
	EnvTargetReleaseID   = "DHOW_TARGET_RELEASE_ID"
	EnvTargetKeepRelease = "DHOW_TARGET_KEEP_RELEASES"
	EnvTargetHosts       = "DHOW_TARGET_HOSTS"
	EnvTargetSettings    = "DHOW_TARGET_SETTINGS"
	EnvTargetKeyFile     = "DHOW_TARGET_KEY_FILE"

	HealthHostPlaceholder = "{host}"
	MaxHealthURLLength    = 2048
)

var ErrHealthURL = errors.New("the health check address must be an http or https address without a user name, and {host} may only stand for the whole host name")

type targetHostEnv struct {
	HostID         string `json:"HostId"`
	Host           string `json:"Host"`
	Port           int    `json:"Port"`
	KnownHostsLine string `json:"KnownHostsLine"`
}

type targetSettingsEnv struct {
	BatchSize         int      `json:"BatchSize"`
	BatchPauseSeconds int      `json:"BatchPauseSeconds"`
	ReloadCommand     string   `json:"ReloadCommand"`
	PreSwitchCommand  string   `json:"PreSwitchCommand"`
	PostSwitchCommand string   `json:"PostSwitchCommand"`
	Shared            []string `json:"Shared"`
	ComposeProject    string   `json:"ComposeProject"`
	ComposeFile       string   `json:"ComposeFile"`
	AllowUnsafe       bool     `json:"AllowUnsafeCompose"`
}

func TargetEnv(t *runnerv1.StepTarget, index int, secretsDir string) (map[string]string, error) {
	hosts := make([]targetHostEnv, 0, len(t.GetHosts()))
	for _, h := range t.GetHosts() {
		hosts = append(hosts, targetHostEnv{HostID: h.GetHostId(), Host: h.GetHost(), Port: int(h.GetPort()), KnownHostsLine: h.GetKnownHostsLine()})
	}
	rawHosts, err := json.Marshal(hosts)
	if err != nil {
		return nil, err
	}
	s := t.GetSettings()
	shared := s.GetShared()
	if shared == nil {
		shared = []string{}
	}
	rawSettings, err := json.Marshal(targetSettingsEnv{
		BatchSize: int(s.GetBatchSize()), BatchPauseSeconds: int(s.GetBatchPauseSeconds()), ReloadCommand: s.GetReloadCommand(),
		PreSwitchCommand: s.GetPreSwitchCommand(), PostSwitchCommand: s.GetPostSwitchCommand(), Shared: shared,
		ComposeProject: s.GetComposeProject(), ComposeFile: s.GetComposeFile(), AllowUnsafe: s.GetAllowUnsafeCompose(),
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{
		EnvTargetName: t.GetName(), EnvTargetKind: t.GetKind(), EnvTargetUser: t.GetUser(), EnvTargetDeployPath: t.GetDeployPath(),
		EnvTargetReleaseID: t.GetReleaseId(), EnvTargetKeepRelease: strconv.Itoa(int(t.GetKeepReleases())),
		EnvTargetHosts: string(rawHosts), EnvTargetSettings: string(rawSettings),
		EnvTargetKeyFile: strings.TrimSuffix(secretsDir, "/") + "/" + TargetKeyFile(index),
	}, nil
}

func hostLiteral(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func HealthEndpoints(raw string, hosts []string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > MaxHealthURLLength || strings.ContainsFunc(raw, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return nil, ErrHealthURL
	}
	if !strings.Contains(raw, HealthHostPlaceholder) {
		hosts = []string{""}
	}
	var out []string
	for _, h := range hosts {
		endpoint, err := healthEndpoint(raw, h)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, endpoint) {
			out = append(out, endpoint)
		}
	}
	return out, nil
}

func healthEndpoint(raw, host string) (string, error) {
	if strings.Contains(raw, HealthHostPlaceholder) {
		if !placeholderIsHost(raw) {
			return "", ErrHealthURL
		}
		raw = strings.ReplaceAll(raw, HealthHostPlaceholder, hostLiteral(host))
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrHealthURL
	}
	name := strings.ToLower(u.Hostname())
	if !validTargetHost(name) {
		return "", ErrHealthURL
	}
	port := DefaultHTTPPort
	if u.Scheme == "https" {
		port = DefaultTLSPort
	}
	if text := u.Port(); text != "" {
		p, ok := parsePort(text)
		if !ok {
			return "", ErrHealthURL
		}
		port = p
	}
	return Endpoint(name, port), nil
}

func placeholderIsHost(raw string) bool {
	_, rest, found := strings.Cut(raw, "://")
	if !found {
		return false
	}
	authority, _, _ := strings.Cut(rest, "/")
	authority, _, _ = strings.Cut(authority, "?")
	authority, _, _ = strings.Cut(authority, "#")
	if strings.Count(raw, HealthHostPlaceholder) != strings.Count(authority, HealthHostPlaceholder) {
		return false
	}
	name, port, hasPort := strings.Cut(authority, ":")
	if name != HealthHostPlaceholder {
		return false
	}
	return !hasPort || !strings.Contains(port, HealthHostPlaceholder)
}

var internalSuffixes = []string{".localhost", ".local", ".internal", ".localdomain", ".home.arpa"}

func HealthHostAllowed(host string, targetHosts []string, extra bool) bool {
	if slices.Contains(targetHosts, host) {
		return true
	}
	if !extra {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return netguard.Policy{}.Denial(addr) == ""
	}
	if _, err := ParseEgress(host); err != nil {
		return false
	}
	return !slices.ContainsFunc(internalSuffixes, func(s string) bool { return strings.HasSuffix(host, s) })
}
