package sshremote

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	DefaultConnectTimeout = 15 * time.Second
	keepAliveEvery        = 15 * time.Second
	keepAliveLimit        = 15 * time.Second
	keepAliveRequest      = "keepalive@openssh.com"
)

var (
	ErrKnownHosts = errors.New("the pinned host key line is not valid for this host")
	ErrPrivateKey = errors.New("the private key could not be read")
)

type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

type Options struct {
	Host           string
	Port           int
	User           string
	PrivateKey     []byte
	KnownHostsLine string
	Dial           DialFunc
	ConnectTimeout time.Duration
	KeepAlive      time.Duration
	KeepAliveLimit time.Duration
}

func PinnedKey(host string, port int, line string) (ssh.PublicKey, error) {
	fields := strings.Split(line, " ")
	if len(fields) != 3 || fields[0] != jobspec.KnownHostsPattern(host, port) {
		return nil, ErrKnownHosts
	}
	key, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(fields[1] + " " + fields[2]))
	if err != nil || len(rest) != 0 || key.Type() != fields[1] {
		return nil, ErrKnownHosts
	}
	return key, nil
}

func hostKeyAlgorithms(key ssh.PublicKey) []string {
	if key.Type() == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{key.Type()}
}

type Client struct {
	ssh  *ssh.Client
	sftp *sftp.Client
	done chan struct{}
	once sync.Once
}

func Connect(ctx context.Context, o Options) (*Client, error) {
	pinned, err := PinnedKey(o.Host, o.Port, o.KnownHostsLine)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(o.PrivateKey)
	if err != nil {
		return nil, ErrPrivateKey
	}
	timeout := o.ConnectTimeout
	if timeout <= 0 {
		timeout = DefaultConnectTimeout
	}
	cfg := &ssh.ClientConfig{
		User:              o.User,
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback:   pinnedCallback(pinned),
		HostKeyAlgorithms: hostKeyAlgorithms(pinned),
		Timeout:           timeout,
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := o.Dial(dctx, "tcp", jobspec.Endpoint(o.Host, o.Port))
	if err != nil {
		return nil, fmt.Errorf("connect to the server: %w", err)
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	sc, chans, reqs, err := ssh.NewClientConn(conn, jobspec.Endpoint(o.Host, o.Port), cfg)
	if err != nil {
		_ = conn.Close()
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, fmt.Errorf("log in to the server: %w", errors.Join(deployrelease.ErrAuthFailed, err))
		}
		return nil, fmt.Errorf("log in to the server: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = sc.Close()
		return nil, err
	}
	client := ssh.NewClient(sc, chans, reqs)
	files, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("start the file transfer: %w", err)
	}
	c := &Client{ssh: client, sftp: files, done: make(chan struct{})}
	go c.keepAlive(cmp.Or(o.KeepAlive, keepAliveEvery), cmp.Or(o.KeepAliveLimit, keepAliveLimit))
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-c.done:
		}
	}()
	return c, nil
}

func pinnedCallback(pinned ssh.PublicKey) ssh.HostKeyCallback {
	want := pinned.Marshal()
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if bytes.Equal(key.Marshal(), want) {
			return nil
		}
		return fmt.Errorf("%w: it offered %s", deployrelease.ErrHostKeyMismatch, ssh.FingerprintSHA256(key))
	}
}

func (c *Client) keepAlive(every, limit time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			answered := make(chan error, 1)
			go func() {
				_, _, err := c.ssh.SendRequest(keepAliveRequest, true, nil)
				answered <- err
			}()
			select {
			case err := <-answered:
				if err != nil {
					_ = c.Close()
					return
				}
			case <-time.After(limit):
				_ = c.Close()
				return
			case <-c.done:
				return
			}
		}
	}
}

func (c *Client) Close() error {
	var err error
	c.once.Do(func() {
		close(c.done)
		err = c.ssh.Close()
		_ = c.sftp.Close()
	})
	return err
}
