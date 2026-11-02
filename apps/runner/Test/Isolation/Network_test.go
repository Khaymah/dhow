//go:build isolation

package isolation_test

import (
	"context"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	egress "github.com/Khaymah/dhow/apps/runner/internal/Egress"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type testResolver struct {
	mu      sync.Mutex
	answers map[string][]string
}

func (r *testResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	host = strings.TrimSuffix(host, ".")
	queue := r.answers[host]
	if len(queue) == 0 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	if len(queue) > 1 {
		r.answers[host] = queue[1:]
	}
	return []netip.Addr{netip.MustParseAddr(queue[0])}, nil
}

func echoOnLoopback(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port
}

func startWithEgress(t *testing.T, allow []string, answers map[string][]string) (sandbox.Sandbox, func() []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	id := token()
	dir := filepath.Join(config.Config{StateDir: shared.stateDir}.AttemptsDir(), id)
	require.NoError(t, config.EnsurePrivateDir(dir))
	box, err := shared.driver.Prepare(ctx, sandbox.PrepareSpec{
		RunnerID: runnerID, AttemptID: id, Image: image, Isolation: runnerv1.Isolation_ISOLATION_CONTAINER,
		Resources: defaultResources(), Dir: dir, DiskMode: config.DiskModeTmpfs, Egress: allow,
		Workspace: workspaceFor(ctx, t, id, dir, defaultResources()),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = box.Destroy(context.Background()) })
	eb, ok := box.(sandbox.Egress)
	require.True(t, ok)
	require.NotNil(t, eb.EgressListener())
	var mu sync.Mutex
	lines := []string{}
	p, err := egress.New(egress.Options{
		Allow:        allow,
		AllowPrivate: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
		Resolver:     &testResolver{answers: answers},
		Report: func(d egress.Decision) {
			mu.Lock()
			lines = append(lines, egress.Line(d))
			mu.Unlock()
		},
	})
	require.NoError(t, err)
	sctx, scancel := context.WithCancel(context.Background())
	t.Cleanup(scancel)
	go func() { _ = p.Serve(sctx, eb.EgressListener()) }()
	require.NoError(t, box.Start(ctx))
	return box, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), lines...)
	}
}

func TestAJobWithoutEgressHasNoNetworkAtAll(t *testing.T) {
	box, _ := start(t, defaultResources())
	out := run(t, box, probe+" netnone\n", nil)
	assert.Empty(t, escapeLines(out))
	assert.Zero(t, out.result.ExitCode, "no TCP, no UDP, no DNS and no proxy without network.egress")
}

func TestTheEgressProxyAllowsOnlyTheListedHostAndPort(t *testing.T) {
	port := echoOnLoopback(t)
	answers := map[string][]string{
		"allowed.dhow.test": {"127.0.0.1", "127.0.0.1", "127.0.0.1"},
		"private.dhow.test": {"10.0.0.7"},
		"meta.dhow.test":    {"169.254.169.254"},
		"rebind.dhow.test":  {"127.0.0.1", "10.0.0.8"},
	}
	allow := []string{"allowed.dhow.test:" + port, "private.dhow.test:" + port, "meta.dhow.test:" + port, "rebind.dhow.test:" + port}
	box, _ := startWithEgress(t, allow, answers)
	tests := []struct {
		target string
		ok     bool
		status string
	}{
		{"allowed.dhow.test:" + port, true, "status 200"},
		{"allowed.dhow.test:1", false, "status 403"},
		{"other.dhow.test:" + port, false, "status 403"},
		{"127.0.0.1:" + port, false, "status 403"},
		{"private.dhow.test:" + port, false, "status 403"},
		{"meta.dhow.test:" + port, false, "status 403"},
		{"rebind.dhow.test:" + port, true, "status 200"},
		{"rebind.dhow.test:" + port, false, "status 403"},
	}
	for _, tt := range tests {
		out := run(t, box, probe+" tunnel "+tt.target+"\n", nil)
		assert.Contains(t, out.stdout, tt.status, tt.target)
		if tt.ok {
			assert.Zero(t, out.result.ExitCode, tt.target)
			assert.Contains(t, out.stdout, "echo ping", tt.target)
		} else {
			assert.NotZero(t, out.result.ExitCode, tt.target)
		}
	}
}

func TestTheConnectHelperTunnelsLikeSSHProxyCommand(t *testing.T) {
	port := echoOnLoopback(t)
	box, lines := startWithEgress(t, []string{"git.dhow.test:" + port}, map[string][]string{"git.dhow.test": {"127.0.0.1"}})
	out := run(t, box, "echo SSH-2.0-dhow | /run/dhow/bin/connect git.dhow.test "+port+"\n", nil)
	assert.Zero(t, out.result.ExitCode)
	assert.Equal(t, "SSH-2.0-dhow", strings.TrimSpace(out.stdout))
	denied := run(t, box, "/run/dhow/bin/connect evil.dhow.test 22 </dev/null\n", nil)
	assert.NotZero(t, denied.result.ExitCode)
	assert.Contains(t, denied.stderr, "403")
	assert.Contains(t, strings.Join(lines(), "\n"), "dhow: egress denied evil.dhow.test:22")
}
