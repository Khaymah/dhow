package archive

import (
	"errors"
	"fmt"
	"io"
	"os"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	core "github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/cachekey"
)

func ExitCode(err error) int {
	switch {
	case err == nil:
		return sandbox.TransferExitOK
	case errors.Is(err, core.ErrMissing):
		return sandbox.TransferExitMissing
	case errors.Is(err, core.ErrUnsafe):
		return sandbox.TransferExitUnsafe
	case errors.Is(err, core.ErrTooLarge):
		return sandbox.TransferExitTooLarge
	case errors.Is(err, core.ErrUnsupported), errors.Is(err, core.ErrPath), errors.Is(err, cachekey.ErrPattern):
		return sandbox.TransferExitUnsupported
	default:
		return sandbox.TransferExitFailed
	}
}

func report(err error, errOut io.Writer) int {
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err.Error())
	}
	return ExitCode(err)
}

func RunPack(dir string, paths []string, out, errOut io.Writer) int {
	return report(core.Pack(dir, paths, out), errOut)
}

func RunUnpack(dir string, in io.Reader, limit int64, errOut io.Writer) int {
	return report(core.Unpack(in, dir, core.Limits{MaxBytes: limit}), errOut)
}

func RunHashFiles(dir string, patterns []string, out, errOut io.Writer) int {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return report(err, errOut)
	}
	defer func() { _ = root.Close() }()
	sum, err := cachekey.HashFiles(root.FS(), patterns)
	if err != nil {
		return report(err, errOut)
	}
	_, err = fmt.Fprintln(out, sum)
	return report(err, errOut)
}
