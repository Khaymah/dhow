package checkout

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Khaymah/dhow/internal/gitsource"
)

func hostKeyFailures() []string {
	return []string{"Host key verification failed", "REMOTE HOST IDENTIFICATION HAS CHANGED"}
}

type HelperOptions struct {
	Getenv     func(string) string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	SSH        string
	SecretsDir string
	Connect    string
}

func (o *HelperOptions) defaults() {
	if o.SSH == "" {
		o.SSH = SSHBinary
	}
	if o.SecretsDir == "" {
		o.SecretsDir = SecretsDir
	}
	if o.Connect == "" {
		o.Connect = ConnectHelper
	}
}

func RunSSH(ctx context.Context, args []string, o HelperOptions) int {
	o.defaults()
	u, err := gitsource.Parse(o.Getenv(envGitURL))
	if err != nil {
		_, _ = fmt.Fprintln(o.Stderr, "dhow: git-ssh: no repository address")
		return ExitFailed
	}
	command, err := gitsource.ParseSSHInvocation(u, args)
	if err != nil {
		_, _ = fmt.Fprintf(o.Stderr, "dhow: git-ssh: %v\n", err)
		return ExitFailed
	}
	argv, err := gitsource.SSHArgs(u, gitsource.SSHOptions{
		Binary: o.SSH, KeyFile: filepath.Join(o.SecretsDir, CredentialFile),
		KnownHostsFile: filepath.Join(o.SecretsDir, KnownHostsFile), ProxyCommand: o.Connect,
	}, command)
	if err != nil {
		_, _ = fmt.Fprintf(o.Stderr, "dhow: git-ssh: %v\n", err)
		return ExitFailed
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // gitsource builds a fixed ssh argv for the one repository host.
	cmd.Env = []string{"PATH=" + gitsource.FixedPath, "HOME=" + o.Getenv("HOME"), "LC_ALL=C"}
	var errb bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, io.MultiWriter(o.Stderr, &errb)
	runErr := cmd.Run()
	if markHostKey(errb.Bytes(), o.Getenv(envMarkers), o.Stderr) {
		return ExitHostKeyMismatch
	}
	var ee *exec.ExitError
	switch {
	case runErr == nil:
		return ExitOK
	case errors.As(runErr, &ee):
		return ee.ExitCode()
	default:
		return ExitFailed
	}
}

func hostKeyFailed(stderr []byte) bool {
	sc := bufio.NewScanner(bytes.NewReader(stderr))
	for sc.Scan() {
		for _, f := range hostKeyFailures() {
			if strings.Contains(sc.Text(), f) {
				return true
			}
		}
	}
	return false
}

func markHostKey(stderr []byte, dir string, errOut io.Writer) bool {
	if !hostKeyFailed(stderr) {
		return false
	}
	if dir == "" || !filepath.IsAbs(dir) {
		_, _ = fmt.Fprintln(errOut, "dhow: HOST_KEY_MISMATCH: the checkout has no marker directory to record it")
		return true
	}
	if err := os.WriteFile(filepath.Join(dir, hostKeyMarker), nil, 0o600); err != nil {
		_, _ = fmt.Fprintf(errOut, "dhow: HOST_KEY_MISMATCH: could not record it (%v)\n", err)
	}
	return true
}

func RunAskpass(args []string, o HelperOptions) int {
	o.defaults()
	if len(args) != 1 {
		return ExitFailed
	}
	switch {
	case strings.HasPrefix(args[0], "Username"):
		_, _ = fmt.Fprintln(o.Stdout, gitsource.CheckoutUser)
		return ExitOK
	case strings.HasPrefix(args[0], "Password"):
		raw, err := os.ReadFile(filepath.Join(o.SecretsDir, CredentialFile))
		if err != nil {
			return ExitFailed
		}
		defer clear(raw)
		token := bytes.TrimRight(raw, "\r\n")
		if len(token) == 0 || bytes.ContainsAny(token, "\r\n") {
			return ExitFailed
		}
		_, _ = o.Stdout.Write(append(token, '\n'))
		return ExitOK
	default:
		return ExitFailed
	}
}
