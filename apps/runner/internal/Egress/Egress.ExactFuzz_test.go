package egress

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/Khaymah/dhow/internal/jobspec"
)

func FuzzExactEndpoints(f *testing.F) {
	f.Add("203.0.113.9:22", "203.0.113.9", 22)
	f.Add("web-1.example.com:2222", "WEB-1.example.com.", 2222)
	f.Add("[2606:2800:220:1::1]:22", "2606:2800:0220:0001::1", 22)
	f.Add("10.0.0.1:22", "10.0.0.2", 22)
	f.Fuzz(func(t *testing.T, entry, host string, port int) {
		p, err := New(Options{Exact: []string{entry}})
		if err != nil {
			return
		}
		name, why := p.decide(host, port, true)
		if why != "" {
			return
		}
		listedHost, listedPort, ok := jobspec.ParseEndpoint(entry)
		if !ok {
			t.Fatalf("the proxy started with %q, which is not an endpoint", entry)
		}
		if port != listedPort {
			t.Fatalf("%q was allowed on port %d, not on %d", host, port, listedPort)
		}
		if addr, err := netip.ParseAddr(listedHost); err == nil {
			got, gerr := netip.ParseAddr(name)
			if gerr != nil || got != addr.Unmap() {
				t.Fatalf("%q was allowed for the address %s", host, listedHost)
			}
			return
		}
		if !strings.EqualFold(strings.TrimSuffix(host, "."), listedHost) || net.JoinHostPort(name, strconv.Itoa(port)) != net.JoinHostPort(strings.ToLower(listedHost), strconv.Itoa(listedPort)) {
			t.Fatalf("%q was allowed for %q", host, entry)
		}
	})
}
