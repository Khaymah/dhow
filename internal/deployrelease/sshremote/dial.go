package sshremote

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

var ErrProxy = errors.New("the egress proxy refused the connection")

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }

func ProxyDialer(proxy string, timeout time.Duration) (DialFunc, error) {
	u, err := url.Parse(proxy)
	if err != nil || u.Scheme != "http" || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("%w: the proxy address must be http://host:port", ErrProxy)
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			return nil, fmt.Errorf("%w: only tcp", ErrProxy)
		}
		d := net.Dialer{Timeout: timeout}
		conn, err := d.DialContext(ctx, "tcp", u.Host)
		if err != nil {
			return nil, err
		}
		br, err := connectThrough(ctx, conn, address)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return bufferedConn{Conn: conn, r: br}, nil
	}, nil
}

func connectThrough(ctx context.Context, conn net.Conn, address string) (*bufio.Reader, error) {
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: http.Header{}}
	if err := req.Write(conn); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrProxy, resp.StatusCode)
	}
	return br, conn.SetDeadline(time.Time{})
}
