package streams

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type Writer struct {
	w     http.ResponseWriter
	rc    *http.ResponseController
	wait  time.Duration
	retry time.Duration
	buf   bytes.Buffer
}

func NewWriter(w http.ResponseWriter, wait, retry time.Duration) *Writer {
	return &Writer{w: w, rc: http.NewResponseController(w), wait: wait, retry: retry}
}

func supported(err error) error {
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

func (e *Writer) Start() error {
	if err := supported(e.rc.SetReadDeadline(time.Time{})); err != nil {
		return err
	}
	h := e.w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	e.w.WriteHeader(http.StatusOK)
	e.buf.WriteString("retry: " + strconv.FormatInt(e.retry.Milliseconds(), 10) + "\n\n")
	return e.Flush()
}

func (e *Writer) Event(name, id string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.buf.WriteString("event: " + name + "\n")
	if id != "" {
		e.buf.WriteString("id: " + id + "\n")
	}
	e.buf.WriteString("data: ")
	e.buf.Write(data)
	e.buf.WriteString("\n\n")
	return nil
}

func (e *Writer) Comment() error {
	e.buf.WriteString(": keep-alive\n\n")
	return e.Flush()
}

func (e *Writer) Flush() error {
	if e.buf.Len() == 0 {
		return nil
	}
	if err := supported(e.rc.SetWriteDeadline(time.Now().Add(e.wait))); err != nil {
		return err
	}
	if _, err := e.w.Write(e.buf.Bytes()); err != nil {
		return err
	}
	e.buf.Reset()
	return supported(e.rc.Flush())
}
