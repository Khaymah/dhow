package agent

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const MaxOutputFileBytes = 64 << 10

func clearOutput(path string) bool {
	err := os.Remove(path)
	return err == nil || errors.Is(err, os.ErrNotExist)
}

func readOutput(path string) (data []byte, ok bool) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, true
	}
	if err != nil {
		return nil, false
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() {
		if err := errors.Join(f.Close(), os.Remove(path)); err != nil {
			data, ok = nil, false
		}
	}()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Size > MaxOutputFileBytes {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxOutputFileBytes+1))
	if err != nil || len(raw) > MaxOutputFileBytes {
		return nil, false
	}
	return raw, true
}
