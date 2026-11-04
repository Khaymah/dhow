package edge_test

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func TestConcurrentWritesMakeOneSecretWithContiguousVersions(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	project := env.SeedProject(org, uuid.MustParse(owner.UserID))

	const writers = 16
	var wg sync.WaitGroup
	statuses := make([]int, writers)
	for i := range writers {
		wg.Go(func() {
			name := "RACED"
			if i%2 == 1 {
				name = "raced"
			}
			res := owner.Do(apptest.Request{
				Method: http.MethodPut, Path: apptest.OrgPath(org, "/projects/"+project.String()+"/secrets/"+name),
				Body: map[string]string{"Value": "value from writer " + strconv.Itoa(i)},
			})
			statuses[i] = res.Status
		})
	}
	wg.Wait()
	for i, s := range statuses {
		assert.Equal(t, http.StatusOK, s, "writer %d", i)
	}
	var secrets, versions, top int
	ctx := context.Background()
	require.NoError(t, env.Pool.QueryRow(ctx, `SELECT count(*) FROM "Secrets" WHERE lower("Name") = 'raced' AND NOT "IsDeleted"`).Scan(&secrets))
	require.NoError(t, env.Pool.QueryRow(ctx,
		`SELECT count(*), max(v."Version") FROM "SecretVersions" v JOIN "Secrets" s ON s."Id" = v."SecretId" WHERE lower(s."Name") = 'raced'`).Scan(&versions, &top))
	assert.Equal(t, 1, secrets)
	assert.Equal(t, writers, versions)
	assert.Equal(t, writers, top)
}

func TestConcurrentCreatesStopAtTheLimit(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	ownerID := uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	_, err := env.Pool.Exec(context.Background(),
		`INSERT INTO "Secrets" ("OrganizationId", "ProjectId", "Scope", "Name", "CreatedByUserId", "UpdatedByUserId")
		 SELECT $1, $2, 'project', 'FILL_' || n, $3, $3 FROM generate_series(1, 195) AS n`, org, project, ownerID)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			owner.Do(apptest.Request{
				Method: http.MethodPut, Path: apptest.OrgPath(org, "/projects/"+project.String()+"/secrets/NEW_"+strconv.Itoa(i)),
				Body: map[string]string{"Value": "a long value"},
			})
		})
	}
	wg.Wait()
	var n int
	require.NoError(t, env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "Secrets" WHERE "ProjectId" = $1 AND NOT "IsDeleted"`, project).Scan(&n))
	assert.Equal(t, 200, n, "the limit holds under concurrent creates")
}

func TestAWriteWaitsForAnEnvironmentBeingRemoved(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	project := env.SeedProject(org, uuid.MustParse(owner.UserID))
	envID := env.SeedEnvironment(project, "production")
	ctx := context.Background()
	tx, err := env.Pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `UPDATE "Environments" SET "IsDeleted" = true WHERE "Id" = $1`, envID)
	require.NoError(t, err)
	done := make(chan apptest.Response, 1)
	go func() {
		done <- owner.Do(apptest.Request{Method: http.MethodPut,
			Path: apptest.OrgPath(org, "/projects/"+project.String()+"/environments/production/secrets/LATE"),
			Body: map[string]string{"Value": "written during the removal"}})
	}()
	select {
	case res := <-done:
		t.Fatalf("the write did not wait for the removal: %d %s", res.Status, res.Raw)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Commit(ctx))
	res := <-done
	assert.Equal(t, http.StatusNotFound, res.Status, string(res.Raw))
	var n int
	require.NoError(t, env.Pool.QueryRow(ctx, `SELECT count(*) FROM "Secrets" WHERE "EnvironmentId" = $1`, envID).Scan(&n))
	assert.Zero(t, n)
}
