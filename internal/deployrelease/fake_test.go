package deployrelease_test

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

type localRemote struct {
	t      *testing.T
	root   string
	mu     sync.Mutex
	execs  [][]string
	hook   func(argv []string) deployrelease.ExecResult
	free   uint64
	closed bool
}

func newLocalRemote(t *testing.T) *localRemote {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "srv", "app"), 0o755))
	return &localRemote{t: t, root: root, free: 1 << 40}
}

func (l *localRemote) path(name string) string {
	return filepath.Join(l.root, filepath.FromSlash(name))
}

func (l *localRemote) Lstat(name string) (deployrelease.FileInfo, error) {
	info, err := os.Lstat(l.path(name))
	if err != nil {
		return deployrelease.FileInfo{}, err
	}
	return deployrelease.FileInfo{Name: info.Name(), Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime()}, nil
}

func (l *localRemote) ReadDir(name string) ([]deployrelease.FileInfo, error) {
	list, err := os.ReadDir(l.path(name))
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

func (l *localRemote) Readlink(name string) (string, error) { return os.Readlink(l.path(name)) }
func (l *localRemote) Mkdir(name string, mode fs.FileMode) error {
	return os.Mkdir(l.path(name), mode)
}

func (l *localRemote) MkdirAll(name string, mode fs.FileMode) error {
	return os.MkdirAll(l.path(name), mode)
}
func (l *localRemote) Symlink(target, name string) error { return os.Symlink(target, l.path(name)) }
func (l *localRemote) Rename(a, b string) error          { return os.Rename(l.path(a), l.path(b)) }
func (l *localRemote) Remove(name string) error          { return os.Remove(l.path(name)) }
func (l *localRemote) RemoveAll(name string) error       { return os.RemoveAll(l.path(name)) }
func (l *localRemote) Create(name string, mode fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(l.path(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
}

func (l *localRemote) ReadFile(name string, limit int64) ([]byte, error) {
	f, err := os.Open(l.path(name))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, limit))
}

func (l *localRemote) FreeBytes(string) (uint64, error) { return l.free, nil }
func (l *localRemote) Close() error                     { l.closed = true; return nil }

func (l *localRemote) Exec(_ context.Context, argv []string) (deployrelease.ExecResult, error) {
	l.mu.Lock()
	l.execs = append(l.execs, append([]string(nil), argv...))
	l.mu.Unlock()
	if _, err := deployrelease.Command(argv...); err != nil {
		return deployrelease.ExecResult{}, err
	}
	if argv[0] == "sha256sum" {
		return l.checksums(argv[len(argv)-1]), nil
	}
	if l.hook != nil {
		return l.hook(argv), nil
	}
	return deployrelease.ExecResult{}, nil
}

func (l *localRemote) checksums(manifest string) deployrelease.ExecResult {
	f, err := os.Open(l.path(manifest))
	if err != nil {
		return deployrelease.ExecResult{ExitCode: 1, Output: []byte(err.Error())}
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, name, ok := strings.Cut(sc.Text(), "  ")
		if !ok {
			return deployrelease.ExecResult{ExitCode: 1}
		}
		raw, err := os.ReadFile(l.path(name))
		if err != nil {
			return deployrelease.ExecResult{ExitCode: 1, Output: []byte(err.Error())}
		}
		got := sha256.Sum256(raw)
		if hex.EncodeToString(got[:]) != sum {
			return deployrelease.ExecResult{ExitCode: 1, Output: []byte(name + ": FAILED")}
		}
	}
	return deployrelease.ExecResult{}
}

func (l *localRemote) hookArgvs() [][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out [][]string
	for _, a := range l.execs {
		if a[0] == "env" {
			out = append(out, a)
		}
	}
	return out
}

type probe struct {
	mu     sync.Mutex
	status func(call int, url string) int
	calls  int
	urls   []string
}

func (p *probe) Do(r *http.Request) (*http.Response, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.urls = append(p.urls, r.URL.String())
	p.mu.Unlock()
	code := p.status(n, r.URL.String())
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
}

func healthy(int, string) int   { return 200 }
func unhealthy(int, string) int { return 503 }

type recorder struct {
	mu     sync.Mutex
	events []deployrelease.Event
}

func (r *recorder) Emit(e deployrelease.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		k := e.Event
		if e.HostID == "" && k != deployrelease.EventStarted {
			k += "/group"
		}
		out = append(out, k)
	}
	return out
}

func (r *recorder) find(kind string) (deployrelease.Event, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Event == kind {
			return e, true
		}
	}
	return deployrelease.Event{}, false
}

func fastClock() deployrelease.Clock {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	return deployrelease.Clock{
		Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			now = now.Add(10 * time.Millisecond)
			return now
		},
		Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	}
}

func makeArchive(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for name, body := range files {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
		paths = append(paths, strings.SplitN(name, "/", 2)[0])
	}
	out := filepath.Join(t.TempDir(), "artifact.tgz")
	f, err := os.Create(out)
	require.NoError(t, err)
	require.NoError(t, archive.Pack(dir, paths, f))
	require.NoError(t, f.Close())
	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return out, hex.EncodeToString(sum[:])
}

func baseConfig(archivePath, digest, id string) deployrelease.Config {
	return deployrelease.Config{
		DeployPath: "/srv/app", ReleaseID: id, HostID: "0190a1b2-0000-7000-8000-000000000001", HostName: "web-1.example.com",
		Keep: 5, HookTimeout: time.Minute, FirstFailure: deployrelease.FirstFailureKeep, ArchivePath: archivePath, ArchiveDigest: digest,
		Health: deployrelease.Health{
			URL: "https://{host}/healthz", StatusMin: 200, StatusMax: 399, Timeout: time.Second, Retries: 5, Interval: time.Second, Stable: 3,
		},
	}
}

type outcome struct {
	failure *deployrelease.Failure
	events  *recorder
}

func deploy(t *testing.T, r *localRemote, cfg deployrelease.Config, doer deployrelease.Doer) outcome {
	t.Helper()
	rec := &recorder{}
	e := &deployrelease.Engine{
		Config: cfg, Doer: doer, Report: rec, Clock: fastClock(),
		Connect: func(context.Context) (deployrelease.Remote, error) { return r, nil },
	}
	return outcome{failure: e.Run(context.Background()), events: rec}
}

func (l *localRemote) currentTarget() string {
	target, err := os.Readlink(l.path("/srv/app/current"))
	if err != nil {
		return ""
	}
	return target
}

func (l *localRemote) releaseNames() []string {
	list, err := os.ReadDir(l.path("/srv/app/releases"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

func (l *localRemote) read(name string) string {
	raw, err := os.ReadFile(l.path(name))
	if err != nil {
		return ""
	}
	return string(raw)
}

var errConnect = errors.New("the dial failed")

func unsafeArchive(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "evil.tgz")
	f, err := os.Create(out)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "index.html", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}))
	_, err = tw.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "../escape.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}))
	_, err = tw.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
	return out
}
