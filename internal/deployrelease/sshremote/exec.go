package sshremote

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

const maxOutput = 64 << 10

type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > maxOutput {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-maxOutput:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.buf...)
}

func (c *Client) Exec(ctx context.Context, argv []string) (deployrelease.ExecResult, error) {
	line, err := deployrelease.Command(argv...)
	if err != nil {
		return deployrelease.ExecResult{}, err
	}
	session, err := c.ssh.NewSession()
	if err != nil {
		return deployrelease.ExecResult{}, err
	}
	defer func() { _ = session.Close() }()
	out := &tailBuffer{}
	session.Stdout, session.Stderr = out, out
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Signal(ssh.SIGKILL)
			_ = session.Close()
		case <-finished:
		}
	}()
	runErr := session.Run(line)
	res := deployrelease.ExecResult{Output: out.bytes()}
	var exit *ssh.ExitError
	switch {
	case ctx.Err() != nil:
		return res, ctx.Err()
	case errors.As(runErr, &exit):
		res.ExitCode = exit.ExitStatus()
		return res, nil
	default:
		return res, runErr
	}
}
