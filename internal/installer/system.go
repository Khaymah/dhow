package installer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

var ErrCommandFailed = errors.New("command failed")

type Command struct {
	Name  string
	Args  []string
	Stdin []byte
	Env   []string
	Dir   string
}

func Cmd(name string, args ...string) Command { return Command{Name: name, Args: args} }

func (c Command) String() string { return strings.Join(append([]string{c.Name}, c.Args...), " ") }

type Output struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type FileMode struct {
	Perm  fs.FileMode
	Owner string
	Group string
}

type FileInfo struct {
	Mode  fs.FileMode
	UID   int
	GID   int
	Size  int64
	IsDir bool
}

type Probe struct {
	Method  string
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

type ProbeResult struct {
	Status   int
	Header   http.Header
	Body     []byte
	CertDays int
}

type System interface {
	Run(ctx context.Context, c Command) (Output, error)
	LookPath(name string) bool
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, mode FileMode) error
	Chmod(path string, mode FileMode) error
	Stat(path string) (FileInfo, error)
	Lstat(path string) (FileInfo, error)
	Remove(path string) error
	RemoveAll(path string) error
	MkdirAll(path string, mode FileMode) error
	Glob(pattern string) ([]string, error)
	Fetch(ctx context.Context, url string, limit int64) ([]byte, error)
	HTTP(ctx context.Context, p Probe) (ProbeResult, error)
	FreeBytes(path string) (uint64, error)
	IsRoot() bool
	Executable() (string, error)
	Now() time.Time
}

func Run(ctx context.Context, sys System, c Command) (Output, error) {
	out, err := sys.Run(ctx, c)
	if err != nil {
		return out, fmt.Errorf("%s: %w%s", c.String(), err, StderrTail(out.Stderr))
	}
	return out, nil
}

func Succeeds(ctx context.Context, sys System, c Command) bool {
	_, err := sys.Run(ctx, c)
	return err == nil
}

func OutputHas(ctx context.Context, sys System, c Command, want string) bool {
	out, err := sys.Run(ctx, c)
	return err == nil && strings.Contains(string(out.Stdout), want)
}

const stderrTailLines = 6

func StderrTail(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	if len(lines) > stderrTailLines {
		lines = lines[len(lines)-stderrTailLines:]
	}
	return "\n    " + strings.Join(lines, "\n    ")
}

func Exists(sys System, path string) bool {
	_, err := sys.Stat(path)
	return err == nil
}
