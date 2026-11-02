package agent

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const MaxOutputFileBytes = 64 << 10

func clearOutput(path string) {
	_ = os.Remove(path)
}

func readOutput(path string) ([]byte, bool) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, true
	}
	if err != nil {
		return nil, false
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() {
		_ = f.Close()
		_ = os.Remove(path)
	}()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Size > MaxOutputFileBytes {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxOutputFileBytes+1))
	if err != nil || len(data) > MaxOutputFileBytes {
		return nil, false
	}
	return data, true
}
