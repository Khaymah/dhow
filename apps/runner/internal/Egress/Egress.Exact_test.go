package egress

import (
	"net/http"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPublicIPEndpointIsTunnelledToThatAddressOnly(t *testing.T) {
	f := start(t, Options{Exact: []string{public + ":22"}}, nil)
	_, _, status := connect(t, f.proxy, public+":22")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, public+":22", <-f.dialed)
	assert.Equal(t, Decision{Host: public, Port: 22, Allowed: true}, <-f.report)
	for _, target := range []string{public + ":2222", public + ":443", "93.184.216.35:22"} {
		_, _, status = connect(t, f.proxy, target)
		assert.Equal(t, http.StatusForbidden, status, target)
		assert.Equal(t, "an IP address that is not one of the job's exact endpoints", (<-f.report).Reason, target)
	}
	select {
	case a := <-f.dialed:
		t.Fatalf("a refused target was dialed: %s", a)
	default:
	}
}

func TestAPrivateIPEndpointIsRefusedUnlessTheOperatorOpenedItsRange(t *testing.T) {
	closed := start(t, Options{Exact: []string{"10.20.0.5:22", "127.0.0.1:22", "169.254.169.254:80"}}, nil)
	for _, target := range []string{"10.20.0.5:22", "127.0.0.1:22", "169.254.169.254:80"} {
		_, _, status := connect(t, closed.proxy, target)
		assert.Equal(t, http.StatusForbidden, status, target)
		assert.False(t, (<-closed.report).Allowed, target)
	}
	select {
	case a := <-closed.dialed:
		t.Fatalf("a private target was dialed: %s", a)
	default:
	}
	open := start(t, Options{
		Exact:        []string{"10.20.0.5:22", "169.254.169.254:80"},
		AllowPrivate: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16"), netip.MustParsePrefix("169.254.0.0/16")},
	}, nil)
	_, _, status := connect(t, open.proxy, "10.20.0.5:22")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "10.20.0.5:22", <-open.dialed)
	<-open.report
	_, _, status = connect(t, open.proxy, "169.254.169.254:80")
	assert.Equal(t, http.StatusForbidden, status, "metadata stays closed even inside an operator range")
	assert.Equal(t, "a cloud metadata address", (<-open.report).Reason)
}

func TestAnIPv6EndpointMatchesInEveryWrittenForm(t *testing.T) {
	f := start(t, Options{Exact: []string{"[2606:2800:220:1::1]:22"}}, nil)
	_, _, status := connect(t, f.proxy, "[2606:2800:0220:0001:0000:0000:0000:0001]:22")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "[2606:2800:220:1::1]:22", <-f.dialed)
	<-f.report
	_, _, status = connect(t, f.proxy, "[2606:2800:220:1::2]:22")
	assert.Equal(t, http.StatusForbidden, status)
}

func TestAnExactHostNameIsHeldToItsPortAndResolvedThroughTheGuard(t *testing.T) {
	answers := map[string][][]netip.Addr{
		"web1.example.com": {addrs(public)},
		"evil.example.com": {addrs("127.0.0.1")},
	}
	f := start(t, Options{Exact: []string{"web1.example.com:2222", "evil.example.com:22"}}, answers)
	_, _, status := connect(t, f.proxy, "Web1.Example.com.:2222")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, public+":2222", <-f.dialed)
	<-f.report
	for _, target := range []string{"web1.example.com:22", "web1.example.com:443", "other.example.com:2222", "sub.web1.example.com:2222"} {
		_, _, status = connect(t, f.proxy, target)
		assert.Equal(t, http.StatusForbidden, status, target)
		assert.Equal(t, "not in the job's network.egress list", (<-f.report).Reason, target)
	}
	_, _, status = connect(t, f.proxy, "evil.example.com:22")
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "a private, loopback or reserved address", (<-f.report).Reason)
}

func TestExactEndpointsAddToTheListWithoutWideningIt(t *testing.T) {
	answers := map[string][][]netip.Addr{"registry.npmjs.org": {addrs(public)}}
	f := start(t, Options{Allow: []string{"registry.npmjs.org"}, Exact: []string{"198.51.100.7:22"}}, answers)
	_, _, status := connect(t, f.proxy, "registry.npmjs.org:443")
	assert.Equal(t, http.StatusOK, status)
	<-f.dialed
	<-f.report
	_, _, status = connect(t, f.proxy, "198.51.100.7:443")
	assert.Equal(t, http.StatusForbidden, status)
	assert.False(t, (<-f.report).Allowed)
}

func TestAnInvalidExactEndpointStopsTheProxyFromStarting(t *testing.T) {
	for _, bad := range []string{"", "web1.example.com", "web1.example.com:0", "-bad:22", "10.0.0.1:99999", "web1.example.com:22/x"} {
		_, err := New(Options{Exact: []string{bad}})
		require.ErrorIs(t, err, ErrExactEndpoint, bad)
	}
	_, err := New(Options{})
	require.ErrorIs(t, err, ErrNoRules)
}
