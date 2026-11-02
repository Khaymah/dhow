package pipelinespec

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func matrixKeys(t *testing.T, matrix string) []string {
	t.Helper()
	plan := compileSource(t, []byte(job("    strategy:\n      matrix: "+matrix+"\n    steps: [{run: x}]\n")))
	keys := make([]string, 0, len(plan.Jobs))
	for _, j := range plan.Jobs {
		keys = append(keys, j.Key)
	}
	return keys
}

func TestMatrixExpansion(t *testing.T) {
	cases := []struct {
		name   string
		matrix string
		want   []string
	}{
		{"product sorted by key", "{os: [debian, ubuntu], node: [20, 22]}", []string{
			"a (node=20, os=debian)", "a (node=20, os=ubuntu)", "a (node=22, os=debian)", "a (node=22, os=ubuntu)",
		}},
		{"exclude", "{os: [debian, ubuntu], node: [20, 22], exclude: [{os: ubuntu, node: 20}]}", []string{
			"a (node=20, os=debian)", "a (node=22, os=debian)", "a (node=22, os=ubuntu)",
		}},
		{"exclude by one key", "{os: [debian, ubuntu], node: [20, 22], exclude: [{os: ubuntu}]}", []string{
			"a (node=20, os=debian)", "a (node=22, os=debian)",
		}},
		{"exclude matches type", "{node: [20, '22'], exclude: [{node: '20'}]}", []string{"a (node=20)", "a (node=22)"}},
		{"include", "{os: [debian], include: [{os: alpine}]}", []string{"a (os=debian)", "a (os=alpine)"}},
		{"include only", "{include: [{os: a, v: 1}, {os: b, v: 2}]}", []string{"a (os=a, v=1)", "a (os=b, v=2)"}},
		{"booleans", "{race: [true, false]}", []string{"a (race=true)", "a (race=false)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matrixKeys(t, tc.matrix))
		})
	}
}

func TestMatrixAtTheCap(t *testing.T) {
	keys := matrixKeys(t, "{a: [1,2,3,4,5,6,7,8], b: [1,2,3,4,5,6,7,8], c: [1,2,3,4]}")
	assert.Len(t, keys, MaxMatrixJobs)
}

func TestMatrixExcludeBringsItUnderTheCap(t *testing.T) {
	keys := matrixKeys(t, "{a: [1,2,3,4,5,6,7,8], b: [1,2,3,4,5,6,7,8], c: [1,2,3,4,5], exclude: [{c: 5}]}")
	assert.Len(t, keys, MaxMatrixJobs)
}

func TestNeedsOnAMatrixJobWaitForEveryCopy(t *testing.T) {
	plan := compileSource(t, []byte(pipeline("  t:\n    image: "+img+"\n    strategy: {matrix: {n: [1, 2, 3]}}\n    steps: [{run: x}]\n  d:\n    needs: [t]\n    if: ${{ size(needs.t.outputs) == 3 }}\n    image: "+img+"\n    steps: [{run: x}]\n")))
	d := jobByKey(t, plan, "d")
	assert.Equal(t, []string{"t (n=1)", "t (n=2)", "t (n=3)"}, d.Needs)
}

func TestMatrixTypes(t *testing.T) {
	combos := []combination{
		{{Key: "s", Value: Scalar{Kind: KindString, Text: "a"}}, {Key: "m", Value: Scalar{Kind: KindInt, Text: "1"}}},
		{{Key: "s", Value: Scalar{Kind: KindString, Text: "b"}}, {Key: "m", Value: Scalar{Kind: KindBool, Text: "true"}}},
	}
	types := matrixTypes(combos)
	require.Len(t, types, 2)
	assert.Equal(t, "string", types["s"].String())
	assert.Equal(t, "dyn", types["m"].String())
}

