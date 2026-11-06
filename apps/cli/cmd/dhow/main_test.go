package main

import (
	"bytes"
	"errors"
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/cli/internal/App"
	"github.com/Khaymah/dhow/internal/pipelinespec/pipelinespectest"
)

const (
	dialProbeEnv    = "DHOW_CLI_TEST_DIAL_PROBE"
	requireNetnsEnv = "DHOW_CLI_REQUIRE_NETNS"
)

var updateDeps = flag.Bool("update-deps", false, "rewrite testdata/deps.linux-amd64.txt after reviewing the new packages")

var execCalls = regexp.MustCompile(`(?m)^\d+\s+execve(at)?\(`)

var networkCalls = regexp.MustCompile(`(?m)^\d+\s+(socket|socketpair|connect|bind|listen|accept4?|sendto|recvfrom|sendm?msg|recvm?msg|getsockopt|setsockopt|getsockname|getpeername|shutdown)\(`)

var (
	buildOnce sync.Once
	buildDir  string
	buildBin  string
	errBuild  error
)

func TestMain(m *testing.M) {
	if os.Getenv(dialProbeEnv) == "1" {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:9", time.Second)
		if err == nil {
			_ = conn.Close()
		}
		os.Exit(0)
	}
	code := m.Run()
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	os.Exit(code)
}

func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		buildDir, errBuild = os.MkdirTemp("", "dhow-cli-")
		if errBuild != nil {
			return
		}
		buildBin = filepath.Join(buildDir, "dhow")
		cmd := exec.Command("go", "build", "-trimpath", "-o", buildBin, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			errBuild = errors.New(string(out))
		}
	})
	require.NoError(t, errBuild)
	return buildBin
}

func TestTheCommandLinksOnlyTheAllowedPackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.Output()
	require.NoError(t, err)
	got := strings.Fields(string(out))
	slices.Sort(got)
	allowed := filepath.Join("testdata", "deps.linux-amd64.txt")
	if *updateDeps {
		require.NoError(t, os.WriteFile(allowed, []byte(strings.Join(got, "\n")+"\n"), 0o600))
	}
	raw, err := os.ReadFile(allowed)
	require.NoError(t, err)
	want := strings.Fields(string(raw))
	extra, missing := difference(got, want), difference(want, got)
	assert.Empty(t, extra, "the CLI links packages that are not in %s. Check that none of them can start a process and that "+
		"only internal/ingestpush reaches the network (ADR-0099), then run go test ./apps/cli/cmd/dhow -run Allowed -update-deps", allowed)
	assert.Empty(t, missing, "%s lists packages the CLI no longer links: run go test ./apps/cli/cmd/dhow -run Allowed -update-deps", allowed)
	for _, d := range got {
		assert.NotContains(t, []string{"net/rpc", "net/smtp", "os/exec", "plugin", "runtime/cgo"}, d)
		for _, prefix := range []string{"golang.org/x/net/", "golang.org/x/sys/", "google.golang.org/grpc", "connectrpc.com/"} {
			assert.False(t, strings.HasPrefix(d, prefix), "the CLI must not link %s", d)
		}
	}
}

const pushPackage = "github.com/Khaymah/dhow/internal/ingestpush"

var riskyImports = []string{"net", "crypto/tls", "crypto/x509", "syscall", "unsafe", "os/exec"}

