//go:build chaos

package chaos_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type hookHit struct {
	Delivery  string
	Timestamp string
	Signature string
	Body      []byte
}

type hook struct {
	srv  *http.Server
	url  string
	mu   sync.Mutex
	hits []hookHit
	hold bool
	seen chan struct{}
}

func lanAddress() (net.IP, bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, false
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := ipnet.IP.To4(); ip != nil && ip.IsPrivate() {
			return ip, true
		}
	}
	return nil, false
}

func newNotifyPKI(dir string, ip net.IP) (string, tls.Certificate, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", tls.Certificate{}, err
	}
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "dhow chaos notify CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return "", tls.Certificate{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", tls.Certificate{}, err
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(12), Subject: pkix.Name{CommonName: "notify receiver"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IPAddresses: []net.IP{ip}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		return "", tls.Certificate{}, err
	}
	caPath := filepath.Join(dir, "notify-ca.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		return "", tls.Certificate{}, err
	}
	return caPath, tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: key}, nil
}

func newHook(ip net.IP, cert tls.Certificate) (*hook, func(), error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		return nil, nil, err
	}
	h := &hook{url: "https://" + ln.Addr().String() + "/chaos-hook", seen: make(chan struct{}, 16)}
	h.srv = &http.Server{Handler: http.HandlerFunc(h.serve), ReadHeaderTimeout: 5 * time.Second}
	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	go func() { _ = h.srv.Serve(tlsLn) }()
	return h, func() { _ = h.srv.Close() }, nil
}

func (h *hook) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	h.hits = append(h.hits, hookHit{
		Delivery: r.Header.Get("Dhow-Delivery"), Timestamp: r.Header.Get("Dhow-Timestamp"), Signature: r.Header.Get("Dhow-Signature"), Body: body,
	})
	hold := h.hold
	h.mu.Unlock()
	select {
	case h.seen <- struct{}{}:
	default:
	}
	if hold {
		<-r.Context().Done()
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *hook) holdAnswers(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hold = on
}

func (h *hook) received() []hookHit {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]hookHit(nil), h.hits...)
}

func (h *hook) waitHits(t *testing.T, n int, within time.Duration) []hookHit {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if got := h.received(); len(got) >= n {
			return got
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("the receiver got %d requests within %s, wanted %d", len(h.received()), within, n)
	return nil
}

func sign(secret string, hit hookHit) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(hit.Timestamp + "." + string(hit.Body)))
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

type deliveryRow struct {
	ID        string `json:"Id"`
	State     string `json:"State"`
	Attempts  int    `json:"Attempts"`
	LastError string `json:"LastError"`
}

func TestKillingTheServerAfterTheCommitDeliversTheNotificationAfterTheRestart(t *testing.T) {
	if shared.hook == nil {
		t.Skip("this machine has no private IPv4 address, and Dhow refuses to send to loopback")
	}
	a := shared.api
	var created struct {
		Channel struct {
			ID string `json:"Id"`
		} `json:"Channel"`
		Secret string `json:"Secret"`
	}
	_, err := a.call(http.MethodPost, "/organizations/"+a.org+"/notification-channels", map[string]any{
		"Kind": "webhook", "Name": "chaos-" + randomHex(4), "Url": shared.hook.url,
		"CurrentFactor": map[string]string{"Method": "password", "Password": ownerPassword},
	}, &created)
	require.NoError(t, err)
	require.NotEmpty(t, created.Secret)

	shared.hook.holdAnswers(true)
	_, err = a.call(http.MethodPost, "/organizations/"+a.org+"/notification-channels/"+created.Channel.ID+"/test", nil, nil)
	require.NoError(t, err)
	first := shared.hook.waitHits(t, 1, time.Minute)[0]
	assert.True(t, hmac.Equal([]byte(sign(created.Secret, first)), []byte(first.Signature)), "the first attempt is signed")

	shared.stack.killServer()
	shared.hook.holdAnswers(false)
	_, err = shared.stack.startServer()
	require.NoError(t, err)

	hits := shared.hook.waitHits(t, 2, 4*time.Minute)
	again := hits[len(hits)-1]
	assert.Equal(t, first.Delivery, again.Delivery, "the same delivery comes again, so the receiver can drop a repeat")
	assert.True(t, hmac.Equal([]byte(sign(created.Secret, again)), []byte(again.Signature)), "and it is signed again")
	_, err = strconv.ParseInt(again.Timestamp, 10, 64)
	require.NoError(t, err)

	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		var list []deliveryRow
		if _, err := a.call(http.MethodGet, "/organizations/"+a.org+"/notification-deliveries", nil, &list); err == nil {
			for _, d := range list {
				if d.ID == first.Delivery && d.State == "sent" {
					assert.GreaterOrEqual(t, d.Attempts, 2)
					return
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("the delivery never reached the state sent after the restart")
}
