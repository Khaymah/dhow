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
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	envImage  = "DHOW_E2E_COMPOSE_HOST_IMAGE"
	envPlugin = "DHOW_E2E_PLUGIN"
	envReport = "DHOW_E2E_REPORTS"
	sshPort   = 22
	appBase   = "docker.io/library/busybox:1.37.0@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec"
	hostCount = 2
)

var (
	digestLine = regexp.MustCompile(`digest: (sha256:[0-9a-f]{64})`)

	hostsOnce sync.Once
	hosts     []*composeHost
	started   []string

	imagesMu sync.Mutex
	images   = map[string]string{}
)

func TestMain(m *testing.M) {
	code := m.Run()
	for _, name := range started {
		_ = exec.CommandContext(context.Background(), "docker", "rm", "-f", name).Run()
	}
	os.Exit(code)
}

type composeHost struct {
	name       string
	ip         string
	clientKey  []byte
	hostPublic string
}

func requireE2E(t *testing.T) (image, plugin string) {
	t.Helper()
	image, plugin = os.Getenv(envImage), os.Getenv(envPlugin)
	if image == "" || plugin == "" {
		t.Skipf("set %s to the compose host image and %s to the built dhow-deploy-compose to run the compose end to end tests (make deploy-e2e does)", envImage, envPlugin)
	}
	return image, plugin
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := dockerRun(nil, args...)
	require.NoError(t, err, "docker %v: %s", args, out)
	return out
}

func dockerRun(stdin *bytes.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "docker", args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if stdin != nil {
		cmd.Stdin = stdin
	}
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
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

func sharedHosts(t *testing.T, image string) []*composeHost {
	t.Helper()
	hostsOnce.Do(func() { hosts = startHosts(t, image) })
	require.Len(t, hosts, hostCount, "the compose hosts did not start")
	return hosts
}

func startHosts(t *testing.T, image string) []*composeHost {
	t.Helper()
	private, authorized := newClientKey(t)
	keys := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(keys, "authorized_keys"), []byte("restrict "+authorized+"\n"), 0o644))
	require.NoError(t, os.Chmod(keys, 0o755))
	var out []*composeHost
	for i := range hostCount {
		name := fmt.Sprintf("dhow-e2e-compose-%d-%d", i+1, time.Now().UnixNano())
		docker(t, "run", "-d", "--rm", "--privileged", "--name", name, "-v", keys+":/keys:ro", image)
		started = append(started, name)
		h := &composeHost{name: name, clientKey: private}
		h.ip = docker(t, "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
		require.NotEmpty(t, h.ip)
		out = append(out, h)
	}
	for _, h := range out {
		h.waitReady(t)
		h.hostPublic = docker(t, "exec", h.name, "cat", "/etc/ssh/ssh_host_ed25519_key.pub")
	}
	return out
}

func (h *composeHost) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(h.ip, fmt.Sprint(sshPort)), time.Second)
		if err == nil {
			_ = c.Close()
			_, dockerErr := dockerRun(nil, "exec", h.name, "docker", "info")
			_, registryErr := dockerRun(nil, "exec", h.name, "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:5000/v2/")
			if dockerErr == nil && registryErr == nil {
				return
			}
		}
		require.True(t, time.Now().Before(deadline), "the compose host did not come up: %s", docker(t, "logs", h.name))
		time.Sleep(500 * time.Millisecond)
	}
}

func (h *composeHost) knownHostsLine() string {
	fields := strings.Fields(h.hostPublic)
	return h.ip + " " + fields[0] + " " + fields[1]
}

func (h *composeHost) exec(t *testing.T, args ...string) string {
	t.Helper()
	return docker(t, append([]string{"exec", h.name}, args...)...)
}

func (h *composeHost) tryExec(args ...string) string {
	out, err := dockerRun(nil, append([]string{"exec", h.name}, args...)...)
	if err != nil {
		return ""
	}
	return out
}

func (h *composeHost) get(port int) (int, string) {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s:%d/", h.ip, port))
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, strings.TrimSpace(buf.String())
}

func (h *composeHost) served(port int) string {
	_, body := h.get(port)
	return body
}

func (h *composeHost) current(path string) string {
	return h.tryExec("readlink", path+"/current")
}

func (h *composeHost) releases(path string) []string {
	return strings.Fields(h.tryExec("ls", path+"/releases"))
}

func (h *composeHost) runningImages(project string) []string {
	var out []string
	for _, id := range strings.Fields(h.tryExec("docker", "ps", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.ID}}")) {
		out = append(out, h.tryExec("docker", "inspect", "--format", "{{.Config.Image}}", id))
	}
	return out
}

func (h *composeHost) prepare(t *testing.T, path, project string) {
	t.Helper()
	docker(t, "exec", "-u", "deploy", h.name, "mkdir", "-p", path+"/shared/state")
	t.Cleanup(func() {
		_, _ = dockerRun(nil, "exec", h.name, "docker", "compose", "--project-name="+project, "down", "--volumes", "--remove-orphans", "--timeout", "1")
	})
}

func (h *composeHost) setBad(t *testing.T, path, version string) {
	t.Helper()
	flag := path + "/shared/state/bad"
	if version == "" {
		h.exec(t, "rm", "-f", flag)
		return
	}
	out, err := dockerRun(bytes.NewReader([]byte(version)), "exec", "-i", "-u", "deploy", h.name, "tee", flag)
	require.NoError(t, err, out)
}

func publishApp(t *testing.T, version string) string {
	t.Helper()
	imagesMu.Lock()
	defer imagesMu.Unlock()
	if ref, ok := images[version]; ok {
		return ref
	}
	tag := "dhow-e2e-app:" + version
	dockerfile := fmt.Sprintf("FROM %s\nRUN mkdir /www && echo %s > /www/index.html\nEXPOSE 8080\nCMD [\"httpd\", \"-f\", \"-p\", \"8080\", \"-h\", \"/www\"]\n", appBase, version)
	out, err := dockerRun(bytes.NewReader([]byte(dockerfile)), "build", "--tag", tag, "-")
	require.NoError(t, err, out)
	var digest string
	for _, h := range sharedHosts(t, os.Getenv(envImage)) {
		save := exec.CommandContext(context.Background(), "docker", "save", tag)
		load := exec.CommandContext(context.Background(), "docker", "exec", "-i", h.name, "docker", "load")
		pipe, err := save.StdoutPipe()
		require.NoError(t, err)
		load.Stdin = pipe
		var loadOut bytes.Buffer
		load.Stdout, load.Stderr = &loadOut, &loadOut
		require.NoError(t, save.Start())
		require.NoError(t, load.Start())
		require.NoError(t, load.Wait(), loadOut.String())
		require.NoError(t, save.Wait())
		registryTag := "localhost:5000/app:" + version
		h.exec(t, "docker", "tag", tag, registryTag)
		pushed := h.exec(t, "docker", "push", registryTag)
		m := digestLine.FindStringSubmatch(pushed)
		require.Len(t, m, 2, pushed)
		if digest == "" {
			digest = m[1]
		}
		require.Equal(t, digest, m[1], "both hosts hold the same image digest")
		h.exec(t, "docker", "rmi", registryTag, tag)
	}
	ref := "localhost:5000/app@" + digest
	images[version] = ref
	return ref
}
