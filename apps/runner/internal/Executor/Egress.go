package executor

import (
	"context"
	"fmt"
	"strings"

	egress "github.com/Khaymah/dhow/apps/runner/internal/Egress"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

const (
	ProxyURL = "http://127.0.0.1:3128"
	NoProxy  = "localhost,127.0.0.1,::1"
)

func proxyEnv() map[string]string {
	env := map[string]string{}
	for _, name := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"} {
		env[name] = ProxyURL
		env[strings.ToLower(name)] = ProxyURL
	}
	env["NO_PROXY"] = NoProxy
	env["no_proxy"] = NoProxy
	return env
}

func (a *attempt) startEgress(ctx context.Context, box sandbox.Sandbox, allow []string) error {
	eb, ok := box.(sandbox.Egress)
	if len(allow) == 0 || !ok || eb.EgressListener() == nil {
		return nil
	}
	hosts, err := egress.HostAddrs()
	if err != nil {
		return fmt.Errorf("list the runner's own addresses: %w", err)
	}
	reporter := egress.NewReporter(nil, func(line string) { a.pipe.Systemf(nil, "%s", line) })
	p, err := egress.New(egress.Options{
		Allow: allow, AllowPrivate: a.e.o.Config.EgressAllowPrivate, HostAddrs: hosts, Report: reporter.Report,
	})
	if err != nil {
		return err
	}
	ln := eb.EgressListener()
	go func() {
		if err := p.Serve(ctx, ln); err != nil {
			a.log.Warn("the egress proxy stopped", "error", a.masked(err))
		}
	}()
	return nil
}
