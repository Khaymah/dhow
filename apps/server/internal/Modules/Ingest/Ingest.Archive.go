package ingest

import (
	"errors"
	"io"

	"github.com/Khaymah/dhow/internal/archive"
)

type archiveCheck struct {
	writer *io.PipeWriter
	done   chan error
}

func startArchiveCheck(limits archive.Limits) *archiveCheck {
	pr, pw := io.Pipe()
	c := &archiveCheck{writer: pw, done: make(chan error, 1)}
	go func() {
		_, err := archive.Walk(pr, limits, func(archive.Entry, io.Reader) error { return nil })
		if err == nil {
			_, err = io.Copy(io.Discard, pr)
		}
		_ = pr.CloseWithError(err)
		c.done <- err
	}()
	return c
}

func (c *archiveCheck) tee(src io.Reader) io.Reader { return io.TeeReader(src, c.writer) }

func (c *archiveCheck) finish() error {
	if err := c.writer.Close(); err != nil {
		return err
	}
	return <-c.done
}

func unsafeArchive(err error) bool {
	return errors.Is(err, archive.ErrUnsafe) || errors.Is(err, archive.ErrTooLarge)
}
