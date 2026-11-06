package app

import (
	"context"

	"github.com/Khaymah/dhow/internal/ingestpush"
)

func SignalContext() (context.Context, context.CancelFunc) {
	return ingestpush.SignalContext(context.Background())
}
