package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
)

func valuesOf(list []secretstore.Secret) map[string]string {
	out := map[string]string{}
	for _, s := range list {
		out[s.Name] = s.Scope + "=" + string(s.Value)
	}
	return out
}

func TestTheClosestScopeWins(t *testing.T) {
	w := newWorld(t)
	production := w.env.SeedEnvironment(w.project, "production")
	other := w.env.SeedProject(w.org, w.ownerID)
	put(w.owner, w.orgPath("/secrets/TOKEN"), map[string]string{"Value": "organization value"})
	put(w.owner, w.projectPath("/secrets/TOKEN"), map[string]string{"Value": "project value"})
	put(w.owner, w.envPath("production", "/secrets/TOKEN"), map[string]string{"Value": "environment value"})
	put(w.owner, w.orgPath("/secrets/PICKED"), map[string]any{"Value": "for other only", "Visibility": "selected_projects", "ProjectIds": []string{other.String()}})

	ctx := context.Background()
	got, err := w.service().Resolve(ctx, secretstore.Request{OrganizationID: w.org, ProjectID: w.project, EnvironmentID: &production, Names: []string{"TOKEN"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"TOKEN": "environment=environment value"}, valuesOf(got))

	got, err = w.service().Resolve(ctx, secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"token"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"token": "project=project value"}, valuesOf(got), "names match whatever their case")

	got, err = w.service().Resolve(ctx, secretstore.Request{OrganizationID: w.org, ProjectID: other, Names: []string{"TOKEN", "PICKED"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"TOKEN": "organization=organization value", "PICKED": "organization=for other only"}, valuesOf(got))

	res, err := w.service().Check(ctx, nil, secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"PICKED", "MISSING"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"PICKED", "MISSING"}, res.Missing, "an organization secret for other projects does not exist here")
	_, err = w.service().Resolve(ctx, secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"PICKED"}})
	require.ErrorIs(t, err, secretstore.ErrMissingName)

	res, err = w.service().Check(ctx, nil, secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"TOKEN"}, PullRequest: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"TOKEN"}, res.NotForPullRequests)
	put(w.owner, w.projectPath("/secrets/TOKEN"), map[string]any{"Value": "project value 2", "AvailableToPullRequests": true})
	res, err = w.service().Check(ctx, nil, secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"TOKEN"}, PullRequest: true})
	require.NoError(t, err)
	assert.True(t, res.OK())

	_, err = w.service().Resolve(ctx, secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"TOKEN"}, Fork: true})
	require.ErrorIs(t, err, secretstore.ErrFork, "a fork never reads a secret")

	listed := w.owner.Get(w.envPath("production", "/secrets"))
	var list []secretView
	listed.Decode(t, &list)
	require.Len(t, list, 1)
	assert.Equal(t, []string{"project", "organization"}, list[0].Overrides)
	project := w.owner.Get(w.projectPath("/secrets"))
	project.Decode(t, &list)
	assert.Equal(t, []string{"organization"}, list[0].Overrides)
	assert.NotNil(t, list[0].LastUsedAt, "a resolve marks the secret as used")
}

func TestVersionsAndRetention(t *testing.T) {
	w := newWorld(t)
	path := w.projectPath("/secrets/ROTATED")
	for i := 1; i <= 6; i++ {
		put(w.owner, path, map[string]string{"Value": "value number " + string(rune('0'+i))})
	}
	erased := func() []int {
		rows, err := w.env.Pool.Query(context.Background(),
			`SELECT v."Version" FROM "SecretVersions" v JOIN "Secrets" s ON s."Id" = v."SecretId"
			 WHERE s."Name" = 'ROTATED' AND v."ValueSealed" IS NULL ORDER BY v."Version"`)
		require.NoError(t, err)
		defer rows.Close()
		var out []int
		for rows.Next() {
			var v int
			require.NoError(t, rows.Scan(&v))
			out = append(out, v)
		}
		return out
	}
	assert.Equal(t, []int{1}, erased(), "the sixth version erases the first")
	requireError(t, post(w.owner, path+"/restore-version", map[string]int{"Version": 1}), http.StatusConflict, "SECRET_VERSION_ERASED")
	requireError(t, post(w.owner, path+"/restore-version", map[string]int{"Version": 9}), http.StatusNotFound, "NOT_FOUND")

	restored := secretOf(t, post(w.owner, path+"/restore-version", map[string]int{"Version": 3}), http.StatusOK)
	assert.Equal(t, 7, restored.Version)
	got, err := w.service().Resolve(context.Background(), secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"ROTATED"}})
	require.NoError(t, err)
	assert.Equal(t, "value number 3", string(got[0].Value), "the restored version has the old value")

	w.env.Clock.Advance(31 * 24 * time.Hour)
	_, err = w.env.Wired.Retention.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, erased(), "only the current version keeps its value after 30 days")
	got, err = w.service().Resolve(context.Background(), secretstore.Request{OrganizationID: w.org, ProjectID: w.project, Names: []string{"ROTATED"}})
	require.NoError(t, err)
	assert.Equal(t, "value number 3", string(got[0].Value))
}

func TestRemovingAnEnvironmentShredsItsSecrets(t *testing.T) {
	w := newWorld(t)
	res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.projectPath("/environments"), Body: map[string]any{
		"Name": "staging", "CurrentFactor": apptest.PasswordFactor(w.password),
	}})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var env struct{ Id string }
	res.Decode(t, &env)
	put(w.owner, w.envPath("staging", "/secrets/SSH_KEY"), map[string]string{"Value": "a private key"})
	put(w.owner, w.envPath("staging", "/variables/HOST"), map[string]string{"Value": "staging.example.com"})

	removed := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.envPath("staging", "/remove"), Body: map[string]any{
		"CurrentFactor": apptest.PasswordFactor(w.password),
	}})
	require.Equal(t, http.StatusOK, removed.Status, string(removed.Raw))
	var live, values, keys int
	ctx := context.Background()
	require.NoError(t, w.env.Pool.QueryRow(ctx, `SELECT count(*) FROM "Secrets" WHERE NOT "IsDeleted" AND "EnvironmentId" IS NOT NULL`).Scan(&live))
	require.NoError(t, w.env.Pool.QueryRow(ctx, `SELECT count(*) FROM "SecretVersions" WHERE "ValueSealed" IS NOT NULL`).Scan(&values))
	require.NoError(t, w.env.Pool.QueryRow(ctx,
		`SELECT count(*) FROM "DataKeys" WHERE "OwnerKind" = 'environment' AND "OwnerId" = $1 AND "State" <> 'destroyed'`, uuid.MustParse(env.Id)).Scan(&keys))
	assert.Zero(t, live)
	assert.Zero(t, values)
	assert.Zero(t, keys, "the data key of the environment is destroyed")
	var vars int
	require.NoError(t, w.env.Pool.QueryRow(ctx, `SELECT count(*) FROM "Variables" WHERE NOT "IsDeleted"`).Scan(&vars))
	assert.Zero(t, vars)
}

func TestVariablesWithOverrides(t *testing.T) {
	w := newWorld(t)
	put(w.owner, w.orgPath("/variables/REGION"), map[string]string{"Value": "eu"})
	res := put(w.owner, w.projectPath("/variables/region"), map[string]string{"Value": "us"})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var v variableView
	res.Decode(t, &v)
	assert.Equal(t, "us", v.Value)
	assert.Equal(t, []string{"organization"}, v.Overrides)
	requireError(t, put(w.owner, w.projectPath("/variables/BIG"), map[string]string{"Value": strings.Repeat("x", 4097)}), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	assert.Equal(t, 2, w.env.AuditCount("variable.created"))
}
