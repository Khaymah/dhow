package e2e_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func TestCreateAnEnvironmentWithSafeDefaults(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	assert.Equal(t, "production", e.Name)
	assert.Equal(t, w.project.String(), e.ProjectId)
	assert.Equal(t, w.org.String(), e.OrganizationId)
	assert.Equal(t, int64(1), e.Version)
	assert.Equal(t, int64(1), e.RuleVersion)
	assert.False(t, e.Paused)
	assert.Equal(t, []refView{{Kind: "branch", Pattern: "main"}}, e.Rules.AllowedRefs, "without a repository the default branch is main")
	assert.Equal(t, 1, e.Rules.RequiredApprovals)
	assert.False(t, e.Rules.AllowSelfApproval, "self-approval is blocked by default")
	assert.False(t, e.Rules.AllowAdminBypass)
	assert.False(t, e.Rules.AllowPullRequests)
	assert.True(t, e.Rules.SupersedePending)
	assert.Equal(t, 72, e.Rules.ApprovalTimeoutHours)
	assert.Equal(t, "container", e.Rules.RequiredIsolation)
	assert.Empty(t, e.Rules.Reviewers.UserIds)
	assert.Empty(t, e.Rules.AllowedPlugins)

	got := decode(t, w.get(w.owner, "production"), http.StatusOK)
	assert.Equal(t, e, got)
	requireError(t, w.create(w.owner, map[string]any{"Name": "production"}), http.StatusConflict, "ENVIRONMENT_EXISTS")
	assert.Equal(t, 1, w.env.AuditCount("environment.created"))

	res := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: w.path(w.project, "")})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var list []environmentView
	res.Decode(t, &list)
	require.Len(t, list, 1)
	assert.Equal(t, 1, res.Meta.Total)
}

func TestTheDefaultRefIsTheDefaultBranchOfTheRepository(t *testing.T) {
	w := newWorld(t)
	repo := w.env.SeedRepository(w.org, w.project, w.ownerID)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Repositories" SET "DefaultBranch" = 'trunk' WHERE "Id" = $1`, repo)
	require.NoError(t, err)
	e := w.production(t)
	assert.Equal(t, []refView{{Kind: "branch", Pattern: "trunk"}}, e.Rules.AllowedRefs)
}

func TestCreateWithExplicitRules(t *testing.T) {
	w := newWorld(t)
	_, dev := w.member(t, "developer")
	e := decode(t, w.create(w.owner, map[string]any{
		"Name": "staging", "Description": "Before production",
		"Rules": map[string]any{
			"AllowedRefs":       []map[string]string{{"Kind": "branch", "Pattern": "release/*"}, {"Kind": "tag", "Pattern": "v*"}},
			"AllowPullRequests": true, "WaitMinutes": 30, "ApprovalTimeoutHours": 24, "AllowSelfApproval": true,
			"Reviewers":      map[string]any{"UserIds": []string{dev.String()}, "TeamIds": []string{}},
			"AllowedPlugins": []string{"ghcr.io/khaymah/"}, "RequirePinnedImages": true, "AllowAdminBypass": true,
			"SupersedePending": false,
		},
	}), http.StatusCreated)
	assert.Equal(t, "Before production", e.Description)
	assert.Equal(t, []refView{{Kind: "branch", Pattern: "release/*"}, {Kind: "tag", Pattern: "v*"}}, e.Rules.AllowedRefs)
	assert.Equal(t, []string{dev.String()}, e.Rules.Reviewers.UserIds)
	assert.Equal(t, 30, e.Rules.WaitMinutes)
	assert.Equal(t, 24, e.Rules.ApprovalTimeoutHours)
	assert.True(t, e.Rules.AllowSelfApproval)
	assert.True(t, e.Rules.AllowPullRequests)
	assert.True(t, e.Rules.RequirePinnedImages)
	assert.True(t, e.Rules.AllowAdminBypass)
	assert.False(t, e.Rules.SupersedePending)
	assert.Equal(t, []string{"ghcr.io/khaymah/"}, e.Rules.AllowedPlugins)
}

