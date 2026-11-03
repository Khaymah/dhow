package pathglob_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Khaymah/dhow/internal/pathglob"
)

func FuzzMatchPattern(f *testing.F) {
	for _, seed := range [][2]string{
		{"src/**/*.go", "src/a/b/c.go"}, {"*.lock", "deps.lock"}, {"a?c", "abc"}, {"**", "x/y/z"},
		{"a/**/b", "a/b"}, {"[a]", "a"}, {"a**", "ab"}, {"*", "a/b"}, {"?", "é"}, {"${{ x }}", "x"},
		{strings.Repeat("*", 64), strings.Repeat("a", 256)}, {"**/**/**/x", "x"},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, pattern, name string) {
		got := pathglob.MatchPattern(pattern, name)
		if !pathglob.ValidPattern(pattern) {
			if got {
				t.Fatalf("an invalid pattern %q matched %q", pattern, name)
			}
			return
		}
		if !utf8.ValidString(name) {
			return
		}
		if !strings.ContainsAny(pattern, "*?") && got != (pattern == name) {
			t.Fatalf("the literal pattern %q gave %v for %q", pattern, got, name)
		}
		if got && !strings.Contains(pattern, "**") && strings.Count(pattern, "/") != strings.Count(name, "/") {
			t.Fatalf("%q matched %q across a path separator", pattern, name)
		}
		if got && !strings.Contains(pattern, "**") && !strings.Contains(pattern, "*") &&
			utf8.RuneCountInString(pattern) != utf8.RuneCountInString(name) {
			t.Fatalf("%q matched %q of another length without a star", pattern, name)
		}
	})
}
