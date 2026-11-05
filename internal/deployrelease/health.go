package deployrelease

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	MaxHealthBudget    = 10 * time.Minute
	maxHealthBody      = 64 << 10
	maxHealthRedirects = 3
	DefaultHealthTime  = 5 * time.Second
	MaxHealthTime      = 30 * time.Second
)

var ErrHealthConfig = errors.New("the health check settings are not valid")

type Health struct {
	URL        string
	StatusMin  int
	StatusMax  int
	Timeout    time.Duration
	Retries    int
	Interval   time.Duration
	Stable     int
	StartDelay time.Duration
}

func (h Health) Enabled() bool { return h.URL != "" }

func (h Health) Budget() time.Duration {
	return h.StartDelay + time.Duration(h.Retries)*(h.Interval+h.Timeout)
}

func (h Health) Validate() error {
	if !h.Enabled() {
		return nil
	}
	if _, err := jobspec.ResolveHealthURL(h.URL, "example.com"); err != nil {
		return fmt.Errorf("%w: %w", ErrHealthConfig, err)
	}
	switch {
	case h.StatusMin < 100 || h.StatusMax > 599 || h.StatusMin > h.StatusMax:
		return fmt.Errorf("%w: status", ErrHealthConfig)
	case h.Timeout <= 0 || h.Timeout > MaxHealthTime:
		return fmt.Errorf("%w: timeout", ErrHealthConfig)
	case h.Retries < 1 || h.Retries > 60:
		return fmt.Errorf("%w: retries", ErrHealthConfig)
	case h.Interval < time.Second || h.Interval > time.Minute:
		return fmt.Errorf("%w: interval", ErrHealthConfig)
	case h.Stable < 1 || h.Stable > h.Retries:
		return fmt.Errorf("%w: stable", ErrHealthConfig)
	case h.StartDelay < 0 || h.StartDelay > 300*time.Second:
		return fmt.Errorf("%w: start delay", ErrHealthConfig)
	case h.Budget() > MaxHealthBudget:
		return fmt.Errorf("%w: the whole check may not take more than 10 minutes", ErrHealthConfig)
	}
	return nil
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type HealthResult struct {
	Attempts   int
	Passed     bool
	LastStatus int
	LastError  string
	Duration   time.Duration
	Stable     int
}

func (r HealthResult) Report() *HealthReport {
	return &HealthReport{
		Attempts: r.Attempts, Passed: r.Passed, LastStatus: r.LastStatus, LastError: r.LastError,
		DurationMs: r.Duration.Milliseconds(), StableChecks: r.Stable,
	}
}

type Clock struct {
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
}

func RealClock() Clock { return Clock{Now: time.Now, Sleep: sleep} }

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func CheckHealth(ctx context.Context, doer Doer, host string, h Health, clock Clock) (HealthResult, error) {
	target, err := jobspec.ResolveHealthURL(h.URL, host)
	if err != nil {
		return HealthResult{}, fmt.Errorf("%w: %w", ErrHealthConfig, err)
	}
	start := clock.Now()
	res := HealthResult{}
	finish := func() HealthResult {
		res.Duration = clock.Now().Sub(start)
		return res
	}
	if err := clock.Sleep(ctx, h.StartDelay); err != nil {
		return finish(), err
	}
	for res.Attempts < h.Retries {
		if res.Attempts > 0 {
			if err := clock.Sleep(ctx, h.Interval); err != nil {
				return finish(), err
			}
		}
		res.Attempts++
		status, kind := probe(ctx, doer, target, h)
		res.LastStatus, res.LastError = status, kind
		if kind == "" {
			res.Stable++
			if res.Stable >= h.Stable {
				res.Passed = true
				return finish(), nil
			}
			continue
		}
		res.Stable = 0
		if ctx.Err() != nil {
			return finish(), ctx.Err()
		}
	}
	return finish(), nil
}

func probe(ctx context.Context, doer Doer, target *url.URL, h Health) (int, string) {
	rctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, ErrKindOther
	}
	resp, err := doer.Do(req)
	if err != nil {
		return 0, classify(rctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxHealthBody))
	if resp.StatusCode < h.StatusMin || resp.StatusCode > h.StatusMax {
		return resp.StatusCode, ErrKindStatus
	}
	return resp.StatusCode, ""
}

func classify(ctx context.Context, err error) string {
	var timeout net.Error
	var unknownAuthority x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostErr x509.HostnameError
	switch {
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil || (errors.As(err, &timeout) && timeout.Timeout()):
		return ErrKindTimeout
	case errors.Is(err, syscall.ECONNREFUSED):
		return ErrKindRefused
	case errors.As(err, &unknownAuthority) || errors.As(err, &invalid) || errors.As(err, &hostErr):
		return ErrKindCertificate
	default:
		return ErrKindOther
	}
}

func NewHealthClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxHealthRedirects {
				return errors.New("too many redirects")
			}
			if req.URL.Host != via[0].URL.Host {
				return errors.New("a redirect to another host")
			}
			return nil
		},
	}
}
