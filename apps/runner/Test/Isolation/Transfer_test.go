//go:build isolation

package isolation_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	archive "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Archive"
)

func craftArchive(t *testing.T, entries []tar.Header, bodies map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range entries {
		body := bodies[h.Name]
		h.Size = int64(len(body))
		if h.Typeflag != tar.TypeReg {
			h.Size = 0
		}
		h.Mode = 0o644
		require.NoError(t, tw.WriteHeader(&h))
		if h.Typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func transfer(t *testing.T, box sandbox.Sandbox, argv []string, stdin []byte) output {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	step := sandbox.StepSpec{Index: sandbox.TransferStepIndex, Argv: argv}
	if stdin != nil {
		step.Stdin = bytes.NewReader(stdin)
	}
	return runCtx(ctx, t, box, step)
}

func hostEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestAnArtifactIsUnpackedInsideTheJobsNamespaceAndAnUnsafeOneIsRefused(t *testing.T) {
	a := newAttempt(t)
	before := hostEntries(t, filepath.Dir(a.dir))
	box := a.sandbox(t, sandbox.KindTransfer, image)

	evil := craftArchive(t, []tar.Header{
		{Name: "ok.txt", Typeflag: tar.TypeReg},
		{Name: "../../../../escape", Typeflag: tar.TypeReg},
	}, map[string]string{"ok.txt": "fine", "../../../../escape": "pwned"})
	out := transfer(t, box, []string{sandbox.AgentPath, sandbox.UnpackCommand}, evil)
	assert.Equal(t, int32(sandbox.TransferExitUnsafe), out.result.ExitCode, out.stderr)
	link := craftArchive(t, []tar.Header{{Name: "etc", Typeflag: tar.TypeSymlink, Linkname: "/etc"}, {Name: "etc/passwd", Typeflag: tar.TypeReg}}, nil)
	out = transfer(t, box, []string{sandbox.AgentPath, sandbox.UnpackCommand}, link)
	assert.Equal(t, int32(sandbox.TransferExitUnsafe), out.result.ExitCode, out.stderr)
	device := craftArchive(t, []tar.Header{{Name: "null", Typeflag: tar.TypeChar, Devmajor: 1, Devminor: 3}}, nil)
	out = transfer(t, box, []string{sandbox.AgentPath, sandbox.UnpackCommand}, device)
	assert.Equal(t, int32(sandbox.TransferExitUnsafe), out.result.ExitCode, out.stderr)

	var good bytes.Buffer
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "dist", "app.js"), []byte("built in the first job\n"), 0o644))
	require.NoError(t, archive.Pack(src, []string{"dist"}, &good))
	out = transfer(t, box, []string{sandbox.AgentPath, sandbox.UnpackCommand}, good.Bytes())
	require.Zero(t, out.result.ExitCode, out.stderr)
	destroy(t, box)

	job := a.sandbox(t, sandbox.KindJob, image)
	read := run(t, job, busybox+" cat /workspace/dist/app.js && "+busybox+" stat -c '%u:%g' /workspace/dist/app.js && "+
		busybox+" test ! -e /workspace/escape && "+busybox+" test ! -e /escape\n", nil)
	require.Zero(t, read.result.ExitCode, read.stderr)
	assert.Contains(t, read.stdout, "built in the first job\n")
	assert.Contains(t, read.stdout, "0:0\n", "the files belong to the job's root, not to the runner user")
	destroy(t, job)
	assert.ElementsMatch(t, before, hostEntries(t, filepath.Dir(a.dir)), "nothing was written next to the attempt on the host")
	_, err := os.Stat(filepath.Join(filepath.Dir(a.dir), "escape"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestAnArtifactPackedInTheSandboxIsTheSameEachTime(t *testing.T) {
	a := newAttempt(t)
	job := a.sandbox(t, sandbox.KindJob, image)
	made := run(t, job, busybox+" mkdir -p /workspace/dist/sub && echo a > /workspace/dist/a && echo b > /workspace/dist/sub/b && "+
		busybox+" ln -s a /workspace/dist/link\n", nil)
	require.Zero(t, made.result.ExitCode, made.stderr)
	destroy(t, job)

	box := a.sandbox(t, sandbox.KindTransfer, image)
	first := transfer(t, box, []string{sandbox.AgentPath, sandbox.PackCommand, "--", "dist"}, nil)
	require.Zero(t, first.result.ExitCode, first.stderr)
	second := transfer(t, box, []string{sandbox.AgentPath, sandbox.PackCommand, "--", "dist"}, nil)
	require.Zero(t, second.result.ExitCode, second.stderr)
	assert.Equal(t, sha256.Sum256([]byte(first.stdout)), sha256.Sum256([]byte(second.stdout)))
	dst := t.TempDir()
	require.NoError(t, archive.Unpack(bytes.NewReader([]byte(first.stdout)), dst, archive.Limits{}))
	got, err := os.ReadFile(filepath.Join(dst, "dist", "sub", "b"))
	require.NoError(t, err)
	assert.Equal(t, "b\n", string(got))
	missing := transfer(t, box, []string{sandbox.AgentPath, sandbox.PackCommand, "--", "nope"}, nil)
	assert.Equal(t, int32(sandbox.TransferExitMissing), missing.result.ExitCode)
	sum := transfer(t, box, []string{sandbox.AgentPath, sandbox.HashFilesCommand, "--", "dist/**"}, nil)
	require.Zero(t, sum.result.ExitCode, sum.stderr)
	assert.Len(t, bytes.TrimSpace([]byte(sum.stdout)), 64)
	destroy(t, box)
}
