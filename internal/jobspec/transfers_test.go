package jobspec_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func transferSpec() *runnerv1.JobSpec {
	return jobspec.Normalize(&runnerv1.JobSpec{
		Image:     "alpine@sha256:" + strings.Repeat("a", 64),
		Steps:     []*runnerv1.Step{{Name: "build", Run: "make"}},
		Artifacts: []*runnerv1.ArtifactUpload{{Name: "web", Paths: []string{"dist"}, MaxBytes: 1 << 20}},
		Inputs:    []*runnerv1.ArtifactInput{{Name: "lib", Digest: strings.Repeat("b", 64), SizeBytes: 10, DownloadToken: "AbC_-1"}},
		Cache: &runnerv1.Cache{
			KeyExpression: "npm-%0", Hashes: []*runnerv1.CacheHash{{Patterns: []string{"package-lock.json"}}},
			Paths: []string{".npm"}, MaxBytes: 1 << 20,
		},
	})
}

func TestTransfersAreCheckedAndAskForTheirFeatures(t *testing.T) {
	require.NoError(t, jobspec.Validate(transferSpec()))
	assert.ElementsMatch(t, []string{jobspec.FeatureArtifacts, jobspec.FeatureCache}, jobspec.Features(transferSpec()))

	cases := map[string]func(*runnerv1.JobSpec){
		"an artifact path that climbs out":   func(s *runnerv1.JobSpec) { s.Artifacts[0].Paths = []string{"../etc"} },
		"an absolute artifact path":          func(s *runnerv1.JobSpec) { s.Artifacts[0].Paths = []string{"/etc"} },
		"an artifact without paths":          func(s *runnerv1.JobSpec) { s.Artifacts[0].Paths = nil },
		"an artifact name with a slash":      func(s *runnerv1.JobSpec) { s.Artifacts[0].Name = "a/b" },
		"two artifacts with one name":        func(s *runnerv1.JobSpec) { s.Artifacts = append(s.Artifacts, s.Artifacts[0]) },
		"an artifact without a limit":        func(s *runnerv1.JobSpec) { s.Artifacts[0].MaxBytes = 0 },
		"an input with a short digest":       func(s *runnerv1.JobSpec) { s.Inputs[0].Digest = "abc" },
		"an input without a grant":           func(s *runnerv1.JobSpec) { s.Inputs[0].DownloadToken = "" },
		"an input grant with spaces":         func(s *runnerv1.JobSpec) { s.Inputs[0].DownloadToken = "a b" },
		"a cache key that skips a hash":      func(s *runnerv1.JobSpec) { s.Cache.KeyExpression = "npm" },
		"a cache key with a stray escape":    func(s *runnerv1.JobSpec) { s.Cache.KeyExpression = "npm-%0-%" },
		"a cache hash with an absolute path": func(s *runnerv1.JobSpec) { s.Cache.Hashes[0].Patterns = []string{"/etc/shadow"} },
		"a cache path that climbs out":       func(s *runnerv1.JobSpec) { s.Cache.Paths = []string{"a/../../b"} },
	}
	for name, mutate := range cases {
		spec := transferSpec()
		mutate(spec)
		assert.Error(t, jobspec.Validate(spec), name)
	}
}
