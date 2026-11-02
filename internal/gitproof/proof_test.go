package gitproof_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/gitproof"
	"github.com/Khaymah/dhow/internal/gitproof/gitprooftest"
)

const maxFile = 256 << 10

type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T, format string) repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "--object-format="+format, "-b", "main")
	return r
}

func (r repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.CommandContext(r.t.Context(), "git", append([]string{"-c", "user.name=Dhow", "-c", "user.email=dhow@example.com"}, args...)...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	require.NoError(r.t, err, "%s", out)
	return strings.TrimSpace(string(out))
}

func (r repo) write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, path)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(r.t, os.WriteFile(full, []byte(content), 0o600))
}

func (r repo) commit() string {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", "change")
	return r.git("rev-parse", "HEAD")
}

func (r repo) discover(path string) ([]gitproof.Object, string) {
	r.t.Helper()
	output := filepath.Join(r.t.TempDir(), "output")
	cmd := exec.CommandContext(r.t.Context(), "sh")
	cmd.Dir = r.dir
	cmd.Stdin = strings.NewReader(gitproof.Script)
	cmd.Env = append(os.Environ(), gitproof.EnvPath+"="+path, "DHOW_OUTPUT="+output)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(r.t, cmd.Run(), "%s", stderr.String())
	objects, err := gitproof.ParseLog(stdout.Bytes())
	require.NoError(r.t, err)
	claim, err := os.ReadFile(output)
	require.NoError(r.t, err)
	return objects, strings.TrimSpace(string(claim))
}

func TestTheScriptProvesTheFileOfARealRepository(t *testing.T) {
	for _, tc := range []struct{ format, path string }{
		{"sha1", ".dhow.yml"},
		{"sha256", ".dhow.yml"},
		{"sha1", "ci/deploy/pipeline.yaml"},
		{"sha256", "ci/deploy/pipeline.yaml"},
	} {
		t.Run(tc.format+" "+tc.path, func(t *testing.T) {
			r := newRepo(t, tc.format)
			r.write("README", "hello\n")
			r.write("ci/other.yml", "other\n")
			r.write(tc.path, "version: 1\n")
			sha := r.commit()
			objects, claim := r.discover(tc.path)
			assert.Equal(t, "dhow_found=true", claim)
			proof, err := gitproof.Prove(sha, tc.path, objects, maxFile)
			require.NoError(t, err)
			assert.True(t, proof.Found)
			assert.Equal(t, "version: 1\n", string(proof.Source))
		})
	}
}

func TestTheScriptProvesAMissingFile(t *testing.T) {
	r := newRepo(t, "sha1")
	r.write("README", "hello\n")
	r.write("ci", "a file where a folder is expected\n")
	r.write("link-target.yml", "version: 1\n")
	require.NoError(t, os.Symlink("link-target.yml", filepath.Join(r.dir, "linked.yml")))
	sha := r.commit()
	for _, path := range []string{".dhow.yml", "ci/pipeline.yml", "nested/pipeline.yml", "linked.yml"} {
		objects, claim := r.discover(path)
		assert.Equal(t, "dhow_found=false", claim, path)
		proof, err := gitproof.Prove(sha, path, objects, maxFile)
		require.NoError(t, err, path)
		assert.False(t, proof.Found, path)
	}
}

func TestTheScriptStopsAtAFileLargerThanTheLimit(t *testing.T) {
	r := newRepo(t, "sha1")
	r.write(".dhow.yml", strings.Repeat("a", maxFile+1))
	r.commit()
	output := filepath.Join(t.TempDir(), "output")
	cmd := exec.CommandContext(t.Context(), "sh")
	cmd.Dir, cmd.Stdin = r.dir, strings.NewReader(gitproof.Script)
	cmd.Env = append(os.Environ(), gitproof.EnvPath+"=.dhow.yml", "DHOW_OUTPUT="+output)
	out, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(out), "the pipeline file is larger than")
}