var riskyException = map[string][]string{
	"cel.dev/expr": {"unsafe"},
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1": {"unsafe"},
	"google.golang.org/genproto/googleapis/api/expr/v1alpha1":      {"unsafe"},
	"google.golang.org/protobuf/internal/impl":                     {"unsafe"},
	"google.golang.org/protobuf/internal/protolazy":                {"unsafe"},
	"google.golang.org/protobuf/internal/strs":                     {"unsafe"},
	"google.golang.org/protobuf/reflect/protoreflect":              {"unsafe"},
	"google.golang.org/protobuf/types/descriptorpb":                {"unsafe"},
	"google.golang.org/protobuf/types/gofeaturespb":                {"unsafe"},
	"google.golang.org/protobuf/types/known/anypb":                 {"unsafe"},
	"google.golang.org/protobuf/types/known/durationpb":            {"unsafe"},
	"google.golang.org/protobuf/types/known/emptypb":               {"unsafe"},
	"google.golang.org/protobuf/types/known/structpb":              {"unsafe"},
	"google.golang.org/protobuf/types/known/timestamppb":           {"unsafe"},
	"google.golang.org/protobuf/types/known/wrapperspb":            {"unsafe"},
	"github.com/antlr4-go/antlr/v4":                                {"syscall"},
	"github.com/Khaymah/dhow/internal/safefile":                    {"syscall"},
}

func risky(imp string) bool {
	return slices.Contains(riskyImports, imp) || strings.HasPrefix(imp, "net/http") || strings.HasPrefix(imp, "golang.org/x/sys/")
}

func TestOnlyThePushClientImportsTheNetworkPackages(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", ".")
			cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
			out, err := cmd.Output()
			require.NoError(t, err)
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 0 || fields[0] == pushPackage {
					continue
				}
				first, _, _ := strings.Cut(fields[0], "/")
				if !strings.Contains(first, ".") {
					continue
				}
				for _, imp := range fields[1:] {
					if risky(imp) && !slices.Contains(riskyException[fields[0]], imp) {
						assert.Fail(t, "forbidden import", "%s imports %s: only %s may talk to the network or the system (ADR-0099)", fields[0], imp, pushPackage)
					}
				}
			}
		})
	}
}

func TestOnlyThePushCommandImportsThePushClient(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	err := filepath.WalkDir(filepath.Join(root, "apps", "cli"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), pushPackage) {
			assert.True(t, strings.HasPrefix(filepath.Base(path), "Push") && filepath.Base(filepath.Dir(path)) == "App", "%s imports %s", path, pushPackage)
		}
		return nil
	})
	require.NoError(t, err)
}

