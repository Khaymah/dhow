package policy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

func FuzzComposePolicy(f *testing.F) {
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
