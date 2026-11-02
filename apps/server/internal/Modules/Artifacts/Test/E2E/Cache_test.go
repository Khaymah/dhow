package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
)

const cacheKey = "deps-v1"

func (w world) saveCache(t *testing.T, key string, data []byte) bool {
	t.Helper()
	g := w.next(t)
	hit, err := w.runner.LookupCache(g.Ref, key)
	require.NoError(t, err)
	require.False(t, hit.GetHit(), "the key is new")
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "cache", data).Status)
	stored, err := w.runner.CommitCache(g.Ref, key, data)
	require.NoError(t, err)
	w.succeed(t, g)
	w.drain(t)
	return stored
}

func (w world) scopes(t *testing.T) map[string]string {
	t.Helper()
	rows, err := w.env.Pool.Query(context.Background(), `SELECT "Scope", "Key" FROM "CacheEntries" WHERE NOT "IsDeleted"`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var scope, key string
		require.NoError(t, rows.Scan(&scope, &key))
		out[scope] = key
	}
	return out
}

func TestTheCacheIsRestoredInTheNextRun(t *testing.T) {
	w := newWorld(t)
	data := []byte("node_modules archive")
	w.push(t, "refs/heads/main", shaA)
	assert.True(t, w.saveCache(t, cacheKey, data))
	assert.Equal(t, map[string]string{"branch:main": cacheKey}, w.scopes(t))

	w.push(t, "refs/heads/main", shaB)
	g := w.next(t)
	hit, err := w.runner.LookupCache(g.Ref, cacheKey)
	require.NoError(t, err)
	require.True(t, hit.GetHit())
	assert.Equal(t, int64(len(data)), hit.GetSizeBytes())
	got := w.runner.GetBlob(g.Ref, hit.GetDigest(), hit.GetDownloadToken())
	require.Equal(t, http.StatusOK, got.Status, got.Code)
	assert.Equal(t, data, got.Body)
	stored, err := w.runner.CommitCache(g.Ref, cacheKey, []byte("something newer"))
	assert.Equal(t, "BLOB_NOT_FOUND", reason(err), "the blob must be uploaded first")
	assert.False(t, stored)
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "cache", []byte("something newer")).Status)
	stored, err = w.runner.CommitCache(g.Ref, cacheKey, []byte("something newer"))
	require.NoError(t, err)
	assert.False(t, stored, "an existing key is never replaced")
	assert.NotContains(t, w.env.Logs.String(), hit.GetDownloadToken())
}

func TestABranchFallsBackToTheDefaultBranchAndWritesItsOwnScope(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	w.saveCache(t, cacheKey, []byte("main deps"))
	w.push(t, "refs/heads/feature/login", shaB)
	g := w.next(t)
	hit, err := w.runner.LookupCache(g.Ref, cacheKey)
	require.NoError(t, err)
	assert.True(t, hit.GetHit(), "a branch reads the default branch")
	require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "cache", []byte("feature deps")).Status)
	stored, err := w.runner.CommitCache(g.Ref, "deps-v2", []byte("feature deps"))
	require.NoError(t, err)
	assert.True(t, stored)
	assert.Equal(t, map[string]string{"branch:main": cacheKey, "branch:feature/login": "deps-v2"}, w.scopes(t))
}

func TestAPullRequestCannotWriteTheCacheOfMain(t *testing.T) {
	w := newWorld(t)
	w.pullRequest(t, 5, shaB, triggers.PullOriginSameRepository)
	assert.True(t, w.saveCache(t, cacheKey, []byte("pr deps")))
	assert.Equal(t, map[string]string{"pr:5": cacheKey}, w.scopes(t), "a pull request writes into its own scope")

	w.push(t, "refs/heads/main", shaC)
	g := w.next(t)
	hit, err := w.runner.LookupCache(g.Ref, cacheKey)
	require.NoError(t, err)
	assert.False(t, hit.GetHit(), "main never reads what a pull request wrote")
}

func TestAForkNeverWritesTheCache(t *testing.T) {
	w := newWorld(t)
	w.push(t, "refs/heads/main", shaA)
	w.saveCache(t, cacheKey, []byte("main deps"))
	w.pullRequest(t, 9, shaB, triggers.PullOriginFork)
	g := w.next(t)
	require.True(t, g.Grant.GetSpec().GetCache().GetReadOnly())
	hit, err := w.runner.LookupCache(g.Ref, cacheKey)
	require.NoError(t, err)
	assert.True(t, hit.GetHit(), "a fork may read the default branch")
	res := w.runner.PutBlob(g.Ref, "cache", []byte("poison"))
	assert.Equal(t, http.StatusForbidden, res.Status)
	assert.Equal(t, "CACHE_READ_ONLY", res.Code)
	_, err = w.runner.CommitCache(g.Ref, "deps-v9", []byte("main deps"))
	assert.Equal(t, "CACHE_READ_ONLY", reason(err))
	assert.Equal(t, map[string]string{"branch:main": cacheKey}, w.scopes(t))
}

func TestTheProjectCapRemovesTheLeastRecentlyUsedEntries(t *testing.T) {
	w := newWorldWith(t, func(c *config.Config) { c.Artifacts.CacheMaxBytes = 250 })
	w.push(t, "refs/heads/main", shaA)
	g := w.next(t)
	for _, key := range []string{"k1", "k2", "k3"} {
		data := make([]byte, 100)
		data[0] = key[1]
		require.Equal(t, http.StatusCreated, w.runner.PutBlob(g.Ref, "cache", data).Status)
		stored, err := w.runner.CommitCache(g.Ref, key, data)
		require.NoError(t, err)
		require.True(t, stored)
		if key == "k2" {
			_, err := w.env.Pool.Exec(context.Background(), `UPDATE "CacheEntries" SET "LastUsedAt" = now() - interval '1 hour' WHERE "Key" = 'k1'`)
			require.NoError(t, err)
			_, err = w.env.Pool.Exec(context.Background(), `UPDATE "CacheEntries" SET "LastUsedAt" = now() - interval '2 hours' WHERE "Key" = 'k2'`)
			require.NoError(t, err)
		}
	}
	keys := map[string]bool{}
	for _, k := range w.scopes(t) {
		keys[k] = true
	}
	rows, err := w.env.Pool.Query(context.Background(), `SELECT "Key" FROM "CacheEntries" WHERE NOT "IsDeleted" ORDER BY "Key"`)
	require.NoError(t, err)
	defer rows.Close()
	var left []string
	for rows.Next() {
		var k string
		require.NoError(t, rows.Scan(&k))
		left = append(left, k)
	}
	assert.Equal(t, []string{"k1", "k3"}, left, "the least recently used entry goes first")
	big := make([]byte, 300)
	res := w.runner.PutBlob(g.Ref, "cache", big)
	assert.Equal(t, "BLOB_TOO_LARGE", res.Code, "one entry larger than the cap is refused at the door")
}
