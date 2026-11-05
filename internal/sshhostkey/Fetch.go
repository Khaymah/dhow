package sshhostkey

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/netguard"
)

var (
	ErrUnreachable = errors.New("the server did not answer the SSH handshake")
	errCaptured    = errors.New("the host key was captured")
)

func Algorithms() []string {
	return []string{
		ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256,
	}
}

func Fetch(ctx context.Context, dialer netguard.Dialer, hostPort, user string, timeout time.Duration) (ssh.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if errors.Is(err, netguard.ErrBlocked) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer func() { _ = conn.Close() }()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	var captured ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User:              user,
		HostKeyAlgorithms: Algorithms(),
		Timeout:           timeout,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			captured = key
			return errCaptured
		},
	}
	_, _, _, err = ssh.NewClientConn(conn, hostPort, cfg)
	if captured != nil {
		return captured, nil
	}
	return nil, fmt.Errorf("%w: %w", ErrUnreachable, err)
}
