package sshremote_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/jobspec"
)

type testServer struct {
	t        *testing.T
	ln       net.Listener
	hostKey  ssh.Signer
	clientPK ssh.PublicKey
	mu       sync.Mutex
	execs    []string
	onExec   func(cmd string) (string, uint32)
}

func newKey(t *testing.T) (ssh.Signer, []byte) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer, pem.EncodeToMemory(block)
}

func startServer(t *testing.T) (*testServer, []byte) {
	t.Helper()
	host, _ := newKey(t)
	client, clientPEM := newKey(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &testServer{t: t, ln: ln, hostKey: host, clientPK: client.PublicKey()}
	t.Cleanup(func() { _ = ln.Close() })
	go s.accept()
	return s, clientPEM
}

func (s *testServer) port() int {
	_, p, _ := net.SplitHostPort(s.ln.Addr().String())
	n, _ := strconv.Atoi(p)
	return n
}

func (s *testServer) knownHostsLine() string {
	return jobspec.KnownHostsLine("127.0.0.1", s.port(), string(trimNewline(ssh.MarshalAuthorizedKey(s.hostKey.PublicKey()))))
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b
}

func (s *testServer) accept() {
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if string(key.Marshal()) == string(s.clientPK.Marshal()) {
			return nil, nil
		}
		return nil, errors.New("unknown key")
	}}
	cfg.AddHostKey(s.hostKey)
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.serve(conn, cfg)
	}
}

func (s *testServer) serve(conn net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go s.session(ch, creqs)
	}
	_ = sc.Close()
}

func (s *testServer) session(ch ssh.Channel, reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "subsystem":
			_ = r.Reply(true, nil)
			srv, err := sftp.NewServer(ch)
			if err == nil {
				_ = srv.Serve()
			}
			_ = ch.Close()
			return
		case "exec":
			var p struct{ Command string }
			_ = ssh.Unmarshal(r.Payload, &p)
			_ = r.Reply(true, nil)
			s.mu.Lock()
			s.execs = append(s.execs, p.Command)
			s.mu.Unlock()
			out, code := "", uint32(0)
			if s.onExec != nil {
				out, code = s.onExec(p.Command)
			}
			_, _ = ch.Write([]byte(out))
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{code}))
			_ = ch.Close()
			return
		default:
			_ = r.Reply(false, nil)
		}
	}
}

func direct(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}
