package installer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type fakeFile struct {
	data []byte
	mode FileMode
	dir  bool
}

type fakeSys struct {
	mu       sync.Mutex
	files    map[string]*fakeFile
	bins     map[string]bool
	root     bool
	ok       map[string]bool
	fail     map[string]bool
	failOnce map[string]bool
	stdout   map[string]string
	onRun    func(f *fakeSys, c Command)
	commands []string
	stdins   []string
	httpFn   func(p Probe) (ProbeResult, error)
	fetched  map[string][]byte
	free     uint64
	clock    time.Time
	exe      string
}

func newFake() *fakeSys {
	return &fakeSys{
		files:    map[string]*fakeFile{},
		bins:     map[string]bool{},
		ok:       map[string]bool{},
		fail:     map[string]bool{},
		failOnce: map[string]bool{},
		stdout:   map[string]string{},
		fetched:  map[string][]byte{},
		root:     true,
		free:     100 * gib,
		clock:    time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		exe:      "/tmp/get/dhow-server",
	}
}

func (f *fakeSys) resources(memTotalKiB, cpus string, free uint64) {
	f.put(MemInfoPath, "MemTotal:       "+memTotalKiB+" kB\nMemFree:         1024 kB\n", 0o444)
	f.put(CPUOnlinePath, cpus+"\n", 0o444)
	f.free = free
}

func (f *fakeSys) put(p, content string, perm fs.FileMode) {
	f.files[p] = &fakeFile{data: []byte(content), mode: FileMode{Perm: perm}}
}

func (f *fakeSys) content(p string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if file, ok := f.files[p]; ok {
		return string(file.data)
	}
	return ""
}

func (f *fakeSys) has(p string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.files[p]
	return ok
}

