package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"connectrpc.com/connect"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

const (
	dialTimeout      = 10 * time.Second
	tlsTimeout       = 10 * time.Second
	keepAlive        = 30 * time.Second
	idleConnTimeout  = 90 * time.Second
	maxIdleConns     = 8
	responseMaxBytes = 1 << 20
)

var (
	ErrRedirect  = errors.New("the server answered with a redirect; the runner never follows redirects, so check DHOW_RUNNER_URL")
	ErrPlainHTTP = errors.New("plain http:// needs --dev and a loopback address")
	ErrNoCA      = errors.New("the CA file has no PEM certificate")
)

type Options struct {
	BaseURL  *url.URL
	CAFile   string
	Dev      bool
	Version  string
	Identity *identity.Identity
	Log      *slog.Logger
	Now      func() time.Time
	Features []runnerv1.RunnerFeature
}

type Client struct {
	http         *http.Client
	registration runnerv1connect.RegistrationServiceClient
	auth         runnerv1connect.AuthServiceClient
	lease        runnerv1connect.LeaseServiceClient
	logs         runnerv1connect.LogServiceClient
	tokens       *tokens
	log          *slog.Logger
}

func New(o Options) (*Client, error) {
	if o.BaseURL == nil {
		return nil, fmt.Errorf("%s: %w", config.EnvURL, config.ErrNeedsURL)
	}
	if _, err := config.ParseServerURL(o.BaseURL.String(), o.Dev); err != nil {
		return nil, errors.Join(ErrPlainHTTP, err)
	}
	transport, err := newTransport(o.CAFile)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrRedirect
		},
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	base := o.BaseURL.String() + config.APIPrefix
	opts := []connect.ClientOption{
		connect.WithInterceptors(headers(o.Version)),
		connect.WithReadMaxBytes(responseMaxBytes),
	}
	c := &Client{
		http:         hc,
		registration: runnerv1connect.NewRegistrationServiceClient(hc, base, opts...),
		auth:         runnerv1connect.NewAuthServiceClient(hc, base, opts...),
		lease:        runnerv1connect.NewLeaseServiceClient(hc, base, opts...),
		logs:         runnerv1connect.NewLogServiceClient(hc, base, opts...),
		log:          o.Log,
	}
	if o.Identity != nil {
		c.tokens = &tokens{auth: c.auth, id: o.Identity, now: o.Now, features: o.Features}
	}
	return c, nil
}

func newTransport(caFile string) (*http.Transport, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile) //nolint:gosec // The operator's CA bundle.
		if err != nil {
			return nil, fmt.Errorf("read the CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, ErrNoCA
		}
		tlsConfig.RootCAs = pool
	}
	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: keepAlive}
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         dialer.DialContext,
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: tlsTimeout,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        maxIdleConns,
		IdleConnTimeout:     idleConnTimeout,
	}, nil
}

func headers(version string) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set(config.ProtocolHeader, config.ProtocolVersion)
			req.Header().Set(config.VersionHeader, version)
			return next(ctx, req)
		}
	})
}

func (c *Client) CloseIdle() { c.http.CloseIdleConnections() }
