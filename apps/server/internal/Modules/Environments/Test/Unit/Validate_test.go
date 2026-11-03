package unit_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/pathglob"
)

func TestRefPatterns(t *testing.T) {
	for _, ok := range []string{"main", "release/*", "release/**", "v?.*", "feature/**/done", "hotfix-1", strings.Repeat("a", pathglob.MaxLength)} {
		assert.True(t, environments.ValidRefPattern(ok), "%q", ok)
	}
	bad := []string{
		"", "..", "release/..", "../main", "a..b", "release//x", "/main", "main/", "-main", "ma in", "main\t", "ma~in", "ma^in", "ma:in",
		"main@{1}", "refs/heads/main", "heads/main", "tags/v1", "refs/tags/v1", "ma\u202ein", "ma\u200bin", "\u2066main\u2069", "main\ufeff", "re[a]", "{a,b}", "a\\b", "a**b", "${{ github.ref }}", "main\x00", strings.Repeat("a", pathglob.MaxLength+1),
	}
	for _, p := range bad {
		assert.False(t, environments.ValidRefPattern(p), "%q", p)
	}
}

func refs(n int) []models.Ref {
	out := make([]models.Ref, 0, n)
	for i := range n {
		out = append(out, models.Ref{Kind: models.RefBranch, Pattern: "release/" + strconv.Itoa(i)})
	}
	return out
}

func ids(n int) []uuid.UUID {
	out := make([]uuid.UUID, 0, n)
	for range n {
		out = append(out, uuid.New())
	}
	return out
}

func plugins(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, "ghcr.io/khaymah/p"+strconv.Itoa(i))
	}
	return out
}

func TestValidateRules(t *testing.T) {
	base := environments.DefaultRules("main")
	tests := []struct {
		name  string
		edit  func(r *models.Rules)
		field string
	}{
		{"the defaults", func(*models.Rules) {}, ""},
		{"fifty patterns", func(r *models.Rules) { r.AllowedRefs = refs(50) }, ""},
		{"fifty-one patterns", func(r *models.Rules) { r.AllowedRefs = refs(51) }, "Rules.AllowedRefs"},
		{"no patterns: nothing deploys", func(r *models.Rules) { r.AllowedRefs = []models.Ref{} }, ""},
		{"a pattern with ..", func(r *models.Rules) { r.AllowedRefs = []models.Ref{{Kind: "branch", Pattern: "release/.."}} }, "Rules.AllowedRefs[0].Pattern"},
		{"a pattern over the limit", func(r *models.Rules) {
			r.AllowedRefs = []models.Ref{{Kind: "tag", Pattern: strings.Repeat("v", pathglob.MaxLength+1)}}
		}, "Rules.AllowedRefs[0].Pattern"},
		{"an unknown kind", func(r *models.Rules) { r.AllowedRefs = []models.Ref{{Kind: "commit", Pattern: "main"}} }, "Rules.AllowedRefs[0].Kind"},
		{"two approvals", func(r *models.Rules) { r.RequiredApprovals = 2 }, "Rules.RequiredApprovals"},
		{"no approvals", func(r *models.Rules) { r.RequiredApprovals = 0 }, "Rules.RequiredApprovals"},
		{"twenty reviewers", func(r *models.Rules) { r.Reviewers = models.Reviewers{UserIDs: ids(15), TeamIDs: ids(5)} }, ""},
		{"twenty-one reviewers", func(r *models.Rules) { r.Reviewers = models.Reviewers{UserIDs: ids(11), TeamIDs: ids(10)} }, "Rules.Reviewers"},
		{"a nil reviewer", func(r *models.Rules) { r.Reviewers = models.Reviewers{UserIDs: []uuid.UUID{uuid.Nil}} }, "Rules.Reviewers"},
		{"a negative wait", func(r *models.Rules) { r.WaitMinutes = -1 }, "Rules.WaitMinutes"},
		{"thirty days of wait", func(r *models.Rules) { r.WaitMinutes = 43200 }, ""},
		{"more than thirty days of wait", func(r *models.Rules) { r.WaitMinutes = 43201 }, "Rules.WaitMinutes"},
		{"a zero timeout", func(r *models.Rules) { r.ApprovalTimeoutHours = 0 }, "Rules.ApprovalTimeoutHours"},
		{"a timeout over thirty days", func(r *models.Rules) { r.ApprovalTimeoutHours = 721 }, "Rules.ApprovalTimeoutHours"},
		{"fifty plugins", func(r *models.Rules) { r.AllowedPlugins = plugins(50) }, ""},
		{"fifty-one plugins", func(r *models.Rules) { r.AllowedPlugins = plugins(51) }, "Rules.AllowedPlugins"},
		{"a plugin prefix with a space", func(r *models.Rules) { r.AllowedPlugins = []string{"ghcr.io/ khaymah"} }, "Rules.AllowedPlugins[0]"},
		{"a plugin prefix with ..", func(r *models.Rules) { r.AllowedPlugins = []string{"ghcr.io/../x"} }, "Rules.AllowedPlugins[0]"},
		{"gvisor isolation", func(r *models.Rules) { r.RequiredIsolation = "gvisor" }, "Rules.RequiredIsolation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.edit(&r)
			details := environments.ValidateRules(r)
			if tt.field == "" {
				assert.Empty(t, details)
				return
			}
			got := make([]string, 0, len(details))
			for _, d := range details {
				got = append(got, d.Field)
			}
			assert.Contains(t, got, tt.field)
		})
	}
}

