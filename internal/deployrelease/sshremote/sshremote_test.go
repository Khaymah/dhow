package sshremote_test

import (
	"context"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/deployrelease/sshremote"
	"github.com/Khaymah/dhow/internal/jobspec"
)

func options(s *testServer, key []byte) sshremote.Options {
	return sshremote.Options{
		Host: "127.0.0.1", Port: s.port(), User: "deploy", PrivateKey: key, KnownHostsLine: s.knownHostsLine(),
		Dial: direct, ConnectTimeout: 5 * time.Second,
	}
}

func connect(t *testing.T, s *testServer, key []byte) *sshremote.Client {
	t.Helper()
	c, err := sshremote.Connect(context.Background(), options(s, key))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestItConnectsOnlyToTheServerWhoseKeyIsPinned(t *testing.T) {
	s, key := startServer(t)
	connect(t, s, key)

	other, _ := newKey(t)
	o := options(s, key)
	o.KnownHostsLine = jobspec.KnownHostsLine("127.0.0.1", s.port(), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(other.PublicKey()))))
	_, err := sshremote.Connect(context.Background(), o)
	require.Error(t, err)
	assert.ErrorIs(t, err, deployrelease.ErrHostKeyMismatch, "a different host key is refused and named as such")
}

func TestAKeyOfAnotherTypeThanThePinnedOneIsAMismatchToo(t *testing.T) {
	s, key := startServer(t)
	o := options(s, key)
	o.KnownHostsLine = strings.Replace(o.KnownHostsLine, "ssh-ed25519", "ecdsa-sha2-nistp256", 1)
	_, err := sshremote.Connect(context.Background(), o)
	require.Error(t, err)
}

func TestAPinnedLineForAnotherHostOrPortIsRefusedBeforeDialing(t *testing.T) {
	s, key := startServer(t)
	dialed := false
	for _, line := range []string{
		"other.example.com " + strings.SplitN(s.knownHostsLine(), " ", 2)[1],
		"[127.0.0.1]:1 " + strings.SplitN(s.knownHostsLine(), " ", 2)[1],
		strings.SplitN(s.knownHostsLine(), " ", 2)[0] + " ssh-ed25519 !!!",
		"",
	} {
		o := options(s, key)
		o.KnownHostsLine = line
		o.Dial = func(ctx context.Context, n, a string) (net.Conn, error) { dialed = true; return direct(ctx, n, a) }
		_, err := sshremote.Connect(context.Background(), o)
		assert.ErrorIs(t, err, sshremote.ErrKnownHosts, line)
	}
	assert.False(t, dialed)
}

func TestAnUnknownLoginKeyIsRefusedAndAnUnreadableOneIsReportedWithoutItsContent(t *testing.T) {
	s, _ := startServer(t)
	_, stranger := newKey(t)
	_, err := sshremote.Connect(context.Background(), options(s, stranger))
	require.Error(t, err)

	_, err = sshremote.Connect(context.Background(), options(s, []byte("not a key")))
	assert.ErrorIs(t, err, sshremote.ErrPrivateKey)
	assert.NotContains(t, err.Error(), "not a key")
}

func TestExecSendsOneQuotedLineAndReturnsTheExitCodeAndOutput(t *testing.T) {
	s, key := startServer(t)
	s.onExec = func(cmd string) (string, uint32) {
		if strings.Contains(cmd, "fail") {
			return "it broke", 7
		}
		return "fine", 0
	}
	c := connect(t, s, key)

	res, err := c.Exec(context.Background(), []string{"echo", "a b", "it's"})
	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
	assert.Equal(t, "fine", string(res.Output))
	assert.Equal(t, []string{`echo 'a b' 'it'\''s'`}, s.execs)

	res, err = c.Exec(context.Background(), []string{"echo", "fail"})
	require.NoError(t, err)
	assert.Equal(t, 7, res.ExitCode)

	_, err = c.Exec(context.Background(), []string{"-rf"})
	assert.ErrorIs(t, err, deployrelease.ErrCommand)
	assert.Len(t, s.execs, 2, "a command that cannot be quoted is never sent")
}