func TestAForgedProofIsRefused(t *testing.T) {
	honest := gitprooftest.File(gitproof.SHA1, "ci/pipeline.yml", "version: 1\n")
	other := gitprooftest.File(gitproof.SHA1, "ci/pipeline.yml", "version: 1\njobs: {}\n")
	replace := func(i int, o gitproof.Object) []gitproof.Object {
		out := append([]gitproof.Object(nil), honest.Objects...)
		out[i] = o
		return out
	}
	huge := gitprooftest.File(gitproof.SHA1, ".dhow.yml", strings.Repeat("a", maxFile+1))
	bigTree := gitprooftest.Tree(gitproof.Entry{Mode: gitproof.ModeFile, Name: strings.Repeat("n", gitproof.MaxObjectBytes), ID: strings.Repeat("0", 40)})
	bigTreeCommit := gitprooftest.CommitBody(gitproof.SHA1.ID(gitproof.KindTree, bigTree), "big")
	symlink := gitprooftest.WithEntry(gitproof.SHA1, ".dhow.yml", gitproof.ModeSymlink, []byte("/etc/passwd"))
	for name, tc := range map[string]struct {
		sha, path string
		objects   []gitproof.Object
	}{
		"forged file":               {honest.Sha, "ci/pipeline.yml", replace(3, gitproof.Object{Kind: gitproof.KindBlob, Data: []byte("version: 1\njobs: {}\n")})},
		"forged tree":               {honest.Sha, "ci/pipeline.yml", replace(2, other.Objects[2])},
		"forged root tree":          {honest.Sha, "ci/pipeline.yml", replace(1, other.Objects[1])},
		"commit of another sha":     {honest.Sha, "ci/pipeline.yml", other.Objects},
		"object of the wrong kind":  {honest.Sha, "ci/pipeline.yml", replace(0, gitproof.Object{Kind: gitproof.KindTree, Data: honest.Objects[0].Data})},
		"objects cut short":         {honest.Sha, "ci/pipeline.yml", honest.Objects[:3]},
		"an extra object":           {honest.Sha, "ci/pipeline.yml", append(append([]gitproof.Object(nil), honest.Objects...), honest.Objects[3])},
		"symlink entry with a blob": {symlink.Sha, ".dhow.yml", append(symlink.Objects, gitproof.Object{Kind: gitproof.KindBlob, Data: []byte("/etc/passwd")})},
		"oversized file":            {huge.Sha, ".dhow.yml", huge.Objects},
		"oversized tree":            {gitproof.SHA1.ID(gitproof.KindCommit, bigTreeCommit), ".dhow.yml", []gitproof.Object{{Kind: gitproof.KindCommit, Data: bigTreeCommit}, {Kind: gitproof.KindTree, Data: bigTree}}},
		"sha256 objects for a sha1": {honest.Sha, "ci/pipeline.yml", gitprooftest.File(gitproof.SHA256, "ci/pipeline.yml", "version: 1\n").Objects},
		"uppercase sha":             {strings.ToUpper(honest.Sha), "ci/pipeline.yml", honest.Objects},
		"no objects":                {honest.Sha, "ci/pipeline.yml", nil},
	} {
		t.Run(name, func(t *testing.T) {
			proof, err := gitproof.Prove(tc.sha, tc.path, tc.objects, maxFile)
			require.ErrorIs(t, err, gitproof.ErrUnverified, "%+v", proof)
			assert.False(t, proof.Found)
		})
	}
	proof, err := gitproof.Prove(honest.Sha, "ci/pipeline.yml", honest.Objects, maxFile)
	require.NoError(t, err)
	assert.Equal(t, "version: 1\n", string(proof.Source))
	proof, err = gitproof.Prove(symlink.Sha, ".dhow.yml", symlink.Objects, maxFile)
	require.NoError(t, err)
	assert.False(t, proof.Found, "a symlink is never followed")
}

func TestAMalformedTreeIsRefused(t *testing.T) {
	id := gitproof.SHA1.ID(gitproof.KindBlob, []byte("x"))
	for name, body := range map[string][]byte{
		"repeated name": gitprooftest.Tree(gitproof.Entry{Mode: gitproof.ModeFile, Name: "a", ID: id}, gitproof.Entry{Mode: gitproof.ModeFile, Name: "a", ID: id}),
		"unknown mode":  gitprooftest.Tree(gitproof.Entry{Mode: "100666", Name: ".dhow.yml", ID: id}),
		"dot dot name":  gitprooftest.Tree(gitproof.Entry{Mode: gitproof.ModeTree, Name: "..", ID: id}),
		"cut short":     gitprooftest.Tree(gitproof.Entry{Mode: gitproof.ModeFile, Name: ".dhow.yml", ID: id})[:20],
		"no mode":       []byte(" .dhow.yml\x00" + strings.Repeat("a", 20)),
	} {
		t.Run(name, func(t *testing.T) {
			commit := gitprooftest.CommitBody(gitproof.SHA1.ID(gitproof.KindTree, body), "bad")
			_, err := gitproof.Prove(gitproof.SHA1.ID(gitproof.KindCommit, commit), ".dhow.yml",
				[]gitproof.Object{{Kind: gitproof.KindCommit, Data: commit}, {Kind: gitproof.KindTree, Data: body}}, maxFile)
			require.ErrorIs(t, err, gitproof.ErrMalformed)
		})
	}
}

func TestParseLogReadsOnlyFramedObjects(t *testing.T) {
	c := gitprooftest.File(gitproof.SHA1, ".dhow.yml", "version: 1\n")
	log := "dhow: noise\n" + strings.ReplaceAll(gitprooftest.Log(c.Objects), "\n", "\r\n") + "trailing\n"
	objects, err := gitproof.ParseLog([]byte(log))
	require.NoError(t, err)
	assert.Equal(t, c.Objects, objects)

	_, err = gitproof.ParseLog([]byte("dhow-data:AAAA\n"))
	require.ErrorIs(t, err, gitproof.ErrMalformed)
	_, err = gitproof.ParseLog([]byte("dhow-object:blob\ndhow-data:!!!!\n"))
	require.ErrorIs(t, err, gitproof.ErrMalformed)
	_, err = gitproof.ParseLog([]byte(strings.Repeat("dhow-object:tree\n", gitproof.MaxDepth+3)))
	require.ErrorIs(t, err, gitproof.ErrUnverified)
}
