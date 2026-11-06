package ingest

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func extendDeadline(set func(time.Time) error, idle time.Duration) error {
	if err := set(time.Now().Add(idle)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("extend the upload deadline: %w", err)
	}
	return nil
}

type idleReader struct {
	r    io.Reader
	rc   *http.ResponseController
	idle time.Duration
}

func (i idleReader) Read(p []byte) (int, error) {
	if err := extendDeadline(i.rc.SetReadDeadline, i.idle); err != nil {
		return 0, err
	}
	return i.r.Read(p)
}
