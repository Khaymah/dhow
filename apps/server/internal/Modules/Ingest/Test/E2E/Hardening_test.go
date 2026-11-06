package e2e_test

import (
	"context"
	"crypto/rand"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

func (w world) plantIngested(t *testing.T, size int64, name string) {
	t.Helper()
	digest := make([]byte, 32)
	_, err := rand.Read(digest)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(context.Background(),
		`INSERT INTO "IngestedArtifacts" ("OrganizationId", "ProjectId", "TokenId", "Name", "Digest", "SizeBytes", "Ref", "RefKind", "Sha", "ExpiresAt")
		 SELECT "OrganizationId", "ProjectId", "Id", $2, $3, $4, 'refs/heads/main', 'branch', $5, now() + interval '30 days'
		 FROM "IngestTokens" WHERE "Id" = $1`, w.token.ID, name, digest, size, shaTag)
	require.NoError(t, err)
}

func (w world) revokeInTheDatabase(t *testing.T) {
	t.Helper()
	_, err := w.env.Pool.Exec(context.Background(),
		`UPDATE "IngestTokens" SET "RevokedAt" = now(), "RevokedByUserId" = $2 WHERE "Id" = $1`, w.token.ID, w.ownerID)
	require.NoError(t, err)
}

func TestATokenRevokedWhileTheUploadRunsStopsTheUpload(t *testing.T) {
	w := newWorld(t)
	body := w.archive()
	res := w.upload(body, func(u *apptest.IngestUpload) { u.AfterBody = func() { w.revokeInTheDatabase(t) } })
	assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "INGEST_TOKEN_REVOKED", res.Error.Code)
	w.assertNothingKept(t, body)
}

func TestATokenThatExpiresWhileTheUploadRunsStopsTheUpload(t *testing.T) {
	w := newWorld(t)
	body := w.archive()
	res := w.upload(body, func(u *apptest.IngestUpload) { u.AfterBody = func() { w.env.ExpireIngestToken(w.token.ID) } })
	assert.Equal(t, http.StatusUnauthorized, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "INGEST_TOKEN_EXPIRED", res.Error.Code)
	w.assertNothingKept(t, body)
}

func TestARevokedOrExpiredTokenIsAuditedOncePerMinuteWithoutItsValue(t *testing.T) {
	w := newWorld(t)
	w.revokeInTheDatabase(t)
	for range 3 {
		res := w.upload(w.archive(), nil)
		require.Equal(t, http.StatusUnauthorized, res.Status)
	}
	var data string
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT count(*), coalesce(max("Data"::text), '') FROM "AuditEvents" WHERE "Action" = 'ingest.rejected'`).Scan(&n, &data))
	assert.Equal(t, 1, n)
	assert.Contains(t, data, "INGEST_TOKEN_REVOKED")
	assert.NotContains(t, data, w.token.Value)
	assert.NotContains(t, data, config.IngestTokenPrefix)
}

func (w world) protectedProduction(t *testing.T) {
	t.Helper()
	w.env.SeedDeployTarget(w.project, "production", "prod")
	_, err := w.env.Pool.Exec(context.Background(),
		`UPDATE "Environments" SET "AllowedRefs" = '[{"Kind":"branch","Pattern":"main"}]' WHERE "ProjectId" = $1 AND "Name" = 'production'`, w.project)
	require.NoError(t, err)
}

func TestATokenThatReachesAProtectedEnvironmentNeedsAnExplicitYes(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	body := map[string]any{"Name": "wide", "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": "**"}}}
	res := w.createToken(body)
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "INGEST_TOKEN_REACH_UNCONFIRMED", res.Error.Code)
	assert.Equal(t, 1, w.countTokens(t), "only the seeded token")

	body["AcknowledgeProtectedReach"] = true
	res = w.createToken(body)
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var created struct {
		Token struct{ Reaches []string }
	}
	res.Decode(t, &created)
	assert.Equal(t, []string{"production"}, created.Token.Reaches)

	list := w.owner.Get(w.tokensPath(""))
	var items []struct {
		Name    string
		Reaches []string
	}
	list.Decode(t, &items)
	require.Len(t, items, 2)
	for _, it := range items {
		assert.Equal(t, []string{"production"}, it.Reaches, it.Name)
	}
}

func TestATokenThatCannotMeetAnyEnvironmentRefNeedsNoYes(t *testing.T) {
	w := newWorld(t)
	w.protectedProduction(t)
	res := w.createToken(map[string]any{"Name": "docs", "RefPatterns": []map[string]string{{"Kind": "branch", "Pattern": "docs/*"}}})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var created struct {
		Token struct{ Reaches []string }
	}
	res.Decode(t, &created)
	assert.Empty(t, created.Token.Reaches)

	tag := w.createToken(map[string]any{"Name": "tags", "RefPatterns": []map[string]string{{"Kind": "tag", "Pattern": "main"}}})
	assert.Equal(t, http.StatusCreated, tag.Status, "a tag named main is not the branch main")
}

func TestATokenHasAByteCap(t *testing.T) {
	w := newWorld(t)
	w.plantIngested(t, config.IngestTokenMaxBytes, "old")
	body := w.archive()
	res := w.upload(body, nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "INGEST_TOKEN_QUOTA", res.Error.Code)
	assert.Equal(t, 1, w.env.IngestedCount(w.project), "only the planted row")
	assert.False(t, w.env.BlobOnDisk(apptest.DigestHex(body)))
}

func TestIngestedBytesCountInTheOrganizationQuotaForBothSides(t *testing.T) {
	w := newWorldWith(t, func(c *config.Config) { c.Artifacts.OrgMaxBytes = 16 << 20 })
	w.plantIngested(t, 15<<20, "big")
	w.plantIngested(t, 15<<20, "big-too")
	q := sqlc.New(w.env.Pool)
	used, err := q.OrganizationArtifactBytes(context.Background(), sqlc.OrganizationArtifactBytesParams{
		OrganizationID: w.org, RollbackGraceSeconds: config.RollbackArtifactMaxKeep.Seconds(),
	})
	require.NoError(t, err)
	assert.EqualValues(t, 30<<20, used, "the runner upload quota counts the ingested files")

	res := w.upload(w.archive(), nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "ARTIFACT_QUOTA_EXCEEDED", res.Error.Code)
}

func TestTheSameBytesUnderTwoNamesCountOnceInTheQuota(t *testing.T) {
	w := newWorld(t)
	body := w.archive()
	require.Equal(t, http.StatusCreated, w.upload(body, func(u *apptest.IngestUpload) { u.NoTrigger = true }).Status)
	require.Equal(t, http.StatusCreated, w.upload(body, func(u *apptest.IngestUpload) { u.NoTrigger = true; u.Name = "web2" }).Status)
	used, err := sqlc.New(w.env.Pool).OrganizationArtifactBytes(context.Background(), sqlc.OrganizationArtifactBytesParams{
		OrganizationID: w.org, RollbackGraceSeconds: config.RollbackArtifactMaxKeep.Seconds(),
	})
	require.NoError(t, err)
	assert.EqualValues(t, len(body), used)
}
