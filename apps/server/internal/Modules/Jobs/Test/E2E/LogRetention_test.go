package e2e_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
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

const totpStep = 30 * time.Second

type owned struct {
	world
	fresh func() map[string]any
}

func newOwnedWorld(t *testing.T) owned {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	enrollment := owner.EnableTotp(apptest.PasswordFactor(d.Password))
	w := world{env: env, owner: owner, ownerID: uuid.MustParse(owner.UserID), org: env.OrganizationID(d.OrgSlug)}
	return owned{world: w, fresh: func() map[string]any {
		env.Clock.Advance(totpStep)
		return apptest.CodeFactor("totp", env.TotpCode(enrollment.Secret, 0))
	}}
}

func setRetention(t *testing.T, c *apptest.Client, org uuid.UUID, days any, factor map[string]any) apptest.Response {
	t.Helper()
	return c.Do(apptest.Request{
		Method: http.MethodPut, Path: apptest.OrgPath(org, "/log-retention"),
		Body: map[string]any{"LogRetentionDays": days, "CurrentFactor": factor},
	})
}

func TestAnOrganizationCanKeepItsLogsForFewerDays(t *testing.T) {
	o := newOwnedWorld(t)
	w := o.world
	id, _, g := w.running(t)
	w.oldChunk(t, g.Grant.GetAttemptId(), 9)

	text, _ := w.allText(t, id)
	assert.Contains(t, text, "old secret line", "the server setting keeps 30 days")

	res := setRetention(t, w.owner, w.org, 7, o.fresh())
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var org struct{ LogRetentionDays *int }
	res.Decode(t, &org)
	require.NotNil(t, org.LogRetentionDays)
	assert.Equal(t, 7, *org.LogRetentionDays)
	text, _ = w.allText(t, id)
	assert.NotContains(t, text, "old secret line", "older logs stop showing at once")
	assert.Equal(t, 1, w.env.AuditCount("identity.organization.updated"))

	n, err := w.env.Wired.Jobs.ScrubOrganizationLogs(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	var data []byte
	var deleted bool
	require.NoError(t, w.env.Pool.QueryRow(t.Context(),
		`SELECT "Data", "IsDeleted" FROM "LogChunks" WHERE "AttemptId" = $1 AND "Seq" = 100`, g.Grant.GetAttemptId()).Scan(&data, &deleted))
	assert.True(t, deleted)
	assert.Equal(t, []byte{0}, data, "the content is erased, not only hidden")

	res = setRetention(t, w.owner, w.org, nil, o.fresh())
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var cleared struct{ LogRetentionDays *int }
	res.Decode(t, &cleared)
	assert.Nil(t, cleared.LogRetentionDays, "null follows the server setting again")
}

func TestOnlyAnOwnerWithAFreshSecondFactorShortensLogRetention(t *testing.T) {
	o := newOwnedWorld(t)
	w := o.world
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	res := setRetention(t, admin, w.org, 30, apptest.PasswordFactor("a long password for admin@example.com"))
	assert.Equal(t, http.StatusForbidden, res.Status, "an admin cannot erase the logs: %s", res.Raw)
	res = setRetention(t, w.owner, w.org, 30, map[string]any{"Method": "totp", "Code": "000000"})
	require.NotNil(t, res.Error, string(res.Raw))
	assert.Equal(t, "INVALID_CURRENT_FACTOR", res.Error.Code)
	for _, v := range []any{1, 6, -1, 3651, "7"} {
		res := setRetention(t, w.owner, w.org, v, o.fresh())
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, res.Status, "%v: %s", v, res.Raw)
	}
	assert.Zero(t, w.env.AuditCount("identity.organization.updated"))
}

func TestAnOwnerWithoutASecondFactorCannotShortenLogRetention(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	res := setRetention(t, owner, env.OrganizationID(d.OrgSlug), 30, apptest.PasswordFactor(d.Password))
	require.NotNil(t, res.Error, string(res.Raw))
	assert.Equal(t, "SECOND_FACTOR_REQUIRED", res.Error.Code)
}

func TestTheRetentionChangeAndItsAuditEntryAreWrittenTogether(t *testing.T) {
	o := newOwnedWorld(t)
	w := o.world
	_, err := w.env.Pool.Exec(t.Context(), `CREATE FUNCTION "FailAudit"() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'audit unavailable'; END $$`)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(t.Context(), `CREATE TRIGGER "FailAudit" BEFORE INSERT ON "AuditEvents" FOR EACH ROW EXECUTE FUNCTION "FailAudit"()`)
	require.NoError(t, err)
	res := setRetention(t, w.owner, w.org, 7, o.fresh())
	assert.Equal(t, http.StatusInternalServerError, res.Status, string(res.Raw))
	var days *int32
	require.NoError(t, w.env.Pool.QueryRow(t.Context(), `SELECT "LogRetentionDays" FROM "Organizations" WHERE "Id" = $1`, w.org).Scan(&days))
	assert.Nil(t, days, "a change that could not be audited is not kept")
}

func TestScrubbingOneOrganizationLeavesTheOthers(t *testing.T) {
	w := newWorld(t)
	_, _, g := w.running(t)
	w.oldChunk(t, g.Grant.GetAttemptId(), 9)
	n, err := w.env.Wired.Jobs.ScrubOrganizationLogs(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "an organization without its own retention keeps its logs")
}
