package oidcauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

const (
	RequestTimeout      = 10 * time.Second
	DiscoveryRetryAfter = 30 * time.Second
	MaxResponseBytes    = 1 << 20
	MaxSubjectLength    = 255
)

var (
	ErrUnavailable = errors.New("identity provider is unavailable")
	ErrRejected    = errors.New("identity provider response was rejected")
	ErrUnknown     = errors.New("unknown identity provider")
	errTooLarge    = errors.New("identity provider response is too large")
	errNoRedirects = errors.New("identity provider redirects are not followed")
	errUnsupported = errors.New("unsupported identity provider type")
)

const errStatusPrefix = "identity provider answered"

type Info struct {
	ID   string
	Name string
	Kind string
}

type Email struct {
	Address  string
	Verified bool
	Primary  bool
}

type Identity struct {
	Issuer  string
	Subject string
	Name    string
	Login   string
	Emails  []Email
}

func CleanLogin(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > MaxSubjectLength || strings.ContainsFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return ""
	}
	return v
}

type Flow struct {
	State       string
	Nonce       string
	Verifier    string
	RedirectURL string
}

type Provider interface {
	Info() Info
	Issuer() string
	AllowedDomains() []string
	AuthorizeURL(ctx context.Context, f Flow) (string, error)
	Exchange(ctx context.Context, code string, f Flow) (Identity, error)
}

type Options struct {
	Transport http.RoundTripper
	Now       func() time.Time
	AllowHTTP bool
}

type Registry struct {
	client    *http.Client
	now       func() time.Time
	allowHTTP bool

	mu      sync.Mutex
	current atomic.Pointer[providerSet]
}

type providerSet struct {
	order     []string
	providers map[string]Provider
	configs   map[string]config.OIDCProvider
}

func NewRegistry(cfgs []config.OIDCProvider, o Options) (*Registry, error) {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	r := &Registry{client: newClient(o.Transport), now: now, allowHTTP: o.AllowHTTP}
	if err := r.Replace(cfgs); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) build(c config.OIDCProvider) (Provider, error) {
	switch c.Kind {
	case config.OIDCKindOIDC:
		return newOIDC(c, r.client, r.now, r.allowHTTP), nil
	case config.OIDCKindGitHub:
		return newGitHub(c, r.client), nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupported, c.Kind)
	}
}

func (r *Registry) Replace(cfgs []config.OIDCProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.current.Load()
	next := &providerSet{providers: make(map[string]Provider, len(cfgs)), configs: make(map[string]config.OIDCProvider, len(cfgs))}
	for _, c := range cfgs {
		if old != nil {
			if prev, ok := old.configs[c.ID]; ok && prev.Equal(c) {
				next.add(c, old.providers[c.ID])
				continue
			}
		}
		p, err := r.build(c)
		if err != nil {
			return err
		}
		next.add(c, p)
	}
	r.current.Store(next)
	return nil
}

func (s *providerSet) add(c config.OIDCProvider, p Provider) {
	s.order = append(s.order, c.ID)
	s.providers[c.ID] = p
	s.configs[c.ID] = c
}

func (r *Registry) set() *providerSet {
	if r == nil {
		return nil
	}
	return r.current.Load()
}

func (r *Registry) Get(id string) (Provider, error) {
	s := r.set()
	if s == nil {
		return nil, ErrUnknown
	}
	p, ok := s.providers[id]
	if !ok {
		return nil, ErrUnknown
	}
	return p, nil
}

func (r *Registry) IDs() []string {
	s := r.set()
	if s == nil {
		return []string{}
	}
	return append([]string{}, s.order...)
}

func (r *Registry) List() []Info {
	s := r.set()
	if s == nil {
		return []Info{}
	}
	out := make([]Info, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.providers[id].Info())
	}
	return out
}

func newClient(base http.RoundTripper) *http.Client {
	if base == nil {
		base = guardedTransport()
	}
	return &http.Client{
		Timeout:   RequestTimeout,
		Transport: limitedTransport{base: base},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errNoRedirects
		},
	}
}

type limitedTransport struct {
	base http.RoundTripper
}

func (t limitedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	res.Body = &limitedBody{rc: res.Body, left: MaxResponseBytes}
	return res, nil
}

type limitedBody struct {
	rc   io.ReadCloser
	left int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.left <= 0 {
		var one [1]byte
		if n, _ := b.rc.Read(one[:]); n > 0 {
			return 0, errTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.rc.Read(p)
	b.left -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.rc.Close() }

func rejected(step string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrRejected, step, err)
}

func readJSON(res *http.Response) ([]byte, error) {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %d", errStatusPrefix, res.StatusCode)
	}
	return body, nil
}
