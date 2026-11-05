package edge_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	secrets "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets"
)

var jobSecretName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func nameByHand(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i := range len(name) {
		c := name[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func FuzzSecretName(f *testing.F) {
	for _, seed := range []string{
		"TOKEN", "_x", "a1_B2", "1TOKEN", "", "WITH-DASH", "space x", "DHOW_TOKEN", "dhow_x", "DHOWTOKEN",
		strings.Repeat("A", 64), strings.Repeat("A", 65), "TOKEN\n", "TOKEN\x00", "\u0661TOKEN", "TOKEN\u202e", "\uff21BC",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		valid := secrets.ValidName(name)
		assert.Equal(t, nameByHand(name), valid, "%q", name)
		if valid {
			assert.True(t, jobSecretName.MatchString(name), "a secret name the job spec would refuse: %q", name)
			assert.LessOrEqual(t, len(name), 64)
		}
		assert.Equal(t, len(name) >= 5 && strings.EqualFold(name[:5], "DHOW_"), secrets.Reserved(name), "%q", name)
	})
}
