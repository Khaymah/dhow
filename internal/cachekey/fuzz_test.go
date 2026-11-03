package cachekey_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Khaymah/dhow/internal/cachekey"
)

func FuzzRender(f *testing.F) {
	h := strings.Repeat("a", 64)
	f.Add("npm-%%-%0", "deps-", h, h)
	f.Add("%0-%1", "", h, h)
	f.Add("%1-%0", "x", h, "")
	f.Add("x-%", "%0", "", "")
	f.Add("%9", "line\nbreak", h, h)
	f.Add(strings.Repeat("%%", 300), strings.Repeat("%", 300), h, h)
	f.Fuzz(func(t *testing.T, template, text, first, second string) {
		var hashes []string
		for _, v := range []string{first, second} {
			if v != "" {
				hashes = append(hashes, v)
			}
		}
		key, err := cachekey.Render(template, hashes)
		if err == nil && !cachekey.ValidKey(key) {
			t.Fatalf("Render(%q) gave the key %q that is not valid", template, key)
		}
		if err == nil {
			for _, v := range hashes {
				if !strings.Contains(key, v) {
					t.Fatalf("Render(%q) left out the hash %q", template, v)
				}
			}
		}
		if cachekey.ValidKey(text) && len(cachekey.Escape(text)) <= cachekey.MaxTemplateBytes {
			back, err := cachekey.Render(cachekey.Escape(text), nil)
			if err != nil || back != text {
				t.Fatalf("escaped text %q came back as %q: %v", text, back, err)
			}
		}
		if len(hashes) == 0 || !utf8.ValidString(text) || !utf8.ValidString(hashes[0]) {
			return
		}
		var b cachekey.Builder
		b.Text(text)
		b.Hash(0)
		want := text + hashes[0]
		got, err := cachekey.Render(b.String(), hashes[:1])
		if cachekey.ValidKey(want) && len(b.String()) <= cachekey.MaxTemplateBytes && (err != nil || got != want) {
			t.Fatalf("the template %q gave %q, not %q: %v", b.String(), got, want, err)
		}
	})
}
