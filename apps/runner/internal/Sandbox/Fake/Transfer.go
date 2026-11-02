//go:build dhowtest

package fake

import (
	"io"
	"os"
	"path/filepath"
	"strconv"

	archive "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Archive"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

func (d *Driver) WorkspaceDir(attempt string) string {
	if d.root == "" || attempt == "" {
		return ""
	}
	return filepath.Join(d.root, attempt)
}

func (d *Driver) seedWorkspace(attempt string) error {
	dir := d.WorkspaceDir(attempt)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return writeFiles(dir, d.rec.Workspace)
}

func writeFiles(dir string, files map[string]string) error {
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil { //nolint:gosec // Test files of the fake workspace.
			return err
		}
	}
	return nil
}

func transfer(argv []string) (string, []string, bool) {
	if len(argv) < 2 || argv[0] != sandbox.AgentPath {
		return "", nil, false
	}
	switch argv[1] {
	case sandbox.PackCommand, sandbox.UnpackCommand, sandbox.HashFilesCommand:
		args := argv[2:]
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		return argv[1], args, true
	default:
		return "", nil, false
	}
}

func (b *box) runTransfer(spec sandbox.StepSpec, cmd string, args []string) sandbox.StepResult {
	dir := b.d.WorkspaceDir(b.attempt)
	stderr := spec.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if dir == "" {
		return sandbox.StepResult{ExitCode: sandbox.TransferExitFailed}
	}
	var code int
	switch cmd {
	case sandbox.PackCommand:
		code = archive.RunPack(dir, args, spec.Stdout, stderr)
	case sandbox.UnpackCommand:
		limit, _ := strconv.ParseInt(spec.Env[sandbox.UnpackMaxEnv], 10, 64)
		in := spec.Stdin
		if in == nil {
			in = io.LimitReader(nil, 0)
		}
		code = archive.RunUnpack(dir, in, limit, stderr)
	default:
		code = archive.RunHashFiles(dir, args, spec.Stdout, stderr)
	}
	return sandbox.StepResult{ExitCode: int32(code)} //nolint:gosec // Exit codes are below 256.
}
