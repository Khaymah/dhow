package edge_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/pathglob"
)

func FuzzAllowedRefs(f *testing.F) {
	f.Add("main", "main", false, false)
	f.Add("release/*", "release/1.2", false, false)
	f.Add("release/**", "release/1/2", true, false)
	f.Add("v?.*", "v1.0", true, false)
	f.Add("feature/**/done", "feature/a/b/done", false, true)
	f.Add("refs/heads/main", "main", false, false)
	f.Add("main", "", false, false)
	f.Add("../main", "main", false, false)
	f.Add("ma\u202ein", "ma\u202ein", false, false)
	f.Add("release/*", "release", false, false)
	f.Add("*", "a/b", false, false)
	f.Add("release/?", "release/", false, false)
	f.Add("**", "a/b/c", false, false)
	f.Add("a/**/b", "a/b", true, false)
	f.Fuzz(func(t *testing.T, pattern, name string, tag, pullRequest bool) {
		kind := models.RefBranch
		other := models.RefTag
		if tag {
			kind, other = other, kind
		}
		refs := []models.Ref{{Kind: kind, Pattern: pattern}}
		allowed := environments.RefAllowed(refs, true, kind, name, pullRequest)
		if environments.ValidRefPattern(pattern) {
			assert.True(t, pathglob.ValidPattern(pattern), "a ref pattern the glob rejects is never stored: %q", pattern)
		}
		if !pathglob.ValidPattern(pattern) || name == "" {
			assert.False(t, allowed, "pattern %q name %q", pattern, name)
		}
		assert.False(t, environments.RefAllowed(refs, false, kind, name, true), "a pull request is refused unless the rules allow it")
		assert.False(t, environments.RefAllowed(nil, true, kind, name, pullRequest), "no rules means nothing deploys")
		assert.False(t, environments.RefAllowed(refs, true, "other", name, false), "an unknown kind never deploys")
		if !pullRequest {
			assert.False(t, environments.RefAllowed(refs, true, other, name, false), "a %s rule does not allow a %s", kind, other)
		}
		if pathglob.ValidPattern(pattern) && name != "" && !pullRequest && len(pattern) <= 32 && len(name) <= 64 {
			want := oracleMatch(strings.Split(pattern, "/"), strings.Split(name, "/"))
			assert.Equal(t, want, allowed, "pattern %q name %q: the matcher and an independent backtracking one disagree", pattern, name)
		}
		if environments.ValidRefPattern(pattern) && !strings.ContainsAny(pattern, "*?") && !pullRequest {
			assert.Equal(t, name == pattern, allowed, "a literal pattern %q matches only itself, got name %q", pattern, name)
		}
	})
}

func oracleMatch(patterns, names []string) bool {
	if len(patterns) == 0 {
		return len(names) == 0
	}
	if patterns[0] == "**" {
		for skip := 0; skip <= len(names); skip++ {
			if oracleMatch(patterns[1:], names[skip:]) {
				return true
			}
		}
		return false
	}
	return len(names) > 0 && oracleSegment([]rune(patterns[0]), []rune(names[0])) && oracleMatch(patterns[1:], names[1:])
}

func oracleSegment(pattern, name []rune) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	switch pattern[0] {
	case '*':
		return oracleSegment(pattern[1:], name) || (len(name) > 0 && oracleSegment(pattern, name[1:]))
	case '?':
		return len(name) > 0 && oracleSegment(pattern[1:], name[1:])
	}
	return len(name) > 0 && name[0] == pattern[0] && oracleSegment(pattern[1:], name[1:])
}
