package installer

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	fetchTimeout  = 2 * time.Minute
	probeTimeout  = 10 * time.Second
	probeMaxBytes = 64 << 10
	hoursPerDay   = 24
)

type Host struct{}

func NewHost() *Host { return &Host{} }

func (Host) Run(ctx context.Context, c Command) (Output, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...) //nolint:gosec // Commands are argv slices built by the installer, never a shell string.
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(cmd.Environ(), c.Env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		out.ExitCode = exitErr.ExitCode()
		return out, fmt.Errorf("%w with exit code %d", ErrCommandFailed, out.ExitCode)
	}
	if err != nil {
		out.ExitCode = -1
		return out, fmt.Errorf("%w: %w", ErrCommandFailed, err)
	}
	return out, nil
}

func (Host) LookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (Host) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // The installer reads fixed system paths and files the operator named.
}

func (Host) WriteFile(path string, data []byte, mode FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dhow-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(mode.Perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := chownFile(tmp, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	ok = true
	return nil
}

func (Host) Chmod(path string, mode FileMode) error {
	if err := os.Chmod(path, mode.Perm); err != nil {
		return err
	}
	uid, gid, err := ids(mode)
	if err != nil {
		return err
	}
	return os.Chown(path, uid, gid)
}

func chownFile(f *os.File, mode FileMode) error {
	uid, gid, err := ids(mode)
	if err != nil {
		return err
	}
	return f.Chown(uid, gid)
}

func ids(mode FileMode) (int, int, error) {
	owner, group := mode.Owner, mode.Group
	if owner == "" {
		owner = "root"
	}
	if group == "" {
		group = "root"
	}
	uid, err := numericID(owner, func(name string) (string, error) {
		u, err := user.Lookup(name)
		if err != nil {
			return "", err
		}
		return u.Uid, nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("look up user %s: %w", owner, err)
	}
	gid, err := numericID(group, func(name string) (string, error) {
		g, err := user.LookupGroup(name)
		if err != nil {
			return "", err
		}
		return g.Gid, nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("look up group %s: %w", group, err)
	}
	return uid, gid, nil
}

func numericID(name string, lookup func(string) (string, error)) (int, error) {
	if n, err := strconv.Atoi(name); err == nil && n >= 0 {
		return n, nil
	}
	id, err := lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(id)
}

func (Host) Stat(path string) (FileInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	return infoOf(st), nil
}

func (Host) Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (Host) RemoveAll(path string) error { return os.RemoveAll(path) }

func (Host) MkdirAll(path string, mode FileMode) error {
	if err := os.MkdirAll(path, mode.Perm); err != nil {
		return err
	}
	return Host{}.Chmod(path, mode)
}

func (Host) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }

func (Host) Fetch(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("refusing to download %s: only https is allowed", rawURL)
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", rawURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download %s: larger than %d bytes", rawURL, limit)
	}
	return data, nil
}

func (Host) HTTP(ctx context.Context, p Probe) (ProbeResult, error) {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = probeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	method := p.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, p.URL, http.NoBody)
	if err != nil {
		return ProbeResult{}, err
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Transport:     &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, probeMaxBytes))
	if err != nil {
		return ProbeResult{}, err
	}
	res := ProbeResult{Status: resp.StatusCode, Header: resp.Header, Body: body, CertDays: -1}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		res.CertDays = int(time.Until(resp.TLS.PeerCertificates[0].NotAfter).Hours() / hoursPerDay)
	}
	return res, nil
}

func (Host) FreeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	if st.Bsize <= 0 {
		return 0, nil
	}
	size := uint64(st.Bsize)
	if st.Bavail > math.MaxUint64/size {
		return math.MaxUint64, nil
	}
	return st.Bavail * size, nil
}

func (Host) IsRoot() bool { return os.Geteuid() == 0 }

func (Host) Executable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

func (Host) Now() time.Time { return time.Now() }
