package e2e_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	"github.com/Khaymah/dhow/internal/gitproof"
	"github.com/Khaymah/dhow/internal/gitproof/gitprooftest"
)

const forgedPipeline = `version: 1
on:
  push: {}
jobs:
  steal:
    image: IMAGE
    steps:
      - run: make steal
`

func selfVouched(source string) (string, map[string]string) {
	sum := sha256.Sum256([]byte(source))
	encoded := base64.StdEncoding.EncodeToString([]byte(source))
	var b strings.Builder
	for len(encoded) > 0 {
		n := min(76, len(encoded))
		b.WriteString("dhow-definition:" + encoded[:n] + "\n")
		encoded = encoded[n:]
	}
	return b.String(), map[string]string{"dhow_found": "true", "dhow_sha256": hex.EncodeToString(sum[:])}
}

func (g gitWorld) discoverRun(t *testing.T, sha, log string, outputs map[string]string) []string {
	t.Helper()
	run := g.env.Deliver(g.trigger(triggers.EventPush, "refs/heads/case-"+uuid.NewString()[:8], sha)).RunID
	g.env.StartRuns()
	_, ok := g.runner.DiscoverWith(log, outputs)
	require.True(t, ok)
	g.env.StartRuns()
	state, reason := g.env.RunState(run)
	return []string{state, reason}
}

func replaced(objects []gitproof.Object, i int, o gitproof.Object) []gitproof.Object {
	out := append([]gitproof.Object(nil), objects...)
	out[i] = o
	return out
}

func TestARunnerCannotHandOverAFileThatIsNotInTheCommit(t *testing.T) {
	g := newGitWorld(t)
	honest := apptest.DiscoveryCommit(pipeline(outputsPipeline))
	forged := apptest.DiscoveryCommit(pipeline(forgedPipeline))
	symlink := gitprooftest.WithEntry(gitproof.SHA1, ".dhow.yml", gitproof.ModeSymlink, []byte("/etc/passwd"))
	huge := apptest.DiscoveryCommit(pipeline(outputsPipeline) + "#" + strings.Repeat("a", 256<<10) + "\n")
	vouched, outputs := selfVouched(pipeline(forgedPipeline))
	for _, tc := range []struct {
		name    string
		sha     string
		objects []gitproof.Object
	}{
		{"forged file", honest.Sha, replaced(honest.Objects, 2, gitproof.Object{Kind: gitproof.KindBlob, Data: []byte(pipeline(forgedPipeline))})},
		{"forged tree", honest.Sha, replaced(honest.Objects, 1, forged.Objects[1])},
		{"commit of another sha", honest.Sha, forged.Objects},
		{"symlink entry", symlink.Sha, append(symlink.Objects, gitproof.Object{Kind: gitproof.KindBlob, Data: []byte(pipeline(forgedPipeline))})},
		{"oversized file", huge.Sha, huge.Objects},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := g.discoverRun(t, tc.sha, gitprooftest.Log(tc.objects)+vouched, outputs)
			assert.Equal(t, []string{"failed", "DEFINITION_UNVERIFIED"}, got)
		})
	}
	assert.Zero(t, g.env.CountRows(`SELECT count(*) FROM "RunJobs" WHERE "Key" = 'steal'`), "no job of a forged file is ever planned")
}

func TestADiscoveryClaimMustMatchWhatTheObjectsProve(t *testing.T) {
	g := newGitWorld(t)
	honest := apptest.DiscoveryCommit(pipeline(outputsPipeline))
	got := g.discoverRun(t, honest.Sha, gitprooftest.Log(honest.Objects), map[string]string{"dhow_found": "false"})
	assert.Equal(t, []string{"failed", "DEFINITION_UNVERIFIED"}, got, "a file that exists cannot be reported missing")

	symlink := gitprooftest.WithEntry(gitproof.SHA1, ".dhow.yml", gitproof.ModeSymlink, []byte("/etc/passwd"))
	got = g.discoverRun(t, symlink.Sha, gitprooftest.Log(symlink.Objects), map[string]string{"dhow_found": "false"})
	assert.Equal(t, []string{"skipped", "DEFINITION_NOT_FOUND"}, got, "a symlink is never followed")
}
