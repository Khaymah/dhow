package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditingNeedsAFreshFactorAndTheCurrentVersion(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	rules := map[string]any{"WaitMinutes": 10}

	requireError(t, w.update(w.owner, "production", map[string]any{"Version": e.Version, "Rules": rules}), http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	requireError(t, w.update(w.owner, "production", map[string]any{"Version": e.Version, "Rules": rules, "CurrentFactor": wrongFactor()}),
		http.StatusForbidden, "INVALID_CURRENT_FACTOR")
	unchanged := decode(t, w.get(w.owner, "production"), http.StatusOK)
	assert.Equal(t, e, unchanged, "a refused edit changes nothing")

	changed := decode(t, w.update(w.owner, "production", map[string]any{"Version": e.Version, "Rules": rules, "CurrentFactor": w.ownerFactor()}),
		http.StatusOK)
	assert.Equal(t, 10, changed.Rules.WaitMinutes)
	assert.Equal(t, e.Version+1, changed.Version)
	assert.Equal(t, e.RuleVersion+1, changed.RuleVersion)
	assert.Equal(t, e.Rules.AllowedRefs, changed.Rules.AllowedRefs, "fields that were not sent keep their value")

	stale := w.update(w.owner, "production", map[string]any{"Version": e.Version, "Rules": map[string]any{"WaitMinutes": 20}, "CurrentFactor": w.ownerFactor()})
	requireError(t, stale, http.StatusConflict, "ENVIRONMENT_VERSION_CONFLICT")
	assert.Equal(t, 10, decode(t, w.get(w.owner, "production"), http.StatusOK).Rules.WaitMinutes)

	requireError(t, w.update(w.owner, "production", map[string]any{"Version": changed.Version, "CurrentFactor": w.ownerFactor()}),
		http.StatusBadRequest, "BAD_REQUEST")
}

func TestTheRuleVersionRisesWithEveryRuleChangeOnly(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	described := decode(t, w.update(w.owner, "production", map[string]any{
		"Version": e.Version, "Description": "Live", "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	assert.Equal(t, e.RuleVersion, described.RuleVersion, "the description is not a rule")
	assert.Equal(t, e.Version+1, described.Version)

	same := decode(t, w.update(w.owner, "production", map[string]any{
		"Version": described.Version, "Rules": map[string]any{"WaitMinutes": 0}, "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	assert.Equal(t, described.Version, same.Version, "sending the current value changes nothing")
	assert.Equal(t, described.RuleVersion, same.RuleVersion)

	steps := []map[string]any{
		{"AllowedRefs": []map[string]string{{"Kind": "tag", "Pattern": "v*"}}},
		{"AllowPullRequests": true},
		{"AllowSelfApproval": true},
		{"ApprovalTimeoutHours": 1},
		{"RequirePinnedImages": true},
		{"AllowedPlugins": []string{"ghcr.io/khaymah/"}},
		{"AllowAdminBypass": true},
		{"SupersedePending": false},
	}
	current := same
	for _, rules := range steps {
		next := decode(t, w.update(w.owner, "production", map[string]any{
			"Version": current.Version, "Rules": rules, "CurrentFactor": w.ownerFactor(),
		}), http.StatusOK)
		assert.Equal(t, current.RuleVersion+1, next.RuleVersion, "%v", rules)
		current = next
	}
	paused := decode(t, w.post(w.owner, "production", "pause", map[string]string{"Reason": "Incident 42"}), http.StatusOK)
	assert.Equal(t, current.RuleVersion+1, paused.RuleVersion, "pausing is a rule change")
	resumed := decode(t, w.post(w.owner, "production", "resume", map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	assert.Equal(t, paused.RuleVersion+1, resumed.RuleVersion, "resuming is a rule change")
}

func TestReviewersMustBeEligibleMembers(t *testing.T) {
	w := newWorld(t)
	_, developer := w.member(t, "developer")
	_, viewer := w.member(t, "viewer")
	_, raised := w.member(t, "viewer")
	w.env.SeedUserProjectBinding(w.org, w.project, raised, "developer")
	_, lowered := w.member(t, "maintainer")
	w.env.SeedUserProjectBinding(w.org, w.project, lowered, "viewer")
	foreignOrg := w.env.SeedOrganization("Other", "other")
	outsider := w.env.SeedUser(foreignOrg, "o@example.com", memberPassword, "owner")
	lone := w.env.SeedUserWithoutMembership("lone@example.com", memberPassword)
	var team, foreignTeam uuid.UUID
	ctx := context.Background()
	require.NoError(t, w.env.Pool.QueryRow(ctx, `INSERT INTO "Teams" ("OrganizationId", "Name") VALUES ($1, 'Ops') RETURNING "Id"`, w.org).Scan(&team))
	require.NoError(t, w.env.Pool.QueryRow(ctx, `INSERT INTO "Teams" ("OrganizationId", "Name") VALUES ($1, 'Theirs') RETURNING "Id"`, foreignOrg).
		Scan(&foreignTeam))

	reviewers := func(users []uuid.UUID, teams []uuid.UUID) map[string]any {
		return map[string]any{"Reviewers": map[string]any{"UserIds": users, "TeamIds": teams}}
	}
	ok := decode(t, w.create(w.owner, map[string]any{
		"Name": "production", "Rules": reviewers([]uuid.UUID{developer, raised, w.ownerID}, []uuid.UUID{team}),
	}), http.StatusCreated)
	assert.ElementsMatch(t, []string{developer.String(), raised.String(), w.ownerID.String()}, ok.Rules.Reviewers.UserIds)
	assert.Equal(t, []string{team.String()}, ok.Rules.Reviewers.TeamIds)

	tests := []struct {
		name  string
		users []uuid.UUID
		teams []uuid.UUID
		field string
	}{
		{"a viewer", []uuid.UUID{viewer}, nil, "Rules.Reviewers.UserIds[0]"},
		{"a maintainer lowered to viewer on the project", []uuid.UUID{developer, lowered}, nil, "Rules.Reviewers.UserIds[1]"},
		{"a member of another organization", []uuid.UUID{outsider}, nil, "Rules.Reviewers.UserIds[0]"},
		{"a user without any membership", []uuid.UUID{lone}, nil, "Rules.Reviewers.UserIds[0]"},
		{"a user who does not exist", []uuid.UUID{uuid.New()}, nil, "Rules.Reviewers.UserIds[0]"},
		{"a team of another organization", nil, []uuid.UUID{foreignTeam}, "Rules.Reviewers.TeamIds[0]"},
		{"a team that does not exist", nil, []uuid.UUID{uuid.New()}, "Rules.Reviewers.TeamIds[0]"},
	}
	current := ok
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := w.create(w.owner, map[string]any{"Name": "staging", "Rules": reviewers(tt.users, tt.teams)})
			requireError(t, res, http.StatusUnprocessableEntity, "REVIEWER_NOT_ELIGIBLE")
			assert.Contains(t, detailFields(res), tt.field)

			res = w.update(w.owner, "production", map[string]any{
				"Version": current.Version, "Rules": reviewers(tt.users, tt.teams), "CurrentFactor": w.ownerFactor(),
			})
			requireError(t, res, http.StatusUnprocessableEntity, "REVIEWER_NOT_ELIGIBLE")
		})
	}
	assert.Equal(t, ok, decode(t, w.get(w.owner, "production"), http.StatusOK), "a refused reviewer changes nothing")
	requireError(t, w.get(w.owner, "staging"), http.StatusNotFound, "NOT_FOUND")

	_, err := w.env.Pool.Exec(ctx, `UPDATE "Memberships" SET "Role" = 'viewer' WHERE "UserId" = $1`, developer)
	require.NoError(t, err)
	edited := decode(t, w.update(w.owner, "production", map[string]any{
		"Version": ok.Version, "Description": "Reviewers are checked when they change", "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	assert.Len(t, edited.Rules.Reviewers.UserIds, 3)

	cleared := decode(t, w.update(w.owner, "production", map[string]any{
		"Version": edited.Version, "Rules": reviewers([]uuid.UUID{}, []uuid.UUID{}), "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	assert.Empty(t, cleared.Rules.Reviewers.UserIds)
	assert.Empty(t, cleared.Rules.Reviewers.TeamIds)
	var kept int
	require.NoError(t, w.env.Pool.QueryRow(ctx, `SELECT count(*) FROM "EnvironmentReviewers" WHERE "EnvironmentId" = $1 AND "IsDeleted"`, ok.Id).
		Scan(&kept))
	assert.Equal(t, 4, kept, "removed reviewers are marked deleted, not deleted")
}

type auditRow struct {
	Data map[string]string
}

func auditOf(t *testing.T, w world, action string) []auditRow {
	t.Helper()
	rows, err := w.env.Pool.Query(context.Background(), `SELECT "Data" FROM "AuditEvents" WHERE "Action" = $1 ORDER BY "Seq"`, action)
	require.NoError(t, err)
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		var r auditRow
		require.NoError(t, json.Unmarshal(raw, &r.Data))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestEveryChangeIsAudited(t *testing.T) {
	w := newWorld(t)
	e := w.production(t)
	created := auditOf(t, w, "environment.created")
	require.Len(t, created, 1)
	assert.Equal(t, "production", created[0].Data["Name"])
	assert.Equal(t, w.project.String(), created[0].Data["ProjectId"])

	decode(t, w.update(w.owner, "production", map[string]any{
		"Version": e.Version, "Rules": map[string]any{"WaitMinutes": 5, "AllowSelfApproval": true}, "CurrentFactor": w.ownerFactor(),
	}), http.StatusOK)
	updated := auditOf(t, w, "environment.updated")
	require.Len(t, updated, 1)
	assert.Equal(t, "AllowSelfApproval,WaitMinutes", updated[0].Data["Fields"])
	assert.Equal(t, "0 -> 5", updated[0].Data["WaitMinutes"])
	assert.Equal(t, "false -> true", updated[0].Data["AllowSelfApproval"])
	assert.NotContains(t, updated[0].Data, "AllowAdminBypass", "only the changed rules")
	assert.Equal(t, "2", updated[0].Data["Version"])

	_, maintainer := w.member(t, "maintainer")
	client := w.env.SeedSession(maintainer)
	decode(t, w.post(client, "production", "pause", map[string]string{"Reason": "Bad release"}), http.StatusOK)
	decode(t, w.post(client, "production", "pause", map[string]string{"Reason": "Again"}), http.StatusOK)
	paused := auditOf(t, w, "environment.paused")
	require.Len(t, paused, 1, "pausing a paused environment writes nothing")
	assert.Equal(t, "Bad release", paused[0].Data["Reason"])
	var actor string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "ActorUserId"::text FROM "AuditEvents" WHERE "Action" = 'environment.paused'`).Scan(&actor))
	assert.Equal(t, maintainer.String(), actor)

	decode(t, w.post(w.owner, "production", "resume", map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	decode(t, w.post(w.owner, "production", "resume", map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	assert.Len(t, auditOf(t, w, "environment.resumed"), 1, "resuming an environment that runs writes nothing")

	decode(t, w.post(w.owner, "production", "remove", map[string]any{"CurrentFactor": w.ownerFactor()}), http.StatusOK)
	removed := auditOf(t, w, "environment.removed")
	require.Len(t, removed, 1)
	assert.Equal(t, "production", removed[0].Data["Name"])
}
