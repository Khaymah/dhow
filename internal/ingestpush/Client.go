package ingestpush

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"time"
)

const (
	DefaultRetries  = 3
	MaxRetries      = 10
	backoffStart    = time.Second
	backoffCap      = 30 * time.Second
	MaxRetryAfter   = 2 * time.Minute
	minUpload       = 2 * time.Minute
	maxUpload       = 30 * time.Minute
	slowestRate     = 256 << 10
	dialTimeout     = 30 * time.Second
	handshakeLimit  = 30 * time.Second
	maxCABundle     = 1 << 20
	maxReplyBytes   = 1 << 20
	CodeTLS         = "TLS"
	userAgent       = "dhow-cli"
	acceptJSON      = "application/json"
	acceptLanguage  = "Accept-Language"
	authorizationHd = "Authorization"
)

type Retry struct {
	Attempt int
	Of      int
	Wait    time.Duration
	Code    string
}

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type Options struct {
	Server   string
	Token    string
	Meta     Meta
	CAFile   string
	Retries  int
	Prepare  PrepareOptions
	Lang     string
	Proxy    func(*http.Request) (*url.URL, error)
	Resolver Resolver
	Sleep    func(context.Context, time.Duration) error
	OnRetry  func(Retry)
}

func Push(ctx context.Context, o Options, source string) (Result, error) {
	if err := o.Meta.Validate(); err != nil {
		return Result{}, inputError(err)
	}
	pkg, err := Prepare(source, o.Prepare)
	if err != nil {
		return Result{}, asError(err)
	}
	defer pkg.Close()
	return Upload(ctx, o, pkg)
}

func asError(err error) error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return inputError(err)
}

func Upload(ctx context.Context, o Options, pkg *Package) (Result, error) {
	retries := o.Retries
	if retries < 0 || retries > MaxRetries {
		return Result{}, inputError(ErrRetries)
	}
	if !ValidToken(o.Token) {
		return Result{}, inputError(ErrToken)
	}
	if err := o.Meta.Validate(); err != nil {
		return Result{}, inputError(err)
	}
	target, err := Endpoint(o.Server, pkg.Digest)
	if err != nil {
		return Result{}, inputError(err)
	}
	headers, err := o.Meta.Headers(pkg.Size)
	if err != nil {
		return Result{}, inputError(err)
	}
	client, err := newClient(o, target.Scheme == "http")
	if err != nil {
		return Result{}, inputError(err)
	}
	defer client.CloseIdleConnections()
	return withRetries(ctx, o, retries, func(ctx context.Context) (Result, *Error) {
		return attempt(ctx, client, o, target, headers, pkg)
	})
}

func withRetries(ctx context.Context, o Options, retries int, run func(context.Context) (Result, *Error)) (Result, error) {
	sleep := o.Sleep
	if sleep == nil {
		sleep = pause
	}
	wait := backoffStart
	for n := 1; ; n++ {
		res, fail := run(ctx)
		if fail == nil {
			res.Attempts = n
			return res, nil
		}
		fail.Attempts = n
		if !fail.retryable() || n > retries || ctx.Err() != nil {
			return Result{}, fail
		}
		delay := wait
		if fail.RetryAfter > 0 {
			if fail.RetryAfter > MaxRetryAfter {
				return Result{}, fail
			}
			delay = fail.RetryAfter
		}
		if o.OnRetry != nil {
			o.OnRetry(Retry{Attempt: n, Of: retries + 1, Wait: delay, Code: fail.Code})
		}
		if err := sleep(ctx, delay); err != nil {
			fail.Err = err
			return Result{}, fail
		}
		wait = min(wait*2, backoffCap)
	}
}

func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func uploadTimeout(size int64) time.Duration {
	d := time.Duration(size/slowestRate) * time.Second
	return min(max(d, minUpload), maxUpload)
}

func attempt(ctx context.Context, client *http.Client, o Options, target *url.URL, headers http.Header, pkg *Package) (Result, *Error) {
	f, err := os.Open(pkg.Path)
	if err != nil {
		return Result{}, inputError(fmt.Errorf("%w: %w", ErrSource, err))
	}
	defer func() { _ = f.Close() }()
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout(pkg.Size)+minUpload)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target.String(), io.NopCloser(f))
	if err != nil {
		return Result{}, inputError(ErrServer)
	}
	for k, v := range headers {
		req.Header[k] = v
	}
	req.ContentLength = pkg.Size
	req.Header.Set(authorizationHd, "Bearer "+o.Token)
	req.Header.Set("Accept", acceptJSON)
	req.Header.Set("User-Agent", userAgent)
	if o.Lang != "" {
		req.Header.Set(acceptLanguage, o.Lang)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, transportFailure(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return readReply(resp, pkg.Digest)
}

func transportFailure(ctx context.Context, err error) *Error {
	var verify *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	switch {
	case errors.Is(err, ErrNotLoopback):
		return inputError(ErrNotLoopback)
	case errors.As(err, &verify), errors.As(err, &record):
		return &Error{Class: ClassInput, Code: CodeTLS, Err: err}
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &Error{Class: ClassTemporary, Code: CodeTimeout, Err: err}
	default:
		return &Error{Class: ClassTemporary, Code: CodeNetwork, Err: err}
	}
}

func newClient(o Options, loopbackOnly bool) (*http.Client, error) {
	pool, err := roots(o.CAFile)
	if err != nil {
		return nil, err
	}
	proxy := o.Proxy
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	tr := &http.Transport{
		Proxy:               proxy,
		DialContext:         dialer(o.Resolver, loopbackOnly),
		TLSHandshakeTimeout: handshakeLimit,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool},
		DisableCompression:  true,
		MaxIdleConns:        1,
	}
	return &http.Client{
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

func dialer(resolver Resolver, loopbackOnly bool) func(context.Context, string, string) (net.Conn, error) {
	d := &net.Dialer{Timeout: dialTimeout, KeepAlive: dialTimeout}
	if !loopbackOnly {
		return d.DialContext
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addrs, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, ErrNotLoopback
		}
		for _, a := range addrs {
			if !a.Unmap().IsLoopback() {
				return nil, ErrNotLoopback
			}
		}
		var errs []error
		for _, a := range addrs {
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(a.String(), port))
			if err == nil {
				return conn, nil
			}
			errs = append(errs, err)
		}
		return nil, errors.Join(errs...)
	}
}

func roots(caFile string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if caFile == "" {
		return pool, nil
	}
	f, err := os.Open(caFile) //nolint:gosec // The path is the CA file the user named.
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCAFile, err)
	}
	defer func() { _ = f.Close() }()
	pem, err := io.ReadAll(io.LimitReader(f, maxCABundle+1))
	if err != nil || len(pem) > maxCABundle || !pool.AppendCertsFromPEM(pem) {
		return nil, ErrCAFile
	}
	return pool, nil
}
