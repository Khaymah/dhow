//go:build chaos

package chaos_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

var setupLink = regexp.MustCompile(`/setup#([A-Za-z0-9_-]{43})`)

type process struct {
	name string
	cmd  *exec.Cmd
	log  *os.File
	done chan struct{}
}

func freePort() (string, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port, ln.Close()
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func spawn(name, logPath string, env []string, stdin io.Reader, onLine func(string), bin string, args ...string) (*process, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin = stdin
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	p := &process{name: name, cmd: cmd, log: logFile, done: make(chan struct{})}
	go func() {
		s := bufio.NewScanner(pr)
		s.Buffer(make([]byte, 64<<10), 1<<20)
		for s.Scan() {
			line := s.Text()
			_, _ = logFile.WriteString(line + "\n")
			if onLine != nil {
				onLine(line)
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		_ = pw.Close()
		close(p.done)
	}()
	return p, nil
}

func (p *process) kill9() {
	if p == nil || p.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	<-p.done
}

type stack struct {
	work, bin, publicURL, serverAddr string
	dbURL, secretKey                 string
	runnerDir                        string
	keyring                          string
	runnerProxy                      *faultyProxy
	mu                               sync.Mutex
	server, runner                   *process
}

func (s *stack) serverEnv() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"DHOW_DATABASE_URL=" + s.dbURL, "DHOW_SECRET_KEY=" + s.secretKey, "DHOW_KEYRING_FILE=" + s.keyring,
		"DHOW_PUBLIC_URL=" + s.publicURL,
		"DHOW_LISTEN_ADDR=" + s.serverAddr, "DHOW_DEV=true", "DHOW_BREACHED_PASSWORD_CHECK=false", "DHOW_LOG_LEVEL=debug",
	}
}

func (s *stack) initKeyring() error {
	s.keyring = filepath.Join(s.work, "keyring")
	cmd := exec.Command(filepath.Join(s.bin, "dhow-server"), "keys", "init", "--file", s.keyring)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create the server keyring: %w: %s", err, out)
	}
	return nil
}

func (s *stack) startServer() (string, error) {
	tokens := make(chan string, 1)
	p, err := spawn("server", filepath.Join(s.work, "server.log"), s.serverEnv(), nil, func(line string) {
		if m := setupLink.FindStringSubmatch(line); m != nil {
			select {
			case tokens <- m[1]:
			default:
			}
		}
	}, filepath.Join(s.bin, "dhow-server"))
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.server = p
	s.mu.Unlock()
	if err := waitReady(s.publicURL+"/api/v1/health/ready", 2*time.Minute, p.done); err != nil {
		return "", err
	}
	select {
	case tok := <-tokens:
		return tok, nil
	case <-time.After(2 * time.Second):
		return "", nil
	}
}

func waitReady(url string, within time.Duration, exited <-chan struct{}) error {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return fmt.Errorf("the process exited before %s was ready", url)
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("%s was not ready within %s", url, within)
}

func (s *stack) runnerEnv() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + s.runnerDir,
		"DHOW_RUNNER_STATE_DIR=" + s.runnerDir, "DHOW_RUNNER_SERVER_ORIGIN=" + s.publicURL,
		"DHOW_RUNNER_DRIVER=podman", "DHOW_RUNNER_CAPACITY=2", "DHOW_RUNNER_MAX_MEMORY=512MiB", "DHOW_RUNNER_MAX_DISK=256MiB",
		"DHOW_RUNNER_CANCEL_GRACE=2s", "DHOW_RUNNER_EGRESS_ALLOW_PRIVATE_CIDRS=127.0.0.0/8", "DHOW_RUNNER_LOG_LEVEL=debug",
	}
}

func (s *stack) register(token string) error {
	url := "http://" + s.runnerProxy.Addr()
	p, err := spawn("register", filepath.Join(s.work, "register.log"), s.runnerEnv(), strings.NewReader(token+"\n"), nil,
		filepath.Join(s.bin, "dhow-runner"), "register", "--dev", "--url", url, "--token-file", "-")
	if err != nil {
		return err
	}
	<-p.done
	if code := p.cmd.ProcessState.ExitCode(); code != 0 {
		return fmt.Errorf("dhow-runner register exited %d, see %s", code, filepath.Join(s.work, "register.log"))
	}
	return nil
}

func (s *stack) startRunner() error {
	p, err := spawn("runner", filepath.Join(s.work, "runner.log"), s.runnerEnv(), nil, nil,
		filepath.Join(s.bin, "dhow-runner"), "run", "--dev")
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.runner = p
	s.mu.Unlock()
	return nil
}

func (s *stack) killRunner() {
	s.mu.Lock()
	p := s.runner
	s.runner = nil
	s.mu.Unlock()
	p.kill9()
}

func (s *stack) killServer() {
	s.mu.Lock()
	p := s.server
	s.server = nil
	s.mu.Unlock()
	p.kill9()
}

func (s *stack) runnerPodman(args ...string) (string, error) {
	conf := filepath.Join(s.runnerDir, "containers")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=" + s.runnerDir,
		"CONTAINERS_CONF=" + filepath.Join(conf, "containers.conf"), "CONTAINERS_STORAGE_CONF=" + filepath.Join(conf, "storage.conf"),
		"CONTAINERS_REGISTRIES_CONF=" + filepath.Join(conf, "registries.conf"), "XDG_RUNTIME_DIR=" + filepath.Join(s.runnerDir, "run"),
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (s *stack) sandboxes() []string {
	return s.listSandboxes("--all")
}

func (s *stack) runningSandboxes() []string {
	return s.listSandboxes()
}

func (s *stack) listSandboxes(flags ...string) []string {
	args := append([]string{"ps"}, flags...)
	out, err := s.runnerPodman(append(args, "--filter", "label=dhow.runner", "--format", "{{.Names}}")...)
	if err != nil || out == "" {
		return nil
	}
	return strings.Fields(out)
}