func TestInvalidInputIsRefused(t *testing.T) {
	w := newWorld(t)
	tooMany := make([]map[string]string, 0, 51)
	for i := range 51 {
		tooMany = append(tooMany, map[string]string{"Kind": "branch", "Pattern": "release/" + strconv.Itoa(i)})
	}
	long := strings.Repeat("a", 257)
	tests := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"an upper-case name", map[string]any{"Name": "Production"}, "Name"},
		{"a name with a slash", map[string]any{"Name": "prod/eu"}, "Name"},
		{"a pattern with ..", map[string]any{"Name": "p1", "Rules": map[string]any{
			"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": "release/../main"}},
		}}, "Rules.AllowedRefs[0].Pattern"},
		{"a pattern over the length limit", map[string]any{"Name": "p2", "Rules": map[string]any{
			"AllowedRefs": []map[string]string{{"Kind": "branch", "Pattern": long}},
		}}, "Rules.AllowedRefs[0].Pattern"},
		{"fifty-one patterns", map[string]any{"Name": "p3", "Rules": map[string]any{"AllowedRefs": tooMany}}, "Rules.AllowedRefs"},
		{"two required approvals", map[string]any{"Name": "p4", "Rules": map[string]any{"RequiredApprovals": 2}}, "Rules.RequiredApprovals"},
		{"a wait over thirty days", map[string]any{"Name": "p5", "Rules": map[string]any{"WaitMinutes": 43201}}, "Rules.WaitMinutes"},
		{"a long description", map[string]any{"Name": "p6", "Description": strings.Repeat("d", 501)}, "Description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := w.create(w.owner, tt.body)
			if res.Status == http.StatusBadRequest {
				requireError(t, res, http.StatusBadRequest, "BAD_REQUEST")
				return
			}
			requireError(t, res, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
			assert.Contains(t, detailFields(res), tt.field)
		})
	}
	var n int
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Environments"`).Scan(&n))
	assert.Zero(t, n)
}

func TestAnArchivedProjectGetsNoNewEnvironment(t *testing.T) {
	w := newWorld(t)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Projects" SET "State" = 'archived', "ArchivedAt" = now() WHERE "Id" = $1`, w.project)
	require.NoError(t, err)
	requireError(t, w.create(w.owner, map[string]any{"Name": "production"}), http.StatusConflict, "PROJECT_ARCHIVED")
}

func TestAnEnvironmentOfAnotherProjectOrOrganizationIsNotFound(t *testing.T) {
	w := newWorld(t)
	w.production(t)
	other := w.env.SeedProject(w.org, w.ownerID)
	res := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: w.path(other, "/production")})
	requireError(t, res, http.StatusNotFound, "NOT_FOUND")
	requireError(t, w.get(w.owner, "missing"), http.StatusNotFound, "NOT_FOUND")
	requireError(t, w.get(w.owner, "Not-Valid"), http.StatusNotFound, "NOT_FOUND")

	foreignOrg := w.env.SeedOrganization("Other", "other")
	outsider := w.env.SeedSession(w.env.SeedUser(foreignOrg, "o@example.com", memberPassword, "owner"))
	requireError(t, w.get(outsider, "production"), http.StatusNotFound, "NOT_FOUND")
	requireError(t, w.post(outsider, "production", "pause", map[string]string{"Reason": "no"}), http.StatusNotFound, "NOT_FOUND")
}

func TestRemoveIsSoftAndFreesTheName(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	requireError(t, w.post(w.owner, "production", "remove", map[string]any{"CurrentFactor": wrongFactor()}), http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	decode(t, w.get(w.owner, "production"), http.StatusOK)
	requireError(t, w.post(w.owner, "production", "remove", map[string]any{}), http.StatusForbidden, "INVALID_CURRENT_FACTOR")

	removed := decode(t, w.post(w.owner, "production", "remove", map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	assert.Equal(t, e.Id, removed.Id)
	requireError(t, w.get(w.owner, "production"), http.StatusNotFound, "NOT_FOUND")
	var deleted bool
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "IsDeleted" FROM "Environments" WHERE "Id" = $1`, e.Id).Scan(&deleted))
	assert.True(t, deleted, "the row stays, marked deleted")
	again := w.production(t)
	assert.NotEqual(t, e.Id, again.Id)
	assert.Equal(t, 1, w.env.AuditCount("environment.removed"))
}