func TestTheFileOperationsWorkOverTheSftpSubsystem(t *testing.T) {
	s, key := startServer(t)
	c := connect(t, s, key)
	root := t.TempDir()
	dir := filepath.Join(root, "app")

	require.NoError(t, c.MkdirAll(filepath.Join(dir, "releases", "a"), 0o755))
	err := c.Mkdir(filepath.Join(dir, "releases"), 0o700)
	assert.ErrorIs(t, err, fs.ErrExist, "an existing directory is told apart from other failures")

	w, err := c.Create(filepath.Join(dir, "releases", "a", "index.html"), 0o644)
	require.NoError(t, err)
	_, err = io.WriteString(w, "hello")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	_, err = c.Create(filepath.Join(dir, "releases", "a", "index.html"), 0o644)
	assert.Error(t, err, "an existing file is never overwritten")

	require.NoError(t, c.Symlink("releases/a", filepath.Join(dir, "current.tmp")))
	require.NoError(t, c.Rename(filepath.Join(dir, "current.tmp"), filepath.Join(dir, "current")))
	target, err := c.Readlink(filepath.Join(dir, "current"))
	require.NoError(t, err)
	assert.Equal(t, "releases/a", target)

	require.NoError(t, c.MkdirAll(filepath.Join(dir, "releases", "b"), 0o755))
	require.NoError(t, c.Symlink("releases/b", filepath.Join(dir, "current.tmp")))
	require.NoError(t, c.Rename(filepath.Join(dir, "current.tmp"), filepath.Join(dir, "current")))
	target, err = c.Readlink(filepath.Join(dir, "current"))
	require.NoError(t, err)
	assert.Equal(t, "releases/b", target, "the rename replaces the symlink in one step")

	info, err := c.Lstat(filepath.Join(dir, "current"))
	require.NoError(t, err)
	assert.True(t, info.IsSymlink())
	list, err := c.ReadDir(filepath.Join(dir, "releases"))
	require.NoError(t, err)
	assert.Len(t, list, 2)
	raw, err := c.ReadFile(filepath.Join(dir, "releases", "a", "index.html"), 3)
	require.NoError(t, err)
	assert.Equal(t, "hel", string(raw), "the read stops at the limit")
	free, err := c.FreeBytes(dir)
	require.NoError(t, err)
	assert.NotZero(t, free)

	require.NoError(t, c.RemoveAll(filepath.Join(dir, "releases")))
	require.NoError(t, c.RemoveAll(filepath.Join(dir, "nothing")), "removing what is not there is fine")
	_, err = os.Lstat(filepath.Join(dir, "releases"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
	_, err = c.Lstat(filepath.Join(dir, "releases"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestTheProxyDialerSendsConnectAndKeepsTheBytesThatFollowTheAnswer(t *testing.T) {
	s, key := startServer(t)
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = proxy.Close() })
	seen := make(chan string, 1)
	go func() {
		conn, err := proxy.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 512)
		n, _ := conn.Read(buf)
		seen <- string(buf[:n])
		upstream, err := net.Dial("tcp", s.ln.Addr().String())
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() { _, _ = io.Copy(upstream, conn) }()
		_, _ = io.Copy(conn, upstream)
	}()
	dial, err := sshremote.ProxyDialer("http://"+proxy.Addr().String(), time.Second)
	require.NoError(t, err)
	o := options(s, key)
	o.Dial = dial
	c, err := sshremote.Connect(context.Background(), o)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.True(t, strings.HasPrefix(<-seen, "CONNECT 127.0.0.1:"+strings.Split(s.ln.Addr().String(), ":")[1]+" HTTP/1.1\r\n"))
}

func TestTheProxyDialerRefusesAnythingButAnHttpProxyAndANon200Answer(t *testing.T) {
	for _, bad := range []string{"", "socks5://127.0.0.1:1", "http://user:pw@127.0.0.1:1", "127.0.0.1:3128"} {
		_, err := sshremote.ProxyDialer(bad, time.Second)
		assert.ErrorIs(t, err, sshremote.ErrProxy, bad)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 512)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
		_ = conn.Close()
	}()
	dial, err := sshremote.ProxyDialer("http://"+ln.Addr().String(), time.Second)
	require.NoError(t, err)
	_, err = dial(context.Background(), "tcp", "example.com:22")
	assert.ErrorIs(t, err, sshremote.ErrProxy)
}

type freezeConn struct {
	net.Conn
	frozen atomic.Bool
	closed chan struct{}
	once   sync.Once
}

func (f *freezeConn) Read(p []byte) (int, error) {
	if f.frozen.Load() {
		<-f.closed
		return 0, io.EOF
	}
	return f.Conn.Read(p)
}

func (f *freezeConn) Write(p []byte) (int, error) {
	if f.frozen.Load() {
		return len(p), nil
	}
	return f.Conn.Write(p)
}

func (f *freezeConn) Close() error {
	f.once.Do(func() { close(f.closed) })
	return f.Conn.Close()
}

func TestAServerThatStopsAnsweringIsDroppedByTheKeepAliveAndCallsReturn(t *testing.T) {
	s, key := startServer(t)
	var fc *freezeConn
	o := options(s, key)
	o.KeepAlive, o.KeepAliveLimit = 50*time.Millisecond, 100*time.Millisecond
	o.Dial = func(ctx context.Context, n, a string) (net.Conn, error) {
		c, err := direct(ctx, n, a)
		fc = &freezeConn{Conn: c, closed: make(chan struct{})}
		return fc, err
	}
	c, err := sshremote.Connect(context.Background(), o)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	fc.frozen.Store(true)
	errc := make(chan error, 1)
	go func() { _, err := c.Lstat(t.TempDir()); errc <- err }()
	select {
	case err := <-errc:
		assert.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a call on a server that stopped answering never returned")
	}
}

func TestCancelingTheContextClosesTheConnection(t *testing.T) {
	s, key := startServer(t)
	var fc *freezeConn
	o := options(s, key)
	o.Dial = func(ctx context.Context, n, a string) (net.Conn, error) {
		c, err := direct(ctx, n, a)
		fc = &freezeConn{Conn: c, closed: make(chan struct{})}
		return fc, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c, err := sshremote.Connect(ctx, o)
	require.NoError(t, err)
	fc.frozen.Store(true)
	errc := make(chan error, 1)
	go func() { _, err := c.Lstat(t.TempDir()); errc <- err }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		assert.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a call still hangs after the context was canceled")
	}
}
