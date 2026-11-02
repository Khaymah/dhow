package pipelinespec

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/cachekey"
)

func cachePlan(t *testing.T, key string) *RunJob {
	t.Helper()
	src := "    env:\n      NODE: \"22\"\n    steps:\n      - run: npm ci\n    cache:\n      key: '" + key + "'\n      paths: [.npm]\n"
	plan := compileSource(t, []byte(job(src)))
	require.Len(t, plan.Jobs, 1)
	require.NotNil(t, plan.Jobs[0].Cache)
	return plan.Jobs[0]
}

func TestACacheKeyBecomesATemplateWithItsHashes(t *testing.T) {
	cases := []struct {
		key, template string
		hashes        [][]string
	}{
		{`deps-%v1`, `deps-%%v1`, nil},
		{`${{ "npm-" + env.NODE + "-" + hash_files("package-lock.json") }}`, `npm-22-%0`, [][]string{{"package-lock.json"}}},
		{
			`${{ git.branch + "-" + hash_files(["a/**/*.lock", "b.txt"]) + hash_files("c") }}`, `main-%0%1`,
			[][]string{{"a/**/*.lock", "b.txt"}, {"c"}},
		},
	}
	for _, c := range cases {
		rj := cachePlan(t, c.key)
		got, err := rj.EvalCacheKey(context.Background(), onMain, map[string]string{"NODE": "22"})
		require.NoError(t, err, c.key)
		assert.Equal(t, c.template, got.Template, c.key)
		assert.Equal(t, c.hashes, got.Hashes, c.key)
		sums := make([]string, len(got.Hashes))
		for i := range sums {
			sums[i] = strings.Repeat("f", 64)
		}
		_, err = cachekey.Render(got.Template, sums)
		assert.NoError(t, err, c.key)
	}
}

func TestACacheKeyThatUsesAHashForAnythingButItsTextIsRefused(t *testing.T) {
	for _, key := range []string{
		`${{ size(hash_files("a")) > 3 ? "big" : "small" }}`,
		`${{ [hash_files("a"), hash_files("b")][0] }}`,
		`${{ hash_files("a") == "x" ? "x" : "y" }}`,
		`${{ hash_files("/etc/passwd") }}`,
		`${{ hash_files("a") + hash_files("b") + hash_files("c") + hash_files("d") + hash_files("e") + hash_files("f") + hash_files("g") + hash_files("h") + hash_files("i") }}`,
	} {
		rj := cachePlan(t, key)
		_, err := rj.EvalCacheKey(context.Background(), onMain, nil)
		assert.Error(t, err, key)
	}
}
