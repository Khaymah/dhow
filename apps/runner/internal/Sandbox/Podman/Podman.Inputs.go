package podman

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	inputsDirName = "inputs"
	inputsMount   = sandbox.InputsPath
	inputFileMode = 0o400
)

var ErrInputName = errors.New("an input file has a name that is not a safe artifact name")

func inputsDirOf(dir string, seq int) string {
	if seq == 0 {
		return filepath.Join(dir, inputsDirName)
	}
	return filepath.Join(dir, inputsDirName+"-"+strconv.Itoa(seq))
}

func stageInputs(dir string, seq int, files []sandbox.InputFile) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	target := inputsDirOf(dir, seq)
	if err := config.EnsurePrivateDir(target); err != nil {
		return "", err
	}
	for _, f := range files {
		if !jobspec.ValidArtifactName(f.Name) || f.Path == "" {
			return "", ErrInputName
		}
		dst := filepath.Join(target, f.Name)
		if err := os.Link(f.Path, dst); err != nil {
			if err := copyInput(f.Path, dst); err != nil {
				return "", fmt.Errorf("stage the input %s: %w", f.Name, err)
			}
		}
		if err := os.Chmod(dst, inputFileMode); err != nil {
			return "", fmt.Errorf("lock down the input %s: %w", f.Name, err)
		}
	}
	return target, nil
}

func copyInput(src, dst string) (err error) {
	in, err := os.Open(src) //nolint:gosec // A file this attempt downloaded into its own directory.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, config.PrivateFileMode) //nolint:gosec // The attempt directory of this runner.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}
