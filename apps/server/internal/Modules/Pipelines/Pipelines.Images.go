package pipelines

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	registryTimeout       = 15 * time.Second
	registryManifestBytes = 4 << 20
	registryTokenBytes    = 64 << 10
	dockerHubIndex        = "docker.io"
	dockerHubRegistry     = "registry-1.docker.io"
	defaultTag            = "latest"
)

var (
	ErrImageNotFound = errors.New("the registry has no image with this tag")
	ErrRegistry      = errors.New("the registry did not answer as expected")

	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	challengeKV   = regexp.MustCompile(`([a-z]+)="([^"]*)"`)
)

func manifestTypes() []string {
	return []string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}
}

type Registry struct {
	Dialer  netguard.Dialer
	TLS     *tls.Config
	Timeout time.Duration
}

type imageRef struct {
	host string
	repo string
	tag  string
	name string
}

func splitImage(image string) (imageRef, error) {
	ref := jobspec.QualifyImage(strings.TrimSpace(image))
	if jobspec.ImageReason(ref) != jobspec.ReasonNotPinned {
		return imageRef{}, fmt.Errorf("%w: not an image by tag", ErrRegistry)
	}
	host, rest, found := strings.Cut(ref, "/")
	if !found {
		return imageRef{}, fmt.Errorf("%w: no registry host", ErrRegistry)
	}
	repo, tag := rest, defaultTag
	if i := strings.LastIndex(rest, ":"); i > strings.LastIndex(rest, "/") {
		repo, tag = rest[:i], rest[i+1:]
	}
	return imageRef{host: host, repo: repo, tag: tag, name: host + "/" + repo + ":" + tag}, nil
}

func (r Registry) Resolve(ctx context.Context, image string) (string, error) {
	ref, err := splitImage(image)
	if err != nil {
		return "", err
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = registryTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dialer := r.Dialer
	dialer.Timeout = timeout
	host := ref.host
	if host == dockerHubIndex {
		host = dockerHubRegistry
	}
	public := dialer
	public.Policy = netguard.Policy{}
	s := session{
		client: dialer.HTTPClient(netguard.ClientOptions{TLS: r.TLS}), realms: public.HTTPClient(netguard.ClientOptions{TLS: r.TLS}),
		registry: host,
	}
	target := url.URL{Scheme: "https", Host: host, Path: "/v2/" + ref.repo + "/manifests/" + ref.tag}
	digest, err := s.manifestDigest(ctx, target.String())
	if err != nil {
		return "", err
	}
	return ref.name + "@" + digest, nil
}

func (s session) manifestDigest(ctx context.Context, target string) (string, error) {
	token := ""
	for range 2 {
		res, err := request(ctx, s.client, http.MethodHead, target, token)
		if err != nil {
			return "", err
		}
		_ = res.Body.Close()
		switch {
		case res.StatusCode == http.StatusUnauthorized && token == "":
			if token, err = s.token(ctx, res.Header.Get("WWW-Authenticate")); err != nil {
				return "", err
			}
			continue
		case res.StatusCode == http.StatusNotFound:
			return "", ErrImageNotFound
		case res.StatusCode != http.StatusOK:
			return "", fmt.Errorf("%w: status %d", ErrRegistry, res.StatusCode)
		}
		if d := res.Header.Get("Docker-Content-Digest"); digestPattern.MatchString(d) {
			return d, nil
		}
		return s.hashManifest(ctx, target, token)
	}
	return "", fmt.Errorf("%w: the registry refused the token", ErrRegistry)
}

func (s session) hashManifest(ctx context.Context, target, token string) (string, error) {
	res, err := request(ctx, s.client, http.MethodGet, target, token)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", ErrRegistry, res.StatusCode)
	}
	body, err := netguard.ReadLimited(res.Body, registryManifestBytes)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrRegistry, err)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func request(ctx context.Context, client *http.Client, method, target, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", strings.Join(manifestTypes(), ", "))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRegistry, err)
	}
	return res, nil
}

type session struct {
	client   *http.Client
	realms   *http.Client
	registry string
}

func realmHosts() []string { return []string{"auth.docker.io", "ghcr.io"} }

func (s session) realmAllowed(realm *url.URL) bool {
	if realm.Scheme != "https" || realm.User != nil || realm.Hostname() == "" {
		return false
	}
	registryHost, registryPort := splitHostPort(s.registry)
	if realm.Hostname() == registryHost {
		return realm.Port() == "" || realm.Port() == "443" || realm.Port() == registryPort
	}
	return slices.Contains(realmHosts(), realm.Hostname()) && (realm.Port() == "" || realm.Port() == "443")
}

func splitHostPort(hostport string) (string, string) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport, ""
	}
	return host, port
}

func (s session) token(ctx context.Context, challenge string) (string, error) {
	scheme, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("%w: the registry needs credentials", ErrRegistry)
	}
	fields := map[string]string{}
	for _, m := range challengeKV.FindAllStringSubmatch(params, 8) {
		fields[m[1]] = m[2]
	}
	realm, err := url.Parse(fields["realm"])
	if err != nil || !s.realmAllowed(realm) {
		return "", fmt.Errorf("%w: the token address is not https on the registry host or a known token service", ErrRegistry)
	}
	q := realm.Query()
	for _, k := range []string{"service", "scope"} {
		if v := fields[k]; v != "" {
			q.Set(k, v)
		}
	}
	realm.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), http.NoBody)
	if err != nil {
		return "", err
	}
	res, err := s.realms.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrRegistry, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: the token request answered %d", ErrRegistry, res.StatusCode)
	}
	body, err := netguard.ReadLimited(res.Body, registryTokenBytes)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrRegistry, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("%w: the token answer is not JSON", ErrRegistry)
	}
	for _, key := range []string{"token", "access_token"} {
		var v string
		if json.Unmarshal(raw[key], &v) == nil && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("%w: the token answer has no token", ErrRegistry)
}
