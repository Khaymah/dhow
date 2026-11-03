package edge_test

import (
	"strings"
	"testing"

	artifacts "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts"
)

func TestADownloadNameIsAlwaysASafeFileName(t *testing.T) {
	cases := map[string]string{
		"web":                     "web.tar.gz",
		"web-linux.x64":           "web-linux.x64.tar.gz",
		`a"; filename="evil.sh`:   "a___filename__evil.sh.tar.gz",
		"../../etc/passwd":        "_.._etc_passwd.tar.gz",
		"..":                      "artifact.tar.gz",
		"تقرير":                   "_____.tar.gz",
		"line\r\nSet-Cookie: x=1": "line__Set-Cookie__x_1.tar.gz",
	}
	for in, want := range cases {
		if got := artifacts.DownloadName(in); got != want {
			t.Errorf("DownloadName(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzDownloadName(f *testing.F) {
	for _, s := range []string{"web", `"`, "../x", "\r\n", "", ".hidden", "a b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		got := artifacts.DownloadName(name)
		if !strings.HasSuffix(got, ".tar.gz") || strings.HasPrefix(got, ".") {
			t.Fatalf("%q gives %q", name, got)
		}
		for _, r := range got {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
			if !ok {
				t.Fatalf("%q gives %q with %q", name, got, r)
			}
		}
	})
}
