package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	publicURL    = "https://dhow.bench.test"
	upTimeout    = 4 * time.Minute
	statTimeout  = 5 * time.Second
	containerUID = 65532
)

var setupLinkRe = regexp.MustCompile(regexp.QuoteMeta(publicURL) + `/setup#([A-Za-z0-9_-]{43})`)

type Limits struct {
	Server string
	DB     string
}

func (l Limits) String() string { return "server " + l.Server + " / postgres " + l.DB }

type Stack struct {
	Root     string
	Project  string
	Image    string
	DBImage  string
	Port     int
	CPUs     string
	Limits   Limits
	Socket   string
	Work     string
	keyring  string
	started  time.Time
	docker   *http.Client
	ids      map[string]string
	password string
	secret   string
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Stack) env() []string {
	env := []string{
		"COMPOSE_PROJECT_NAME=" + s.Project,
		"DHOW_IMAGE=" + s.Image,
		"DHOW_PUBLISH=127.0.0.1:" + strconv.Itoa(s.Port),
		"DHOW_PUBLIC_URL=" + publicURL,
		"POSTGRES_PASSWORD=" + s.password,
		"DHOW_SECRET_KEY=" + s.secret,
		"BENCH_KEYRING_FILE=" + s.keyring,
		"DHOW_BREACHED_PASSWORD_CHECK=false",
		"DHOW_TRUSTED_PROXIES=172.16.0.0/12,192.168.0.0/16",
		"DHOW_RATE_LIMIT_PUBLIC=100000",
		"DHOW_RATE_LIMIT_INVITES_USER=100000",
		"DHOW_RATE_LIMIT_INVITES_ORG=100000",
		"BENCH_SERVER_MEMORY=" + s.Limits.Server,
		"BENCH_DB_MEMORY=" + s.Limits.DB,
		"BENCH_SERVER_CPUS=" + s.CPUs,
		"BENCH_DB_CPUS=" + s.CPUs,
	}
	if s.DBImage != "" {
		env = append(env, "BENCH_POSTGRES_IMAGE="+s.DBImage)
	}
	return env
}

func writeKeyring(ctx context.Context, work string) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	dir := filepath.Join(work, "keyring")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create the keyring directory: %w", err)
	}
	path := filepath.Join(dir, "dhow-keyring")
	_ = os.Remove(path)
	if err := os.WriteFile(path, []byte("kek 1 "+base64.RawURLEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write the keyring: %w", err)
	}
	if os.Geteuid() == 0 {
		return path, os.Chown(path, containerUID, containerUID)
	}
	owner := strconv.Itoa(containerUID) + ":" + strconv.Itoa(containerUID)
	if out, err := exec.CommandContext(ctx, "sudo", "chown", "--", owner, path).CombinedOutput(); err != nil { //nolint:gosec // A fixed argv on a path this program made.
		return "", fmt.Errorf("give the keyring to the container user %d: %w: %s", containerUID, err, out)
	}
	return path, nil
}

func (s *Stack) compose(ctx context.Context, args ...string) ([]byte, error) {
	base := []string{
		"compose", "--project-directory", s.Root + "/deploy/compose",
		"-f", s.Root + "/deploy/compose/compose.yaml", "-f", s.Root + "/deploy/bench/compose.yaml",
	}
	cmd := exec.CommandContext(ctx, "docker", append(base, args...)...) //nolint:gosec // A fixed argv: docker compose with the bench files.
	cmd.Env = append(cmd.Environ(), s.env()...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("docker compose %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (s *Stack) Up(ctx context.Context) error {
	var err error
	if s.password, err = randomHex(24); err != nil {
		return err
	}
	if s.secret, err = randomHex(32); err != nil {
		return err
	}
	if s.keyring, err = writeKeyring(ctx, s.Work); err != nil {
		return err
	}
	s.docker = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", s.Socket)
		},
	}}
	s.Down(ctx)
	s.started = time.Now()
	upCtx, cancel := context.WithTimeout(ctx, upTimeout)
	defer cancel()
	if _, err := s.compose(upCtx, "up", "--detach", "--wait", "--quiet-pull"); err != nil {
		return err
	}
	s.ids = map[string]string{}
	for _, svc := range []string{"dhow", "postgres"} {
		out, err := s.compose(ctx, "ps", "--all", "--quiet", svc)
		if err != nil {
			return err
		}
		s.ids[svc] = strings.TrimSpace(string(out))
	}
	return nil
}

func (s *Stack) Down(ctx context.Context) {
	_, _ = s.compose(ctx, "down", "--volumes", "--remove-orphans", "--timeout", "20")
}

func (s *Stack) BaseURL() string { return "http://127.0.0.1:" + strconv.Itoa(s.Port) }

