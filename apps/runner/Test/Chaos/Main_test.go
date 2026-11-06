//go:build chaos

package chaos_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const (
	envBin     = "DHOW_CHAOS_BIN"
	envBusybox = "DHOW_CHAOS_BUSYBOX"
	sinkHost   = "sink.dhow.test"
)

type suite struct {
	stack     *stack
	api       *api
	sink      *sink
	image     string
	slowImage string
	slowPull  *faultyProxy
}

var shared suite

func TestMain(m *testing.M) {
	cleanups, err := setUp()
	code := 1
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "chaos setup failed:", err)
	} else {
		code = m.Run()
	}
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
	os.Exit(code)
}

func setUp() ([]func(), error) {
	var cleanups []func()
	bin, busybox := os.Getenv(envBin), os.Getenv(envBusybox)
	if bin == "" || busybox == "" {
		return nil, fmt.Errorf("set %s and %s (make runner-chaos does)", envBin, envBusybox)
	}
	work, err := os.MkdirTemp("", "dhow-chaos-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(work, 0o711); err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintln(os.Stderr, "chaos work dir:", work)
	s := &stack{work: work, bin: bin, secretKey: randomHex(32), runnerDir: filepath.Join(work, "runner")}
	if err := os.MkdirAll(s.runnerDir, 0o700); err != nil {
		return nil, err
	}
	if err := s.initKeyring(); err != nil {
		return nil, err
	}
	certs := filepath.Join(work, "certs")
	if err := os.MkdirAll(certs, 0o700); err != nil {
		return nil, err
	}
	caPath, err := writeCerts(certs)
	if err != nil {
		return nil, err
	}
	if err := setUpServices(s, work, certs, caPath, &cleanups); err != nil {
		return cleanups, err
	}
	if err := setUpImages(work, busybox, &cleanups); err != nil {
		return cleanups, err
	}
	if err := setUpStack(s, &cleanups); err != nil {
		return cleanups, err
	}
	return cleanups, nil
}

func setUpServices(s *stack, work, certs, caPath string, cleanups *[]func()) error {
	pgPort, err := freePort()
	if err != nil {
		return err
	}
	pgPassword := randomHex(16)
	s.dbURL = "postgres://postgres:" + pgPassword + "@127.0.0.1:" + pgPort + "/dhow?sslmode=disable"
	if err := startPostgres(pgPort, pgPassword, s.dbURL); err != nil {
		return err
	}
	*cleanups = append(*cleanups, func() { _, _ = podman("rm", "--force", pgContainer) })
	if err := startRegistry(certs); err != nil {
		return err
	}
	*cleanups = append(*cleanups, func() { _, _ = podman("rm", "--force", regContainer) })
	pushCerts := filepath.Join(work, "pushcerts")
	if err := os.MkdirAll(pushCerts, 0o700); err != nil {
		return err
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pushCerts, "ca.crt"), ca, 0o600); err != nil {
		return err
	}
	if err := trustRegistry(s.runnerDir, caPath); err != nil {
		return err
	}
	slow, err := newFaultyProxy("127.0.0.1:"+slowPort, "127.0.0.1:"+registryPort)
	if err != nil {
		return err
	}
	*cleanups = append(*cleanups, slow.Close)
	shared.slowPull = slow
	sk, closeSink, err := newSink()
	if err != nil {
		return err
	}
	*cleanups = append(*cleanups, closeSink)
	shared.sink = sk
	return nil
}

func setUpImages(work, busybox string, cleanups *[]func()) error {
	image, err := pushImage(work, busybox, "chaos", 0)
	if err != nil {
		return err
	}
	slow, err := pushImage(work, busybox, "chaos-big", 8<<20)
	if err != nil {
		return err
	}
	shared.image = "localhost:" + registryPort + "/" + image
	shared.slowImage = "localhost:" + slowPort + "/" + slow
	*cleanups = append(*cleanups, func() { _, _ = podman("image", "prune", "--all", "--force") })
	return nil
}

func setUpStack(s *stack, cleanups *[]func()) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	s.serverAddr = "127.0.0.1:" + port
	s.publicURL = "http://localhost:" + port
	setupToken, err := s.startServer()
	if err != nil {
		return err
	}
	*cleanups = append(*cleanups, func() { s.killServer() })
	if setupToken == "" {
		return fmt.Errorf("the server logged no setup link, see %s", filepath.Join(s.work, "server.log"))
	}
	proxy, err := newFaultyProxy("127.0.0.1:0", s.serverAddr)
	if err != nil {
		return err
	}
	*cleanups = append(*cleanups, proxy.Close)
	s.runnerProxy = proxy
	a := &api{base: s.publicURL}
	if err := a.setupE(setupToken); err != nil {
		return err
	}
	token, err := a.registrationTokenE()
	if err != nil {
		return err
	}
	if err := s.register(token); err != nil {
		return err
	}
	if err := s.startRunner(); err != nil {
		return err
	}
	*cleanups = append(*cleanups, func() { s.killRunner(); _, _ = s.runnerPodman("rm", "--all", "--force") })
	shared.stack, shared.api = s, a
	return nil
}