func (f *fakeSys) ran(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.commands {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (f *fakeSys) Run(_ context.Context, c Command) (Output, error) {
	s := c.String()
	f.mu.Lock()
	f.commands = append(f.commands, s)
	f.stdins = append(f.stdins, string(c.Stdin))
	hook := f.onRun
	f.mu.Unlock()
	if hook != nil {
		hook(f, c)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := Output{Stdout: []byte(f.stdout[s])}
	for k := range f.failOnce {
		if strings.HasPrefix(s, k) {
			delete(f.failOnce, k)
			out.ExitCode = 1
			out.Stderr = []byte("simulated failure")
			return out, fmt.Errorf("%w with exit code 1", ErrCommandFailed)
		}
	}
	for k := range f.fail {
		if strings.HasPrefix(s, k) {
			out.ExitCode = 1
			out.Stderr = []byte("simulated failure")
			return out, fmt.Errorf("%w with exit code 1", ErrCommandFailed)
		}
	}
	if isCheck(c) && !f.ok[s] {
		out.ExitCode = 1
		return out, fmt.Errorf("%w with exit code 1", ErrCommandFailed)
	}
	return out, nil
}

func isCheck(c Command) bool {
	s := c.String()
	for _, p := range []string{"getent ", "systemctl is-", "rpm -q", "docker info", "docker compose version", "docker compose --project-directory /etc/dhow/compose --file /etc/dhow/compose/compose.yaml ps", "docker network inspect"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func (f *fakeSys) LookPath(name string) bool { return f.bins[name] }

func (f *fakeSys) ReadFile(p string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.files[p]
	if !ok || file.dir {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), file.data...), nil
}

func (f *fakeSys) WriteFile(p string, data []byte, mode FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[p] = &fakeFile{data: append([]byte(nil), data...), mode: mode}
	return nil
}

func (f *fakeSys) Chmod(p string, mode FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.files[p]
	if !ok {
		return &fs.PathError{Op: "chmod", Path: p, Err: fs.ErrNotExist}
	}
	file.mode = mode
	return nil
}

func (f *fakeSys) Stat(p string) (FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.files[p]
	if !ok {
		return FileInfo{}, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
	}
	uid := 0
	if file.mode.Owner != "" && file.mode.Owner != "root" {
		uid = 1000
		if n, err := strconv.Atoi(file.mode.Owner); err == nil {
			uid = n
		}
	}
	mode := file.mode.Perm
	if file.dir {
		mode |= fs.ModeDir
	}
	return FileInfo{Mode: mode, UID: uid, GID: uid, Size: int64(len(file.data)), IsDir: file.dir}, nil
}

func (f *fakeSys) Remove(p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, p)
	return nil
}

func (f *fakeSys) RemoveAll(p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.files {
		if k == p || strings.HasPrefix(k, p+"/") {
			delete(f.files, k)
		}
	}
	return nil
}

func (f *fakeSys) MkdirAll(p string, mode FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[p] = &fakeFile{mode: mode, dir: true}
	return nil
}

func (f *fakeSys) Glob(pattern string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.files {
		if ok, _ := path.Match(pattern, k); ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeSys) Fetch(_ context.Context, url string, _ int64) ([]byte, error) {
	if data, ok := f.fetched[url]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("download %s: status 404", url)
}

func (f *fakeSys) HTTP(_ context.Context, p Probe) (ProbeResult, error) {
	if f.httpFn != nil {
		return f.httpFn(p)
	}
	return ProbeResult{}, fmt.Errorf("dial %s: connection refused", p.URL)
}

func (f *fakeSys) FreeBytes(string) (uint64, error) { return f.free, nil }
func (f *fakeSys) IsRoot() bool                     { return f.root }
func (f *fakeSys) Executable() (string, error)      { return f.exe, nil }

func (f *fakeSys) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = f.clock.Add(time.Second)
	return f.clock
}

const debianRelease = `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION_CODENAME=bookworm
ID=debian
`

const rockyRelease = `NAME="Rocky Linux"
VERSION_ID="9.4"
ID="rocky"
ID_LIKE="rhel centos fedora"
PRETTY_NAME="Rocky Linux 9.4 (Blue Onyx)"
`

const readyBody = `{"Success":true,"Data":{"Status":"ok","Checks":[{"Name":"database","Ok":true},{"Name":"migrations","Ok":true}]}}`

func debianHost() *fakeSys {
	f := newFake()
	f.resources("8135536", "0-3", 100*gib)
	f.put(OSReleasePath, debianRelease, 0o644)
	f.files["/run/systemd/system"] = &fakeFile{dir: true, mode: FileMode{Perm: 0o755}}
	f.put(f.exe, "dhow-server binary v1", 0o755)
	f.bins["sudo"] = true
	f.fetched[AptKeyURL] = mustRead("testdata/pgdg-apt.asc")
	f.fetched[DnfKeyURL] = mustRead("testdata/pgdg-rpm.asc")
	return f
}

type service struct {
	active bool
}

func liveHost(base *fakeSys) *fakeSys {
	svc := &service{}
	pgUp := false
	base.onRun = func(f *fakeSys, c Command) {
		s := c.String()
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case strings.HasPrefix(s, "useradd "):
			f.ok["getent passwd dhow"] = true
		case s == "systemctl restart dhow":
			svc.active = true
			f.ok["systemctl is-active --quiet dhow"] = true
			f.ok["systemctl is-enabled --quiet dhow"] = true
		case strings.HasPrefix(s, "systemctl enable --now postgresql"):
			pgUp = true
			f.ok["systemctl is-active --quiet postgresql"] = true
		case strings.HasPrefix(s, "apt-get install") && strings.Contains(s, "postgresql-18"):
			f.files["/usr/lib/postgresql/18/bin/postgres"] = &fakeFile{mode: FileMode{Perm: 0o755}}
			f.files["/usr/lib/postgresql/18/bin/psql"] = &fakeFile{mode: FileMode{Perm: 0o755}}
		case strings.Contains(s, "psql") && strings.Contains(s, "-tA"):
			if pgUp && strings.Contains(strings.Join(f.stdins, ""), "CREATE DATABASE dhow") {
				f.stdout[s] = "1\n"
			}
		case strings.HasPrefix(s, "journalctl --unit dhow --output cat"):
			f.stdout[s] = `{"level":"INFO","msg":"first-run setup is required","setup_url":"https://dhow.example.com/setup#TOKEN"}` + "\n"
		}
	}
	base.httpFn = func(p Probe) (ProbeResult, error) {
		if !svc.active || !strings.HasPrefix(p.URL, "http://127.0.0.1:8080/") {
			return ProbeResult{}, fmt.Errorf("dial %s: connection refused", p.URL)
		}
		if strings.HasSuffix(p.URL, "/api/v1/meta") {
			return ProbeResult{Status: 200, Body: []byte(`{"Success":true,"Data":{"SetupRequired":true}}`), CertDays: -1}, nil
		}
		return ProbeResult{Status: 200, Body: []byte(readyBody), CertDays: -1}, nil
	}
	return base
}

type fakeDB struct {
	major     int
	err       error
	stored    map[string]bool
	providers int64
	stranded  Stranded
	active    *[]string
}

func (d fakeDB) ServerMajor(context.Context, string) (int, error) { return d.major, d.err }

func (d fakeDB) StoredSettings(context.Context, string) (map[string]bool, error) {
	return d.stored, d.err
}

func (d fakeDB) StoredProviders(context.Context, string) (int64, error) { return d.providers, d.err }

func (d fakeDB) StrandedIdentities(_ context.Context, _ string, active []string) (Stranded, error) {
	if d.active != nil {
		*d.active = active
	}
	return d.stranded, d.err
}

func fsMode(p uint32) fs.FileMode { return fs.FileMode(p) }

func mustRead(p string) []byte {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return data
}
