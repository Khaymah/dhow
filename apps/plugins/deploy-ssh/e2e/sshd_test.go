package e2e_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	envImage  = "DHOW_E2E_SSHD_IMAGE"
	envPlugin = "DHOW_E2E_PLUGIN"
	envReport = "DHOW_E2E_REPORTS"
	httpPort  = 8080
	sshPort   = 22
)

var (
	once    sync.Once
	shared  *sshd
	started []string
)

func TestMain(m *testing.M) {
	code := m.Run()
	for _, name := range started {
		_ = exec.CommandContext(context.Background(), "docker", "rm", "-f", name).Run()
	}
	os.Exit(code)
}

func sharedSshd(t *testing.T, image string) *sshd {
	t.Helper()
	once.Do(func() { shared = startSshd(t, image) })
	require.NotNil(t, shared, "the test sshd did not start")
	return shared
}

type sshd struct {
	name       string
	ip         string
	clientKey  []byte
	hostPublic string
}

func requireE2E(t *testing.T) (image, plugin string) {
	t.Helper()
	image, plugin = os.Getenv(envImage), os.Getenv(envPlugin)
	if image == "" || plugin == "" {
		t.Skipf("set %s to the sshd image and %s to the built dhow-deploy-ssh to run the deploy end to end tests (make deploy-e2e does)", envImage, envPlugin)
	}
	return image, plugin
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "docker", args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	require.NoError(t, cmd.Run(), "docker %v: %s", args, errOut.String())
	return strings.TrimSpace(out.String())
}

func newClientKey(t *testing.T) (private []byte, authorized string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return pem.EncodeToMemory(block), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

func startSshd(t *testing.T, image string) *sshd {
	t.Helper()
	private, authorized := newClientKey(t)
	return startSshdWith(t, image, private, authorized)
}

func startSshdWith(t *testing.T, image string, private []byte, authorized string, extra ...string) *sshd {
	t.Helper()
	keys := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(keys, "authorized_keys"), []byte("restrict "+authorized+"\n"), 0o644))
	require.NoError(t, os.Chmod(keys, 0o755))
	name := fmt.Sprintf("dhow-e2e-sshd-%d", time.Now().UnixNano())
	docker(t, append(append([]string{"run", "-d", "--rm", "--name", name, "-v", keys + ":/keys:ro"}, extra...), image)...)
	started = append(started, name)
	s := &sshd{name: name, clientKey: private}
	s.ip = docker(t, "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	require.NotEmpty(t, s.ip)
	s.waitReady(t)
	s.hostPublic = docker(t, "exec", name, "cat", "/etc/ssh/ssh_host_ed25519_key.pub")
	return s
}

func (s *sshd) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(s.ip, fmt.Sprint(sshPort)), time.Second)
		if err == nil {
			_ = c.Close()
			resp, httpErr := http.Get(fmt.Sprintf("http://%s:%d/", s.ip, httpPort))
			if httpErr == nil {
				_ = resp.Body.Close()
				return
			}
		}
		require.True(t, time.Now().Before(deadline), "the test sshd did not come up: %s", docker(t, "logs", s.name))
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *sshd) knownHostsLine() string {
	fields := strings.Fields(s.hostPublic)
	return s.ip + " " + fields[0] + " " + fields[1]
}

func (s *sshd) get(path string) (int, string) {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d%s", s.ip, httpPort, path))
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, strings.TrimSpace(buf.String())
}

func otherHostKey(t *testing.T, s *sshd) string {
	t.Helper()
	_, authorized := newClientKey(t)
	fields := strings.Fields(authorized)
	return s.ip + " " + fields[0] + " " + fields[1]
}
