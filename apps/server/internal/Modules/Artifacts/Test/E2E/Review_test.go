package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestARunnerCannotClaimABlobOfAnotherOrganization(t *testing.T) {
	a := newWorld(t)
	b := a.tenant(t)
	secret := []byte("organization B's build output")

	b.push(t, "refs/heads/main", shaA)
	bg := b.next(t)
	require.Equal(t, http.StatusCreated, b.runner.PutBlob(bg.Ref, "artifact", "web", secret).Status)

	a.push(t, "refs/heads/main", shaA)
	ag := a.next(t)
	claimed := reason(a.runner.CommitArtifact(ag.Ref, "web", secret))
	missing := reason(a.runner.CommitArtifact(ag.Ref, "web", []byte("a digest nobody uploaded")))
	assert.Equal(t, "BLOB_NOT_FOUND", claimed, "another organization's blob is refused")
	assert.Equal(t, missing, claimed, "the answer does not tell whether the blob exists elsewhere")
	_, err := a.runner.CommitCache(ag.Ref, cacheKey, secret)
	assert.Equal(t, missing, reason(err))

	own := []byte("organization A's own output")
	require.Equal(t, http.StatusCreated, a.runner.PutBlob(ag.Ref, "artifact", "web", own).Status)
	_, err = a.runner.Artifacts().Commit(context.Background(), connect.NewRequest(&runnerv1.ArtifactServiceCommitRequest{
		Lease: ag.Ref, Name: "web", Digest: apptest.DigestHex(own), SizeBytes: int64(len(own)) + 1,
	}))
	wrong := reason(err)
	assert.Equal(t, missing, wrong, "a wrong size answers like a missing blob")
	require.NoError(t, a.runner.CommitArtifact(ag.Ref, "web", own))
}

func TestEachDeclaredUploadGetsOnePutPerLease(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	g := w.next(t)
	bad := w.runner.PutBlobAs(g.Ref, "artifact", "web", apptest.DigestHex([]byte("other")), archiveBytes)
	require.Equal(t, "DIGEST_MISMATCH", bad.Code)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "artifact", "web", archiveBytes).Status, "a failed upload may be tried again")
	again := w.runner.PutBlob(g.Ref, "artifact", "web", []byte("a second web archive"))
	assert.Equal(t, http.StatusConflict, again.Status)
	assert.Equal(t, "UPLOAD_ALREADY_USED", again.Code)
	for _, name := range []string{"", "secrets"} {
		res := w.runner.PutBlob(g.Ref, "artifact", name, []byte("not declared"))
		assert.Equal(t, "NOT_DECLARED", res.Code, name)
	}
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "cache", "", []byte("deps")).Status)
	assert.Equal(t, "UPLOAD_ALREADY_USED", w.runner.PutBlob(g.Ref, "cache", "", []byte("more deps")).Code)
}

func TestAnOrganizationStaysUnderItsArtifactQuota(t *testing.T) {
	w := newWorldWith(t, func(c *config.Config) { c.Artifacts.OrgMaxBytes = 100 })
	first, second := sized(60, 1), sized(60, 2)
	w.push(t, "refs/heads/main", shaA)
	g1 := w.next(t)
	w.push(t, "refs/heads/feature", shaB)
	g2 := w.next(t)
	if g2.Grant.GetSpec().GetArtifacts() == nil {
		w.succeed(t, g2)
		g2 = w.next(t)
	}
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g1.Ref, "artifact", "web", first).Status)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g2.Ref, "artifact", "web", second).Status, "both fit before either is recorded")
	require.NoError(t, w.runner.CommitArtifact(g1.Ref, "web", first))
	assert.Equal(t, "ARTIFACT_QUOTA_EXCEEDED", reason(w.runner.CommitArtifact(g2.Ref, "web", second)), "the commit counts what is recorded")

	w.push(t, "refs/heads/other", shaC)
	g3 := w.next(t)
	for g3.Grant.GetSpec().GetArtifacts() == nil {
		w.succeed(t, g3)
		g3 = w.next(t)
	}
	res := w.runner.PutBlob(g3.Ref, "artifact", "web", sized(60, 3))
	assert.Equal(t, http.StatusRequestEntityTooLarge, res.Status)
	assert.Equal(t, "ARTIFACT_QUOTA_EXCEEDED", res.Code, "an upload that cannot fit is refused before it is read")
}

func TestArtifactsOfADeletedRunAreNotServed(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	a := w.env.SeedArtifact(w.project, "web", []byte("archive"), false)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "PipelineRuns" SET "IsDeleted" = true WHERE "Id" = $1`, a.RunID)
	require.NoError(t, err)
	res := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/artifacts/"+a.ID.String()+"/download"))
	assert.Equal(t, http.StatusNotFound, res.Status, string(res.Raw))
	list := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/runs/"+a.RunID.String()+"/artifacts"))
	assert.Equal(t, http.StatusNotFound, list.Status)
}

func TestForkRunsShareATenthOfTheQuotaOfTheirProject(t *testing.T) {
	w := newWorldWith(t, func(c *config.Config) { c.Artifacts.OrgMaxBytes = 1000 })
	w.pullRequest(t, 12, shaB, triggers.PullOriginFork)
	fork := w.next(t)
	res := w.runner.PutBlob(fork.Ref, "artifact", "web", sized(150, 1))
	assert.Equal(t, http.StatusRequestEntityTooLarge, res.Status, "a fork run gets a tenth of the quota, not all of it")
	assert.Equal(t, "ARTIFACT_QUOTA_EXCEEDED", res.Code)
	small := sized(60, 2)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(fork.Ref, "artifact", "web", small).Status)
	require.NoError(t, w.runner.CommitArtifact(fork.Ref, "web", small))

	w.pullRequest(t, 13, shaC, triggers.PullOriginFork)
	other := w.next(t)
	res = w.runner.PutBlob(other.Ref, "artifact", "web", sized(60, 3))
	assert.Equal(t, http.StatusRequestEntityTooLarge, res.Status)
	assert.Equal(t, "ARTIFACT_QUOTA_EXCEEDED", res.Code, "the share counts every fork run of the project")

	w.push(t, "refs/heads/main", shaA)
	own := w.next(t)
	for own.Grant.GetSpec().GetArtifacts() == nil || own.Grant.GetSpec().GetCache().GetReadOnly() {
		w.succeed(t, own)
		own = w.next(t)
	}
	big := sized(800, 4)
	put := w.runner.PutBlob(own.Ref, "artifact", "web", big)
	require.Equal(t, http.StatusCreated, put.Status, put.Code)
	assert.NoError(t, w.runner.CommitArtifact(own.Ref, "web", big), "forks never eat the quota of the project's own runs")
}
