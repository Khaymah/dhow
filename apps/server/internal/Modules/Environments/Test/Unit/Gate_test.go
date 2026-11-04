package unit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

func TestRefAllowedMatchesTheKindAndThePattern(t *testing.T) {
	refs := []models.Ref{
		{Kind: models.RefBranch, Pattern: "main"},
		{Kind: models.RefBranch, Pattern: "release/**"},
		{Kind: models.RefTag, Pattern: "v*"},
	}
	cases := []struct {
		name        string
		kind, ref   string
		pullRequest bool
		allowPRs    bool
		want        bool
	}{
		{name: "the default branch", kind: "branch", ref: "main", want: true},
		{name: "a nested release branch", kind: "branch", ref: "release/2026/11", want: true},
		{name: "a version tag", kind: "tag", ref: "v1.2.0", want: true},
		{name: "a tag named like the branch", kind: "tag", ref: "main"},
		{name: "a branch named like the tag", kind: "branch", ref: "v1.2.0"},
		{name: "another branch", kind: "branch", ref: "feature/login"},
		{name: "a prefix is not a match", kind: "branch", ref: "main-old"},
		{name: "an empty name", kind: "branch", ref: ""},
		{name: "an unknown kind", kind: "pull_request", ref: "main"},
		{name: "a PR into main when PRs are off", kind: "branch", ref: "main", pullRequest: true},
		{name: "a PR into main when PRs are on", kind: "branch", ref: "main", pullRequest: true, allowPRs: true, want: true},
		{name: "a PR into another branch", kind: "branch", ref: "dev", pullRequest: true, allowPRs: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, environments.RefAllowed(refs, c.allowPRs, c.kind, c.ref, c.pullRequest))
		})
	}
}

func TestNoAllowedRefsMeansNothingDeploys(t *testing.T) {
	assert.False(t, environments.RefAllowed(nil, true, models.RefBranch, "main", false))
	assert.False(t, environments.RefAllowed([]models.Ref{}, true, models.RefBranch, "main", true))
}
