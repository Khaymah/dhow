package policy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

func stray(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '$' {
			i++
			continue
		}
		return true
	}
	return false
}

func requireNoStrayDollar(t *testing.T, path []string, v any) {
	t.Helper()
	if len(path) >= 3 && path[0] == "services" {
		switch path[2] {
		case "environment", "command", "entrypoint", "labels":
			return
		case "healthcheck":
			if len(path) >= 4 && path[3] == "test" {
				return
			}
		}
	}
	switch x := v.(type) {
	case string:
		require.False(t, stray(x), "%v: %q", path, x)
	case []any:
		for _, item := range x {
			requireNoStrayDollar(t, append(path[:len(path):len(path)], "[]"), item)
		}
	case map[string]any:
		for k, val := range x {
			require.False(t, stray(k), "%v: key %q", path, k)
			requireNoStrayDollar(t, append(path[:len(path):len(path)], k), val)
		}
	}
}

func FuzzComposePolicy(f *testing.F) {
	f.Add([]byte("services:\n  web:\n    image: "+webDigest+"\n    read_only: \"${RO:-false}\"\n"), true)
	f.Add([]byte("services:\n  web:\n    image: "+webDigest+"\n"), false)
	f.Add([]byte("services:\n  web:\n    image: "+webDigest+"\n    privileged: true\n    volumes: ['/:/h']\n"), false)
	f.Add([]byte("x: &a {privileged: true}\nservices:\n  web:\n    <<: *a\n    image: "+webDigest+"\n"), true)
	f.Add([]byte("services:\n  web:\n    image: a:b\n    build: .\n"), false)
	f.Add([]byte("services:\n  web:\n    image: "+webDigest+"\n    volumes:\n      - type: bind\n        source: ../..\n"), false)
	f.Add([]byte("---\n---\n"), false)
	f.Fuzz(func(t *testing.T, raw []byte, unsafe bool) {
		opt := policy.Options{DeployPath: deployDir, AllowUnsafe: unsafe, Links: []string{"link"}}
		res, err := policy.Check(raw, opt)
		if err != nil {
			var v *policy.Violation
			require.True(t, errors.As(err, &v))
			require.True(t, strings.HasPrefix(v.Code, "COMPOSE_"))
			return
		}
		require.NotEmpty(t, res.Images)
		var doc map[string]any
		require.NoError(t, yaml.Unmarshal(raw, &doc))
		requireNoStrayDollar(t, nil, doc)
		for _, b := range res.Binds {
			require.False(t, strings.HasPrefix(b, "/") || b == "." || b == ".." || strings.HasPrefix(b, "../"), b)
		}
		for name, ref := range res.Images {
			require.True(t, policy.ValidImage(ref), name)
		}
		override, err := policy.Override(res.Images)
		require.NoError(t, err)
		again, err := policy.Check(override, opt)
		require.NoError(t, err)
		require.Equal(t, res.Images, again.Images)
	})
}
