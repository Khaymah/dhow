package stack_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

type execLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *execLog) add(host string, argv []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, host+" "+strings.Join(argv, " "))
}

func (l *execLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func (l *execLog) forHost(host string) []string {
	var out []string
	for _, line := range l.all() {
		if rest, ok := strings.CutPrefix(line, host+" "); ok {
			out = append(out, rest)
		}
	}
	return out
}

type fakeHost struct {
	t      *testing.T
	name   string
	root   string
	log    *execLog
	docker func(argv []string) deployrelease.ExecResult
}

func newFakeHost(t *testing.T, name string, log *execLog) *fakeHost {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "srv", "app"), 0o755))
	return &fakeHost{t: t, name: name, root: root, log: log}
}

func (f *fakeHost) path(name string) string { return filepath.Join(f.root, filepath.FromSlash(name)) }

func (f *fakeHost) Lstat(name string) (deployrelease.FileInfo, error) {
	info, err := os.Lstat(f.path(name))
	if err != nil {
		return deployrelease.FileInfo{}, err
	}
	return deployrelease.FileInfo{Name: info.Name(), Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime()}, nil
}

func (f *fakeHost) ReadDir(name string) ([]deployrelease.FileInfo, error) {
	list, err := os.ReadDir(f.path(name))
	if err != nil {
		return nil, err
	}
	out := make([]deployrelease.FileInfo, 0, len(list))
	for _, e := range list {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, deployrelease.FileInfo{Name: e.Name(), Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime()})
	}
	return out, nil
}

func (f *fakeHost) Readlink(name string) (string, error) {
	return os.Readlink(f.path(name))
}

func (f *fakeHost) Mkdir(name string, mode fs.FileMode) error {
	return os.Mkdir(f.path(name), mode)
}

func (f *fakeHost) MkdirAll(name string, mode fs.FileMode) error {
	return os.MkdirAll(f.path(name), mode)
}

func (f *fakeHost) Symlink(target, name string) error {
	return os.Symlink(target, f.path(name))
}

func (f *fakeHost) Rename(a, b string) error {
	return os.Rename(f.path(a), f.path(b))
}

func (f *fakeHost) Remove(name string) error {
	return os.Remove(f.path(name))
}

func (f *fakeHost) RemoveAll(name string) error {
	return os.RemoveAll(f.path(name))
}

func (f *fakeHost) FreeBytes(string) (uint64, error) {
	return 1 << 40, nil
}

func (f *fakeHost) Close() error {
	return nil
}

func (f *fakeHost) Create(name string, mode fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(f.path(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
}

func (f *fakeHost) ReadFile(name string, limit int64) ([]byte, error) {
	file, err := os.Open(f.path(name))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(io.LimitReader(file, limit))
}

func (f *fakeHost) Exec(_ context.Context, argv []string) (deployrelease.ExecResult, error) {
	f.log.add(f.name, argv)
	if _, err := deployrelease.Command(argv...); err != nil {
		return deployrelease.ExecResult{}, err
	}
	switch argv[0] {
	case "sha256sum":
		return f.checksums(argv[len(argv)-1]), nil
	case "docker":
		if f.docker != nil {
			return f.docker(argv), nil
		}
	}
	return deployrelease.ExecResult{}, nil
}

func (f *fakeHost) checksums(manifest string) deployrelease.ExecResult {
	file, err := os.Open(f.path(manifest))
	if err != nil {
		return deployrelease.ExecResult{ExitCode: 1}
	}
	defer func() { _ = file.Close() }()
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		sum, name, ok := strings.Cut(sc.Text(), "  ")
		if !ok {
			return deployrelease.ExecResult{ExitCode: 1}
		}
		raw, err := os.ReadFile(f.path(name))
		if err != nil {
			return deployrelease.ExecResult{ExitCode: 1}
		}
		got := sha256.Sum256(raw)
		if hex.EncodeToString(got[:]) != sum {
			return deployrelease.ExecResult{ExitCode: 1}
		}
	}
	return deployrelease.ExecResult{}
}

func (f *fakeHost) current() string {
	link, err := os.Readlink(f.path("/srv/app/current"))
	if err != nil {
		return ""
	}
	return link
}

func (f *fakeHost) read(name string) string {
	raw, err := os.ReadFile(f.path(name))
	if err != nil {
		return ""
	}
	return string(raw)
}
