package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	DefaultMaxConns      = 256
	DefaultIdle          = 5 * time.Minute
	DefaultDialTimeout   = 10 * time.Second
	DefaultHeaderTimeout = 30 * time.Second
	maxHeaderBytes       = 16 << 10
	acceptBackoff        = 5 * time.Millisecond
	maxAcceptBackoff     = time.Second
)

var (
	ErrNoRules       = errors.New("the job has no egress allowlist, so it gets no proxy")
	ErrExactEndpoint = errors.New("an exact endpoint is not a host and port")
)

const reasonIPNotListed = "an IP address, and the allowlist holds host names only"

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type Decision struct {
	Host    string
	Port    int
	Allowed bool
	Reason  string
}

type Options struct {
	Allow         []string
	Exact         []string
	AllowPrivate  []netip.Prefix
	HostAddrs     []netip.Addr
	Resolver      Resolver
	Dial          func(ctx context.Context, network, address string) (net.Conn, error)
	Report        func(Decision)
	MaxConns      int
	Idle          time.Duration
	DialTimeout   time.Duration
	HeaderTimeout time.Duration
}

type Proxy struct {
	rules  []jobspec.EgressRule
	exact  map[string]struct{}
	ranges ranges
	o      Options
}

func New(o Options) (*Proxy, error) {
	if len(o.Allow) == 0 && len(o.Exact) == 0 {
		return nil, ErrNoRules
	}
	exact, err := parseExact(o.Exact)
	if err != nil {
		return nil, err
	}
	rules := make([]jobspec.EgressRule, 0, len(o.Allow))
	for _, entry := range o.Allow {
		r, err := jobspec.ParseEgress(entry)
		if err != nil {
			return nil, fmt.Errorf("egress entry %q: %w", entry, err)
		}
		rules = append(rules, r)
	}
	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}
	if o.Dial == nil {
		var d net.Dialer
		o.Dial = d.DialContext
	}
	if o.Report == nil {
		o.Report = func(Decision) {}
	}
	o.MaxConns = positive(o.MaxConns, DefaultMaxConns)
	o.Idle = positiveDuration(o.Idle, DefaultIdle)
	o.DialTimeout = positiveDuration(o.DialTimeout, DefaultDialTimeout)
	o.HeaderTimeout = positiveDuration(o.HeaderTimeout, DefaultHeaderTimeout)
	return &Proxy{rules: rules, exact: exact, ranges: ranges{guard: netguard.Policy{Allowed: o.AllowPrivate}, host: o.HostAddrs}, o: o}, nil
}

func (p *Proxy) Serve(ctx context.Context, ln net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	slots := make(chan struct{}, p.o.MaxConns)
	var backoff time.Duration
	var live sync.WaitGroup
	defer live.Wait()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			backoff = min(max(2*backoff, acceptBackoff), maxAcceptBackoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			continue
		}
		backoff = 0
		select {
		case slots <- struct{}{}:
			live.Go(func() {
				defer func() { <-slots }()
				p.handle(ctx, c)
			})
		default:
			refuse(c, statusBusy, "too many connections from this job")
			_ = c.Close()
		}
	}
}

func (p *Proxy) decide(host string, port int, tunnel bool) (string, string) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return p.decideIP(host, addr, port)
	}
	name, err := jobspec.NormalizeHost(host)
	if errors.Is(err, jobspec.ErrEgressIP) {
		return printable(host), reasonIPNotListed
	}
	if err != nil {
		return printable(host), "not a valid host name"
	}
	if p.exactAllows(name, port) {
		return name, ""
	}
	for _, r := range p.rules {
		if r.Allows(name, port, tunnel) {
			return name, ""
		}
	}
	return name, "not in the job's network.egress list"
}

func (p *Proxy) decideIP(host string, addr netip.Addr, port int) (string, string) {
	addr = addr.Unmap()
	if addr.Zone() == "" && p.exactAllows(addr.String(), port) {
		return addr.String(), ""
	}
	if len(p.exact) > 0 {
		return printable(host), "an IP address that is not one of the job's exact endpoints"
	}
	return printable(host), reasonIPNotListed
}

func (p *Proxy) exactAllows(host string, port int) bool {
	_, ok := p.exact[net.JoinHostPort(host, strconv.Itoa(port))]
	return ok
}

func parseExact(entries []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		host, port, ok := jobspec.ParseEndpoint(entry)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrExactEndpoint, printable(entry))
		}
		if addr, err := netip.ParseAddr(host); err == nil {
			host = addr.Unmap().String()
		}
		out[net.JoinHostPort(strings.ToLower(host), strconv.Itoa(port))] = struct{}{}
	}
	return out, nil
}

func (p *Proxy) dial(ctx context.Context, name string, port int) (net.Conn, string, int) {
	addrs, ok := p.resolve(ctx, name)
	if !ok {
		return nil, "the name did not resolve", statusBadGateway
	}
	denial, status := "", statusForbidden
	for _, addr := range addrs {
		if why := p.ranges.denial(addr); why != "" {
			if denial == "" {
				denial = why
			}
			continue
		}
		dctx, dcancel := context.WithTimeout(ctx, p.o.DialTimeout)
		c, err := p.o.Dial(dctx, "tcp", netip.AddrPortFrom(addr.Unmap(), uint16(port)).String()) //nolint:gosec // The port was parsed into 1..65535.
		dcancel()
		if err == nil {
			return c, "", 0
		}
		denial, status = "the connection failed", statusBadGateway
	}
	return nil, denial, status
}

func (p *Proxy) resolve(ctx context.Context, name string) ([]netip.Addr, bool) {
	if addr, err := netip.ParseAddr(name); err == nil {
		return []netip.Addr{addr}, true
	}
	lctx, cancel := context.WithTimeout(ctx, p.o.DialTimeout)
	defer cancel()
	addrs, err := p.o.Resolver.LookupNetIP(lctx, "ip", name+".")
	return addrs, err == nil && len(addrs) > 0
}

func (p *Proxy) report(host string, port int, reason string) {
	p.o.Report(Decision{Host: host, Port: port, Allowed: reason == "", Reason: reason})
}

func positive(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func positiveDuration(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

func printable(host string) string {
	const maxShown = 64
	if len(host) > maxShown {
		host = host[:maxShown]
	}
	return strconv.QuoteToASCII(host)
}
