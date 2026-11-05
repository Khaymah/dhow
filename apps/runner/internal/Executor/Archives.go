package executor

import (
	"os"
	"path/filepath"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (a *attempt) keepArchive(in *runnerv1.ArtifactInput, path string) error {
	kept := filepath.Join(a.dir, "archive-"+in.GetName())
	if err := os.Rename(path, kept); err != nil {
		_ = os.Remove(path)
		return err
	}
	if a.archives == nil {
		a.archives = map[string]archive{}
	}
	a.archives[in.GetName()] = archive{path: kept, digest: in.GetDigest()}
	return nil
}
