package unit_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
)

func TestDeployPaths(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/srv/shop", true},
		{"/srv/shop/app-1.2_x", true},
		{"/home/deploy/apps/shop", true},
		{"/opt/shop", true},
		{"/var/www/shop", true},
		{"", false},
		{"/", false},
		{"/srv", false},
		{"srv/shop", false},
		{"/srv/shop/", false},
		{"/srv//shop", false},
		{"/srv/./shop", false},
		{"/srv/../etc", false},
		{"/srv/shop/..", false},
		{"/srv/-rf", false},
		{"/srv/shop;ls", false},
		{"/srv/shop name", false},
		{"/srv/$HOME", false},
		{"/srv/shop\n", false},
		{"/etc/shop", false},
		{"/etc", false},
		{"/usr/local/shop", false},
		{"/bin/shop", false},
		{"/sbin/shop", false},
		{"/lib64/shop", false},
		{"/boot/shop", false},
		{"/dev/shm", false},
		{"/proc/1", false},
		{"/sys/fs", false},
		{"/root/shop", false},
		{"/run/shop", false},
		{"/var/lib/docker/volumes", false},
		{"/var/run/docker", false},
		{"/" + strings.Repeat("a", 201), false},
		{"/srv/" + strings.Repeat("a", 190), true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, environments.ValidDeployPath(tt.path))
		})
	}
}

func TestTheKeySecretNameFollowsTheTarget(t *testing.T) {
	secretName := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	tests := []struct {
		target string
		want   string
	}{
		{"web", "WEB_SSH_KEY"},
		{"prod-fleet", "PROD_FLEET_SSH_KEY"},
		{"a_b-c", "A_B_C_SSH_KEY"},
		{"1st", "T_1ST_SSH_KEY"},
		{"dhow", "T_DHOW_SSH_KEY"},
		{"dhow-core", "T_DHOW_CORE_SSH_KEY"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			assert.Equal(t, tt.want, environments.KeySecretName(tt.target))
		})
	}
	long := strings.Repeat("x", 64)
	got := environments.KeySecretName(long)
	assert.Regexp(t, secretName, got)
	assert.LessOrEqual(t, len(got), 64)
	assert.True(t, strings.HasSuffix(got, "_SSH_KEY"))
	assert.NotEqual(t, got, environments.KeySecretName(strings.Repeat("x", 63)+"y"), "two long names stay apart")
	assert.Equal(t, got, environments.KeySecretName(long), "the name is stable")
}
