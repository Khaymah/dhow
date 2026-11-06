//go:build unix

package ingestpush_test

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/ingestpush"
)

func TestSIGTERMCancelsTheContext(t *testing.T) {
	ctx, stop := ingestpush.SignalContext(context.Background())
	defer stop()
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not cancel the context")
	}
}