func TestTopoOrder(t *testing.T) {
	order, cyclic := topoOrder(map[string][]string{
		"deploy": {"build", "lint"}, "build": {"test"}, "test": nil, "lint": nil, "docs": nil,
	})
	assert.Equal(t, []string{"docs", "lint", "test", "build", "deploy"}, order)
	assert.Empty(t, cyclic)

	order, cyclic = topoOrder(map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}, "d": nil, "e": {"a"}})
	assert.Equal(t, []string{"d"}, order)
	assert.Equal(t, []string{"a", "b", "c", "e"}, cyclic)
}

func TestUnits(t *testing.T) {
	durations := map[string]int64{"90s": 90, "20m": 1200, "1h30m": 5400, "1d": 86400, "2h0m5s": 7205}
	for in, want := range durations {
		d, err := ParseDuration(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, int64(d.Seconds()), in)
	}
	for _, bad := range []string{"", "1", "m", "1.5h", "30m1h", "1h1h", "-1s", "01m", "1000000s", "1w", "1 h"} {
		_, err := ParseDuration(bad)
		assert.Error(t, err, bad)
	}
	sizes := map[string]int64{"512Mi": 512 << 20, "4Gi": 4 << 30, "1Ti": 1 << 40, "64Ki": 64 << 10}
	for in, want := range sizes {
		v, err := ParseSize(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, v, in)
	}
	for _, bad := range []string{"", "4GB", "4G", "Gi", "-1Gi", "1.5Gi", "99999999Gi"} {
		_, err := ParseSize(bad)
		assert.Error(t, err, bad)
	}
	cpus := map[string]int64{"2": 2000, "1.5": 1500, "0.25": 250, "1500m": 1500, "0.1": 100}
	for in, want := range cpus {
		v, err := ParseCPU(Scalar{Text: in})
		require.NoError(t, err, in)
		assert.Equal(t, want, v, in)
	}
	for _, bad := range []string{"", "m", "1.", "1.2345", "abc", "-1", "1e3"} {
		_, err := ParseCPU(Scalar{Text: bad})
		assert.Error(t, err, bad)
	}
}

func TestPatterns(t *testing.T) {
	matches := []struct {
		pattern, name string
		want          bool
	}{
		{"main", "main", true},
		{"release/*", "release/1.0", true},
		{"release/*", "release/1/0", false},
		{"release/**", "release/1/0", true},
		{"release/**", "release", true},
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "c.go", true},
		{"v?.*", "v1.2", true},
		{"v?.*", "v12.2", false},
		{"docs/**/x", "docs/x", true},
		{"*", "", true},
	}
	for _, m := range matches {
		assert.Equal(t, m.want, MatchPattern(m.pattern, m.name), "%s ~ %s", m.pattern, m.name)
	}
	for _, bad := range []string{"", "a**", "[ab]", "{a,b}", "a\\b", "x\ny", "${{ x }}"} {
		assert.False(t, ValidPattern(bad), bad)
		assert.False(t, MatchPattern(bad, "a"), bad)
	}
}

func longMatrix(axes int) string {
	var b strings.Builder
	b.WriteString("version: 1\non:\n  push: {}\njobs:\n  build:\n    image: alpine@sha256:" + strings.Repeat("a", 64) +
		"\n    strategy:\n      matrix:\n")
	for i := range axes {
		fmt.Fprintf(&b, "        k%02d: [%q]\n", i, strings.Repeat(",", MaxMatrixValue))
	}
	b.WriteString("    steps:\n      - run: make\n")
	return b.String()
}

func TestAMatrixNameTooLongForTheRunIsRefused(t *testing.T) {
	_, issues := Check([]byte(longMatrix(MaxMatrixAxes)))
	assert.Contains(t, issueCodes(issues, SeverityError), CodeMatrixNameTooLong)

	spec, issues := Check([]byte(longMatrix(1)))
	require.False(t, HasErrors(issues), "%v", issues)
	plan, err := Compile(spec)
	require.NoError(t, err)
	for _, rj := range plan.Jobs {
		assert.LessOrEqual(t, utf8.RuneCountInString(rj.Key), 1024)
		assert.LessOrEqual(t, utf8.RuneCountInString(rj.Name), 1024)
	}
}