func (s *Stack) SetupToken(ctx context.Context) (string, error) {
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		out, err := s.compose(ctx, "logs", "--no-color", "--no-log-prefix", "dhow")
		if err == nil {
			if m := setupLinkRe.FindAllSubmatch(out, -1); len(m) > 0 {
				return string(m[len(m)-1][1]), nil
			}
		}
		if err := sleep(ctx, time.Second); err != nil {
			return "", err
		}
	}
	return "", errors.New("no setup link in the server log after a minute")
}

func (s *Stack) dockerGet(ctx context.Context, path string, into any) error {
	ctx, cancel := context.WithTimeout(ctx, statTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, http.NoBody)
	if err != nil {
		return err
	}
	res, err := s.docker.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("docker %s: %d %s", path, res.StatusCode, body)
	}
	return json.NewDecoder(res.Body).Decode(into)
}

type Sample struct {
	At         time.Time
	WorkingSet uint64
	RSS        uint64
	CPUNanos   uint64
}

func number(m map[string]any, path ...string) uint64 {
	var cur any = m
	for _, p := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return 0
		}
		cur = obj[p]
	}
	if f, ok := cur.(float64); ok && f > 0 {
		return uint64(f)
	}
	return 0
}

func (s *Stack) Sample(ctx context.Context, svc string) (Sample, error) {
	var st map[string]any
	if err := s.dockerGet(ctx, "/containers/"+s.ids[svc]+"/stats?stream=false&one-shot=true", &st); err != nil {
		return Sample{}, err
	}
	inactive := number(st, "memory_stats", "stats", "total_inactive_file") + number(st, "memory_stats", "stats", "inactive_file")
	rss := number(st, "memory_stats", "stats", "total_rss") + number(st, "memory_stats", "stats", "anon")
	ws := number(st, "memory_stats", "usage")
	if inactive < ws {
		ws -= inactive
	}
	return Sample{At: time.Now(), WorkingSet: ws, RSS: rss, CPUNanos: number(st, "cpu_stats", "cpu_usage", "total_usage")}, nil
}

type containerState struct {
	State struct {
		Running   bool
		OOMKilled bool
		ExitCode  int
	}
	RestartCount int
}

func (s *Stack) Health(ctx context.Context) (string, error) {
	var problems []string
	for _, svc := range []string{"dhow", "postgres"} {
		var st containerState
		if err := s.dockerGet(ctx, "/containers/"+s.ids[svc]+"/json", &st); err != nil {
			return "", err
		}
		switch {
		case st.State.OOMKilled:
			problems = append(problems, svc+" was killed for running out of memory")
		case !st.State.Running:
			problems = append(problems, fmt.Sprintf("%s stopped with exit code %d", svc, st.State.ExitCode))
		case st.RestartCount > 0:
			problems = append(problems, svc+" restarted")
		}
	}
	return strings.Join(problems, "; "), nil
}

func (s *Stack) OOMEvents(ctx context.Context) (map[string]int, error) {
	filters, err := json.Marshal(map[string][]string{"event": {"oom"}, "type": {"container"}})
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("since", strconv.FormatInt(s.started.Unix(), 10))
	q.Set("until", strconv.FormatInt(time.Now().Unix(), 10))
	q.Set("filters", string(filters))
	ctx, cancel := context.WithTimeout(ctx, statTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/events?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, err
	}
	res, err := s.docker.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	out := map[string]int{}
	dec := json.NewDecoder(res.Body)
	for {
		var ev struct {
			ID    string
			Actor struct{ ID string }
		}
		if err := dec.Decode(&ev); err != nil {
			break
		}
		for svc, id := range s.ids {
			if ev.ID == id || ev.Actor.ID == id {
				out[svc]++
			}
		}
	}
	return out, nil
}

func (s *Stack) DataBytes(ctx context.Context) (uint64, error) {
	out, err := s.compose(ctx, "exec", "-T", "postgres", "du", "-sk", "/var/lib/postgresql")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, errors.New("du printed nothing")
	}
	kb, err := strconv.ParseUint(fields[0], 10, 64)
	return kb << 10, err
}

func (s *Stack) DatabaseBytes(ctx context.Context) (uint64, error) {
	out, err := s.compose(ctx, "exec", "-T", "postgres", "psql", "-U", "dhow", "-d", "dhow", "-tAc", "SELECT pg_database_size(current_database())")
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}

func ImageBytes(ctx context.Context, image string) (uint64, error) {
	if image == "" {
		return 0, nil
	}
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Size}}", "--", image).Output() //nolint:gosec // An argv with the image name after --.
	if err != nil {
		return 0, fmt.Errorf("docker image inspect %s: %w", image, err)
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
