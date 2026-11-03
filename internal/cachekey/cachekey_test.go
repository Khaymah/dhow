package cachekey_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/cachekey"
)

func TestRenderFillsEveryHashExactlyOnce(t *testing.T) {
	h := strings.Repeat("a", 64)
	got, err := cachekey.Render("npm-%%-%0", []string{h})
	require.NoError(t, err)
	assert.Equal(t, "npm-%-"+h, got)
	for _, bad := range []struct {
		tmpl   string
		hashes []string
	}{
		{"x-%0", nil},
		{"x", []string{h}},
		{"%0%0", []string{h}},
		{"x-%", nil},
		{"x-%a", nil},
		{"", nil},
		{"line\nbreak", nil},
		{strings.Repeat("k", 600), nil},
	} {
		_, err := cachekey.Render(bad.tmpl, bad.hashes)
		assert.Error(t, err, bad.tmpl)
	}
}

func TestHashFilesIsStableAndFollowsThePatterns(t *testing.T) {
	fsys := fstest.MapFS{
		"package-lock.json":     {Data: []byte("v1")},
		"web/package-lock.json": {Data: []byte("v2")},
		"README.md":             {Data: []byte("docs")},
	}
	a, err := cachekey.HashFiles(fsys, []string{"**/package-lock.json"})
	require.NoError(t, err)
	b, err := cachekey.HashFiles(fsys, []string{"**/package-lock.json", "**/package-lock.json"})
	require.NoError(t, err)
	assert.Equal(t, a, b)
	assert.Len(t, a, 64)
	fsys["web/package-lock.json"] = &fstest.MapFile{Data: []byte("v3")}
	c, err := cachekey.HashFiles(fsys, []string{"**/package-lock.json"})
	require.NoError(t, err)
	assert.NotEqual(t, a, c, "a changed file changes the key")
	d, err := cachekey.HashFiles(fsys, []string{"README.md"})
	require.NoError(t, err)
	assert.NotEqual(t, a, d)
	for _, bad := range [][]string{nil, {"/abs"}, {"a/**b"}, {"[x]"}} {
		_, err := cachekey.HashFiles(fsys, bad)
		assert.ErrorIs(t, err, cachekey.ErrPattern, "%v", bad)
	}
}
