package e2e_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

func (w world) oldChunk(t *testing.T, attempt string, daysAgo int) {
	t.Helper()
	day := time.Now().UTC().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour)
	_, err := w.env.Pool.Exec(t.Context(),
		`DO $$ BEGIN EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF "LogChunks" FOR VALUES FROM (%L) TO (%L)', `+
			`"LogPartitionName"('`+day.Format("2006-01-02")+`'::date), '`+day.Format("2006-01-02")+` 00:00:00+00', '`+
			day.AddDate(0, 0, 1).Format("2006-01-02")+` 00:00:00+00'); END $$`)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(t.Context(),
		`INSERT INTO "LogChunks" ("OrganizationId", "JobId", "AttemptId", "Seq", "Stream", "Data", "CreatedAt")
		 SELECT a."OrganizationId", a."JobId", a."Id", 100, 'stdout', 'old secret line', $2
		 FROM "JobAttempts" a WHERE a."Id" = $1`, attempt, day.Add(time.Hour))
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(t.Context(), `UPDATE "JobAttempts" SET "LeasedAt" = $2, "LastSeq" = 100 WHERE "Id" = $1`, attempt, day)
	require.NoError(t, err)
}

func (w world) setRetention(t *testing.T, days any) apptest.Response {
	t.Helper()
	return w.owner.Do(apptest.Request{Method: http.MethodPut, Path: w.path("/log-retention"), Body: map[string]any{"LogRetentionDays": days}})
}

func TestAnOrganizationCanKeepItsLogsForFewerDays(t *testing.T) {
	w := newWorld(t)
	id, _, g := w.running(t)
	w.oldChunk(t, g.Grant.GetAttemptId(), 5)

	text, _ := w.allText(t, id)
	assert.Contains(t, text, "old secret line", "the server setting keeps 30 days")

	res := w.setRetention(t, 2)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var org struct{ LogRetentionDays *int }
	res.Decode(t, &org)
	require.NotNil(t, org.LogRetentionDays)
	assert.Equal(t, 2, *org.LogRetentionDays)
	text, _ = w.allText(t, id)
	assert.NotContains(t, text, "old secret line", "older logs stop showing at once")

	n, err := w.env.Wired.Jobs.ScrubOrganizationLogs(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	var data []byte
	var deleted bool
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT "Data", "IsDeleted" FROM "LogChunks" WHERE "AttemptId" = $1 AND "Seq" = 100`, g.Grant.GetAttemptId()).Scan(&data, &deleted))
	assert.True(t, deleted)
	assert.Equal(t, []byte{0}, data, "the content is erased, not only hidden")

	res = w.setRetention(t, nil)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var cleared struct{ LogRetentionDays *int }
	res.Decode(t, &cleared)
	assert.Nil(t, cleared.LogRetentionDays, "null follows the server setting again")
}

func TestLogRetentionRefusesValuesOutOfRange(t *testing.T) {
	w := newWorld(t)
	for _, v := range []any{0, -1, 3651, "7"} {
		res := w.setRetention(t, v)
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, res.Status, "%v: %s", v, res.Raw)
	}
}

func TestScrubbingOneOrganizationLeavesTheOthers(t *testing.T) {
	w := newWorld(t)
	_, _, g := w.running(t)
	w.oldChunk(t, g.Grant.GetAttemptId(), 5)
	n, err := w.env.Wired.Jobs.ScrubOrganizationLogs(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "an organization without its own retention keeps its logs")
}
