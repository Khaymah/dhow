package edge_test

import (
	"testing"

	"github.com/Khaymah/dhow/internal/gitproof"
	"github.com/Khaymah/dhow/internal/gitproof/gitprooftest"
	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const fuzzSource = "version: 1\n"

func FuzzDefinitionProof(f *testing.F) {
	honest := gitprooftest.File(gitproof.SHA1, "ci/pipeline.yml", fuzzSource)
	f.Add([]byte(gitprooftest.Log(honest.Objects)), "ci/pipeline.yml", false)
	f.Add([]byte(gitprooftest.Log(gitprooftest.File(gitproof.SHA256, ".dhow.yml", fuzzSource).Objects)), ".dhow.yml", true)
	f.Add([]byte(gitprooftest.Log(gitprooftest.Empty(gitproof.SHA1).Objects)), ".dhow.yml", true)
	f.Add([]byte("dhow-object:commit\ndhow-data:dHJlZSA=\n"), ".dhow.yml", true)
	f.Fuzz(func(t *testing.T, log []byte, path string, selfSigned bool) {
		objects, err := gitproof.ParseLog(log)
		if err != nil {
			return
		}
		sha := honest.Sha
		if selfSigned && len(objects) > 0 {
			format := gitproof.SHA1
			if len(log)%2 == 1 {
				format = gitproof.SHA256
			}
			sha = format.ID(gitproof.KindCommit, objects[0].Data)
		}
		proof, err := gitproof.Prove(sha, path, objects, pipelinespec.MaxFileBytes)
		switch {
		case err != nil && proof.Found:
			t.Fatalf("a refused proof still reports a file: %v", err)
		case proof.Found && len(proof.Source) > pipelinespec.MaxFileBytes:
			t.Fatalf("a proven file is larger than the limit: %d", len(proof.Source))
		case proof.Found && sha == honest.Sha && (path != "ci/pipeline.yml" || string(proof.Source) != fuzzSource):
			t.Fatalf("a proof for the known commit gave another file: %q at %q", proof.Source, path)
		case proof.Found && len(objects) < 3:
			t.Fatalf("a file was proven without a commit, a tree and a blob")
		}
	})
}
