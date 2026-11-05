package unit_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
)

func TestAllowedPluginsMatchWholeNamePartsOnly(t *testing.T) {
	cases := []struct {
		image    string
		prefixes []string
		want     bool
	}{
		{image: "ghcr.io/khaymah/deploy@sha256:abc", prefixes: nil, want: true},
		{image: "ghcr.io/khaymah/deploy:1", prefixes: []string{"ghcr.io/khaymah/"}, want: true},
		{image: "ghcr.io/khaymah/deploy:1", prefixes: []string{"ghcr.io/khaymah"}, want: true},
		{image: "ghcr.io/khaymah-evil/deploy:1", prefixes: []string{"ghcr.io/khaymah"}},
		{image: "ghcr.io/khaymah-evil/deploy:1", prefixes: []string{"ghcr.io/khaymah/"}},
		{image: "ghcr.io/khaymah/deploy:1", prefixes: []string{"ghcr.io/khaymah/deploy"}, want: true},
		{image: "ghcr.io/khaymah/deployer:1", prefixes: []string{"ghcr.io/khaymah/deploy"}},
		{image: "alpine:3", prefixes: []string{"alpine"}, want: true},
		{image: "alpine-evil:3", prefixes: []string{"alp"}},
		{image: "alpine-evil:3", prefixes: []string{"alpine"}},
		{image: "docker.io/library/alpine:3", prefixes: []string{"alpine"}, want: true},
		{image: "acme/tool:1", prefixes: []string{"a"}},
		{image: "acme/tool:1", prefixes: []string{"acme/"}, want: true},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, pipelines.PluginAllowed(c.image, c.prefixes), "%s with %v", c.image, c.prefixes)
	}
}

func TestOnlyTheExactOfficialNameWithADigestReceivesATarget(t *testing.T) {
	digest := "@sha256:" + strings.Repeat("a", 64)
	cases := []struct {
		image   string
		allowed []string
		want    string
	}{
		{"ghcr.io/khaymah/dhow-deploy-ssh" + digest, nil, ""},
		{"ghcr.io/khaymah/dhow-deploy-compose:1" + digest, nil, ""},
		{"ghcr.io/khaymah/dhow-deploy-ssh/evil" + digest, nil, "TARGET_PLUGIN_NOT_ALLOWED"},
		{"ghcr.io/khaymah/dhow-deploy-ssh-evil" + digest, nil, "TARGET_PLUGIN_NOT_ALLOWED"},
		{"ghcr.io/khaymah/dhow-deploy-ssh:1", nil, "TARGET_PLUGIN_NOT_PINNED"},
		{"ghcr.io/khaymah/dhow-deploy-ssh", nil, "TARGET_PLUGIN_NOT_PINNED"},
		{"ghcr.io/acme/deploy" + digest, nil, "TARGET_PLUGIN_NOT_ALLOWED"},
		{"ghcr.io/acme/deploy" + digest, []string{"ghcr.io/acme/"}, ""},
		{"ghcr.io/acme/deploy:2", []string{"ghcr.io/acme/"}, "TARGET_PLUGIN_NOT_PINNED"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, pipelines.TargetPluginRefusal(c.image, c.allowed), c.image)
	}
}
