package agent

import (
	"bytes"
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"

	agentv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/agent/v1"
)

const (
	MaxReportFileBytes = 64 << 10
	reportPoll         = 200 * time.Millisecond
)

func clearReport(path string) bool {
	err := os.Remove(path)
	return err == nil || errors.Is(err, os.ErrNotExist)
}

type reportTail struct {
	path   string
	offset int64
}

func (r *reportTail) next(final bool) []byte {
	fd, err := unix.Open(r.path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	f := os.NewFile(uintptr(fd), r.path)
	defer func() { _ = f.Close() }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil
	}
	limit := min(st.Size, MaxReportFileBytes)
	if limit <= r.offset {
		return nil
	}
	buf := make([]byte, limit-r.offset)
	n, err := f.ReadAt(buf, r.offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil
	}
	buf = buf[:n]
	end := bytes.LastIndexByte(buf, '\n') + 1
	if end == 0 && final && len(buf) > 0 && st.Size < MaxReportFileBytes {
		r.offset += int64(len(buf))
		return append(buf, '\n')
	}
	r.offset += int64(end)
	return buf[:end]
}

func (a *Agent) sendReport(index int32, data []byte) {
	if len(data) == 0 {
		return
	}
	_ = a.conn.Write(&agentv1.Frame{Body: &agentv1.Frame_Report{Report: &agentv1.Report{StepIndex: index, Data: data}}})
}

func (a *Agent) tailReport(index int32) func() {
	tail := &reportTail{path: a.reportPath}
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(reportPoll)
		defer t.Stop()
		for {
			select {
			case <-quit:
				a.sendReport(index, tail.next(true))
				return
			case <-t.C:
				a.sendReport(index, tail.next(false))
			}
		}
	}()
	return func() {
		close(quit)
		<-done
		_ = os.Remove(a.reportPath)
	}
}
