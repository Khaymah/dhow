package edge_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
)

var safePath = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,200}$`)

func FuzzDeployPath(f *testing.F) {
	for _, seed := range []string{
		"/srv/shop", "/", "", "/srv/../etc", "/etc/x", "/srv/a;b", "/srv/a b", "/srv/a\nb", "/srv//a", "/srv/./a", "//srv/a",
		"/var/lib/docker/x", "/srv/a/", "/srv/-a", "/srv/\u202ea", "/srv/a\x00b", "/proc/self",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		if !environments.ValidDeployPath(p) {
			return
		}
		assert.Regexp(t, safePath, p, "only safe characters reach a command on the server")
		for _, part := range strings.Split(p[1:], "/") {
			assert.NotContains(t, []string{"", ".", ".."}, part, "%q", p)
			assert.False(t, strings.HasPrefix(part, "-"), "%q", p)
		}
		assert.False(t, strings.HasPrefix(p, "/etc/") || strings.HasPrefix(p, "/usr/") || strings.HasPrefix(p, "/proc/"), "%q", p)
	})
}

func FuzzKeySecretName(f *testing.F) {
	for _, seed := range []string{"web", "prod-fleet", "1a", "dhow", "dhow-x", strings.Repeat("a", 64), "a-b_c"} {
		f.Add(seed)
	}
	secretName := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	f.Fuzz(func(t *testing.T, target string) {
		if !environments.ValidName(target) {
			return
		}
		got := environments.KeySecretName(target)
		assert.Regexp(t, secretName, got)
		assert.False(t, strings.HasPrefix(strings.ToUpper(got), "DHOW_"), "%q is reserved for Dhow", got)
		assert.Equal(t, got, environments.KeySecretName(target))
	})
}
