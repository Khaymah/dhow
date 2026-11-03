package edge_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const parallel = 12

type versioned struct{ Version, RuleVersion int64 }

func setup(t *testing.T) (*apptest.Env, *apptest.Client, string, string) {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	project := env.SeedProject(org, uuid.MustParse(owner.UserID))
	return env, owner, apptest.OrgPath(org, "/projects/"+project.String()+"/environments"), d.Password
}

func statuses(t *testing.T, n int, do func(i int) apptest.Response) map[string]int {
	t.Helper()
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := map[string]int{}
	for i := range n {
		wg.Go(func() {
			res := do(i)
			key := http.StatusText(res.Status)
			if res.Error != nil {
				key = res.Error.Code
			}
			mu.Lock()
			out[key]++
			mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

func TestRaceOnlyOneEditWinsTheSameVersion(t *testing.T) {
	env, owner, path, password := setup(t)
	res := owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{"Name": "production"}})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var start versioned
	res.Decode(t, &start)

	got := statuses(t, parallel, func(i int) apptest.Response {
		return owner.Do(apptest.Request{Method: http.MethodPut, Path: path + "/production", Body: map[string]any{
			"Version": start.Version, "Rules": map[string]any{"WaitMinutes": i + 1}, "CurrentFactor": apptest.PasswordFactor(password),
		}})
	})
	assert.Equal(t, 1, got["OK"], "%v", got)
	assert.Equal(t, parallel-1, got["ENVIRONMENT_VERSION_CONFLICT"]+got["INVALID_CURRENT_FACTOR"],
		"the others lose on the version, or on the throttle of factor attempts: %v", got)

	var end versioned
	require.NoError(t, env.Pool.QueryRow(context.Background(),
		`SELECT "Version", "RuleVersion" FROM "Environments" WHERE "Name" = 'production'`).Scan(&end.Version, &end.RuleVersion))
	assert.Equal(t, versioned{start.Version + 1, start.RuleVersion + 1}, end)
	assert.Equal(t, 1, env.AuditCount("environment.updated"))
}

func TestRaceOnlyOneCreateWinsTheSameName(t *testing.T) {
	env, owner, path, _ := setup(t)
	got := statuses(t, parallel, func(int) apptest.Response {
		return owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{"Name": "production"}})
	})
	assert.Equal(t, map[string]int{"Created": 1, "ENVIRONMENT_EXISTS": parallel - 1}, got)
	assert.Equal(t, 1, env.AuditCount("environment.created"))
}

func TestRaceOnePauseOneAuditEvent(t *testing.T) {
	env, owner, path, _ := setup(t)
	res := owner.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: map[string]any{"Name": "production"}})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	got := statuses(t, parallel, func(int) apptest.Response {
		return owner.Do(apptest.Request{Method: http.MethodPost, Path: path + "/production/pause", Body: map[string]any{"Reason": "Incident"}})
	})
	assert.Equal(t, map[string]int{"OK": parallel}, got)
	assert.Equal(t, 1, env.AuditCount("environment.paused"))
	var ruleVersion int64
	require.NoError(t, env.Pool.QueryRow(context.Background(), `SELECT "RuleVersion" FROM "Environments"`).Scan(&ruleVersion))
	assert.Equal(t, int64(2), ruleVersion)
}