func TestDefaultRulesAreSafe(t *testing.T) {
	r := environments.DefaultRules("")
	assert.Equal(t, []models.Ref{{Kind: models.RefBranch, Pattern: "main"}}, r.AllowedRefs)
	assert.Equal(t, 1, r.RequiredApprovals)
	assert.False(t, r.AllowSelfApproval, "self-approval is off by default")
	assert.False(t, r.AllowAdminBypass)
	assert.False(t, r.AllowPullRequests)
	assert.True(t, r.SupersedePending)
	assert.Equal(t, 72, r.ApprovalTimeoutHours)
	assert.Equal(t, "container", r.RequiredIsolation)
	assert.Equal(t, []models.Ref{{Kind: models.RefBranch, Pattern: "trunk"}}, environments.DefaultRules("trunk").AllowedRefs)
	assert.Empty(t, environments.DefaultRules("weird..branch").AllowedRefs, "a default branch that is not a valid pattern deploys nothing")
}

func TestApplyKeepsWhatIsNotSent(t *testing.T) {
	base := environments.DefaultRules("main")
	wait, self := 15, true
	one, two := uuid.New(), uuid.New()
	r := environments.Apply(base, &models.RulesInput{
		WaitMinutes: &wait, AllowSelfApproval: &self,
		Reviewers:   &models.Reviewers{UserIDs: []uuid.UUID{two, one, two}},
		AllowedRefs: &[]models.Ref{{Kind: "branch", Pattern: "main"}, {Kind: "branch", Pattern: "main"}, {Kind: "tag", Pattern: "v*"}},
	})
	assert.Equal(t, 15, r.WaitMinutes)
	assert.True(t, r.AllowSelfApproval)
	assert.Equal(t, base.ApprovalTimeoutHours, r.ApprovalTimeoutHours)
	assert.Equal(t, base.SupersedePending, r.SupersedePending)
	assert.Len(t, r.Reviewers.UserIDs, 2, "duplicate reviewers collapse")
	assert.Empty(t, r.Reviewers.TeamIDs)
	assert.Equal(t, []models.Ref{{Kind: "branch", Pattern: "main"}, {Kind: "tag", Pattern: "v*"}}, r.AllowedRefs)
	assert.Equal(t, environments.Normalize(base), environments.Apply(base, nil))
}

func TestDoubleStarAlsoMatchesTheParent(t *testing.T) {
	assert.True(t, pathglob.MatchPattern("release/**", "release"), "documented: release/** also matches release itself")
	assert.True(t, pathglob.MatchPattern("release/**", "release/1.2"))
	assert.True(t, pathglob.MatchPattern("release/**", "release/2026/1"))
	assert.False(t, pathglob.MatchPattern("release/*", "release"))
	assert.False(t, pathglob.MatchPattern("release/**", "releases"))
}
