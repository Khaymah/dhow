package sshhostkey_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/netguard"
	"github.com/Khaymah/dhow/internal/sshhostkey"
)

func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

func startServer(t *testing.T, signer ssh.Signer) string {
	t.Helper()
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_, _, _, _ = ssh.NewServerConn(conn, cfg)
			}()
		}
	}()
	return ln.Addr().String()
}

func allowLoopback() netguard.Dialer {
	return netguard.Dialer{Policy: netguard.Policy{Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}}
}

func TestFetchReturnsTheKeyTheServerPresents(t *testing.T) {
	signer := newSigner(t)
	addr := startServer(t, signer)
	key, err := sshhostkey.Fetch(context.Background(), allowLoopback(), addr, "deploy", 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, ssh.FingerprintSHA256(signer.PublicKey()), ssh.FingerprintSHA256(key))
	assert.Equal(t, ssh.FingerprintSHA256(signer.PublicKey()), sshhostkey.Fingerprint(sshhostkey.AuthorizedLine(key)))
}

func TestFetchRefusesABlockedAddressBeforeConnecting(t *testing.T) {
	addr := startServer(t, newSigner(t))
	_, err := sshhostkey.Fetch(context.Background(), netguard.Dialer{}, addr, "deploy", 5*time.Second)
	require.ErrorIs(t, err, netguard.ErrBlocked)
	assert.NotErrorIs(t, err, sshhostkey.ErrUnreachable)
}

func TestFetchReportsAServerThatDoesNotSpeakSSH(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			_ = conn.Close()
		}
	}()
	_, err = sshhostkey.Fetch(context.Background(), allowLoopback(), ln.Addr().String(), "deploy", 5*time.Second)
	require.ErrorIs(t, err, sshhostkey.ErrUnreachable)
}

func TestParseReadsKeyscanAndPublicKeyFiles(t *testing.T) {
	signer := newSigner(t)
	line := sshhostkey.AuthorizedLine(signer.PublicKey())
	want := ssh.FingerprintSHA256(signer.PublicKey())
	for name, text := range map[string]string{
		"a public key file":     line + " root@host\n",
		"a keyscan line":        "# host:22 SSH-2.0\nexample.com " + line + "\n",
		"a hashed known host":   "|1|abc=|def= " + line + "\n",
		"windows line endings":  "example.com " + line + "\r\n",
		"a bare key":            line,
		"an address and a port": "[203.0.113.4]:2222 " + line,
	} {
		t.Run(name, func(t *testing.T) {
			key, err := sshhostkey.Parse(text)
			require.NoError(t, err)
			assert.Equal(t, want, ssh.FingerprintSHA256(key))
		})
	}
}

func TestParsePrefersEd25519WhenSeveralKeysArePasted(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecPub, err := ssh.NewPublicKey(&ec.PublicKey)
	require.NoError(t, err)
	edSigner := newSigner(t)
	text := "h " + sshhostkey.AuthorizedLine(ecPub) + "\nh " + sshhostkey.AuthorizedLine(edSigner.PublicKey()) + "\n"
	key, err := sshhostkey.Parse(text)
	require.NoError(t, err)
	assert.Equal(t, ssh.KeyAlgoED25519, key.Type())
}

func TestParseRefusesWhatIsNotAPlainHostKey(t *testing.T) {
	line := sshhostkey.AuthorizedLine(newSigner(t).PublicKey())
	for name, text := range map[string]string{
		"nothing":                 "",
		"only comments":           "# nothing here\n",
		"a certificate authority": "@cert-authority * " + line,
		"garbage":                 "not a key at all",
		"a truncated key":         "ssh-ed25519 AAAA",
		"too much text":           strings.Repeat("a", sshhostkey.MaxPasteBytes+1),
		"too many lines":          strings.Repeat("# x\n", 40),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sshhostkey.Parse(text)
			require.Error(t, err)
		})
	}
}

func TestFingerprintOfNothingIsEmpty(t *testing.T) {
	assert.Empty(t, sshhostkey.Fingerprint(""))
	assert.Empty(t, sshhostkey.Fingerprint("not a key"))
}
