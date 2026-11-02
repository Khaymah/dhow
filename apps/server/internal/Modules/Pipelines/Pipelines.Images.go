package pipelines

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
	client := dialer.HTTPClient(netguard.ClientOptions{TLS: r.TLS})
	host := ref.host
	if host == dockerHubIndex {
		host = dockerHubRegistry
	}
	target := url.URL{Scheme: "https", Host: host, Path: "/v2/" + ref.repo + "/manifests/" + ref.tag}
	digest, err := r.manifestDigest(ctx, client, target.String())
	if err != nil {
		return "", err
	}
	return ref.name + "@" + digest, nil
}

func (r Registry) manifestDigest(ctx context.Context, client *http.Client, target string) (string, error) {
	token := ""
	for range 2 {
		res, err := r.request(ctx, client, http.MethodHead, target, token)
		if err != nil {
			return "", err
		}
		_ = res.Body.Close()
		switch {
		case res.StatusCode == http.StatusUnauthorized && token == "":
			if token, err = r.token(ctx, client, res.Header.Get("WWW-Authenticate")); err != nil {
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
		return r.hashManifest(ctx, client, target, token)
	}
	return "", fmt.Errorf("%w: the registry refused the token", ErrRegistry)
}

func (r Registry) hashManifest(ctx context.Context, client *http.Client, target, token string) (string, error) {
	res, err := r.request(ctx, client, http.MethodGet, target, token)
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

func (r Registry) request(ctx context.Context, client *http.Client, method, target, token string) (*http.Response, error) {
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

func (r Registry) token(ctx context.Context, client *http.Client, challenge string) (string, error) {
	scheme, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("%w: the registry needs credentials", ErrRegistry)
	}
	fields := map[string]string{}
	for _, m := range challengeKV.FindAllStringSubmatch(params, 8) {
		fields[m[1]] = m[2]
	}
	realm, err := url.Parse(fields["realm"])
	if err != nil || realm.Scheme != "https" || realm.Host == "" {
		return "", fmt.Errorf("%w: the token address is not https", ErrRegistry)
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
	res, err := client.Do(req)
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