func difference(a, b []string) []string {
	out := []string{}
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

func TestTheBinaryHoldsNoProcessSymbols(t *testing.T) {
	symbols, err := exec.Command("go", "tool", "nm", binary(t)).Output()
	require.NoError(t, err)
	require.Contains(t, string(symbols), " main.main")
	prefixes := []string{
		"os/exec.", "os.StartProcess", "os.startProcess", "syscall.ForkExec", "syscall.forkExec", "syscall.Exec", "syscall.StartProcess",
	}
	for _, line := range strings.Split(string(symbols), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name := fields[len(fields)-1]
		bad := false
		for _, p := range prefixes {
			bad = bad || strings.HasPrefix(name, p)
		}
		assert.False(t, bad, "the binary holds the network or process symbol %s", name)
	}
}

func isolation(t *testing.T) []string {
	t.Helper()
	candidates := [][]string{{"unshare", "--net"}, {"unshare", "--user", "--map-root-user", "--net"}}
	if os.Geteuid() != 0 {
		candidates = candidates[1:]
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err != nil {
			break
		}
		probe := append(append([]string{}, c...), "true")
		if exec.Command(probe[0], probe[1:]...).Run() == nil {
			return c
		}
	}
	if os.Getenv(requireNetnsEnv) == "1" {
		t.Fatalf("%s=1 but unshare cannot create a network namespace here", requireNetnsEnv)
	}
	t.Skip("unshare cannot create a network namespace here")
	return nil
}

func tracer(t *testing.T, traceFile string) []string {
	t.Helper()
	if _, err := exec.LookPath("strace"); err != nil {
		if os.Getenv(requireNetnsEnv) == "1" {
			t.Fatalf("%s=1 but strace is not installed", requireNetnsEnv)
		}
		return nil
	}
	return []string{"strace", "-f", "-qq", "-e", "trace=%network,execve,execveat", "-e", "signal=none", "-o", traceFile}
}

func runIsolated(t *testing.T, prefix []string, env []string, args ...string) (string, int) {
	t.Helper()
	full := append(append([]string{}, prefix...), args...)
	cmd := exec.Command(full[0], full[1:]...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "LANG=C"}, env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else {
		require.NoError(t, err)
	}
	return out.String(), code
}

func TestTheNamespaceHasNoNetwork(t *testing.T) {
	prefix := isolation(t)
	out, code := runIsolated(t, prefix, nil, "cat", "/proc/self/net/dev")
	require.Zero(t, code)
	var interfaces []string
	for _, line := range strings.Split(out, "\n") {
		if name, _, ok := strings.Cut(line, ":"); ok {
			interfaces = append(interfaces, strings.TrimSpace(name))
		}
	}
	assert.Equal(t, []string{"lo"}, interfaces)
}

func TestTheTracerSeesANetworkAttempt(t *testing.T) {
	prefix := isolation(t)
	trace := filepath.Join(t.TempDir(), "trace")
	strace := tracer(t, trace)
	if strace == nil {
		t.Skip("strace is not installed")
	}
	self, err := os.Executable()
	require.NoError(t, err)
	_, code := runIsolated(t, append(prefix, strace...), []string{dialProbeEnv + "=1"}, self)
	require.Zero(t, code)
	got, err := os.ReadFile(trace)
	require.NoError(t, err)
	assert.NotEmpty(t, networkCalls.FindAllString(string(got), -1), "%s", got)
}

func TestLintInANetworkNamespaceMakesNoNetworkCall(t *testing.T) {
	prefix := isolation(t)
	bin := binary(t)
	dir := t.TempDir()
	cases := pipelinespectest.Invalid()
	cases = append(cases, pipelinespectest.Case{Name: "valid", Source: pipelinespectest.RunJob("")})
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.Name, " ", "-")+".yml")
			require.NoError(t, os.WriteFile(path, []byte(tc.Source), 0o600))
			trace := path + ".trace"
			strace := tracer(t, trace)
			args := []string{"lint", "--format", "json", "--lang", "en", path}
			out, code := runIsolated(t, append(append(prefix, strace...), bin), nil, args...)
			var want, wantErr bytes.Buffer
			wantCode := app.Main(args, app.Env{Getenv: func(string) string { return "" }, Out: &want, Err: &wantErr})
			assert.Equal(t, wantCode, code)
			assert.Equal(t, want.String(), out)
			if strace != nil {
				got, err := os.ReadFile(trace)
				require.NoError(t, err)
				assert.Empty(t, networkCalls.FindAllString(string(got), -1), "dhow lint made network system calls")
				execs := execCalls.FindAllString(string(got), -1)
				assert.Len(t, execs, 1, "dhow lint started another program: %s", got)
				assert.Contains(t, string(got), "execve(\""+bin+"\"")
			}
		})
	}
}

func TestAPushDryRunInANetworkNamespaceMakesNoNetworkCall(t *testing.T) {
	prefix := isolation(t)
	bin := binary(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o600))
	trace := filepath.Join(t.TempDir(), "trace")
	strace := tracer(t, trace)
	args := []string{
		"artifact", "push", "--dry-run", "--format", "json", "--server", "https://dhow.example.com",
		"--name", "web", "--ref", "main", "--sha", strings.Repeat("a", 40), "--", dir,
	}
	out, code := runIsolated(t, append(append(prefix, strace...), bin), []string{"HTTPS_PROXY=http://127.0.0.1:9"}, args...)
	assert.Zero(t, code)
	assert.Contains(t, out, `"DryRun":true`)
	if strace != nil {
		got, err := os.ReadFile(trace)
		require.NoError(t, err)
		assert.Empty(t, networkCalls.FindAllString(string(got), -1), "dhow artifact push --dry-run made network system calls")
		assert.Len(t, execCalls.FindAllString(string(got), -1), 1, "dhow artifact push started another program: %s", got)
	}
}
