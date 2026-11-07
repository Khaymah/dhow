package retention_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	retention "github.com/Khaymah/dhow/apps/server/internal/Database/Retention"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

var now = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return now }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func random32(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

type seeded struct {
	user, org uuid.UUID
}

func seedOwner(t *testing.T, pool *pgxpool.Pool) seeded {
	t.Helper()
	var s seeded
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ('o@example.com', 'O', 'x') RETURNING "Id"`).Scan(&s.user))
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO "Organizations" ("Name", "Slug") VALUES ('Acme', 'acme') RETURNING "Id"`).Scan(&s.org))
	return s
}

func throttle(t *testing.T, pool *pgxpool.Pool, table, key string, lastFailure time.Time, lockedUntil *time.Time) {
	t.Helper()
	exec(t, pool, `INSERT INTO "`+table+`" ("Key", "Failures", "LastFailureAt", "LockedUntil", "CreatedAt", "UpdatedAt") VALUES ($1, 3, $2, $3, $2, $2)`,
		key, lastFailure, lockedUntil)
}

func challenge(t *testing.T, pool *pgxpool.Pool, expires time.Time) {
	t.Helper()
	exec(t, pool, `INSERT INTO "AuthChallenges" ("Kind", "ExpiresAt") VALUES ('passkey_login', $1)`, expires)
}

func flow(t *testing.T, pool *pgxpool.Pool, expires time.Time) {
	t.Helper()
	exec(t, pool, `INSERT INTO "OidcFlows" ("Purpose", "Provider", "TokenHash", "StateHash", "DeviceHash", "ExpiresAt") VALUES ('login', 'corp', $1, $2, $3, $4)`,
		random32(t), random32(t), random32(t), expires)
}

func invitation(t *testing.T, pool *pgxpool.Pool, s seeded, email string, expires time.Time, accepted, revoked *time.Time) {
	t.Helper()
	var acceptedBy *uuid.UUID
	if accepted != nil {
		acceptedBy = &s.user
	}
	exec(t, pool, `INSERT INTO "Invitations" ("OrganizationId", "Email", "Role", "TokenHash", "InvitedByUserId", "ExpiresAt", "AcceptedAt", "AcceptedByUserId", "RevokedAt")
		VALUES ($1, $2, 'viewer', $3, $4, $5, $6, $7, $8)`, s.org, email, random32(t), s.user, expires, accepted, acceptedBy, revoked)
}

func setupToken(t *testing.T, pool *pgxpool.Pool, expires time.Time) {
	t.Helper()
	exec(t, pool, `INSERT INTO "SetupTokens" ("TokenHash", "ExpiresAt") VALUES ($1, $2)`, random32(t), expires)
}

func rateLimit(t *testing.T, pool *pgxpool.Pool, key string, expires time.Time) {
	t.Helper()
	exec(t, pool, `INSERT INTO "RateLimits" ("Key", "WindowStart", "Hits", "ExpiresAt") VALUES ($1, $2, 1, $3)`, key, expires.Add(-time.Minute), expires)
}

func ptr(t time.Time) *time.Time { return &t }

func TestRetentionDeletesOnlyRowsPastTheirPeriod(t *testing.T) {
	pool := dbtest.NewPool(t)
	s := seedOwner(t, pool)
	day, month := 24*time.Hour, 30*24*time.Hour
	throttle(t, pool, "LoginThrottles", "ip:old", now.Add(-day-time.Minute), nil)
	throttle(t, pool, "LoginThrottles", "ip:recent", now.Add(-day+time.Minute), nil)
	throttle(t, pool, "LoginThrottles", "ip:still-locked", now.Add(-2*day), ptr(now.Add(time.Hour)))
	throttle(t, pool, "RunnerThrottles", "register:ip:old", now.Add(-2*day), nil)
	throttle(t, pool, "RunnerThrottles", "register:ip:recent", now.Add(-time.Hour), nil)
	challenge(t, pool, now.Add(-day-time.Minute))
	challenge(t, pool, now.Add(-time.Hour))
	flow(t, pool, now.Add(-day-time.Minute))
	flow(t, pool, now.Add(-time.Minute))
	invitation(t, pool, s, "expired-long-ago@example.com", now.Add(-month-time.Hour), nil, nil)
	invitation(t, pool, s, "expired-lately@example.com", now.Add(-month+time.Hour), nil, nil)
	invitation(t, pool, s, "revoked-long-ago@example.com", now.Add(day), nil, ptr(now.Add(-month-time.Hour)))
	invitation(t, pool, s, "accepted-long-ago@example.com", now.Add(-2*month), ptr(now.Add(-2*month-day)), nil)
	invitation(t, pool, s, "open@example.com", now.Add(6*day), nil, nil)
	setupToken(t, pool, now.Add(-day-time.Minute))
	setupToken(t, pool, now.Add(-time.Minute))
	rateLimit(t, pool, "ip-oidc:a", now.Add(-time.Hour-time.Minute))
	rateLimit(t, pool, "ip-oidc:b", now.Add(-time.Minute))
	audited := count(t, pool, `SELECT count(*) FROM "AuditEvents"`)

	report, err := retention.New(retention.Options{Pool: pool, Log: quiet(), Now: clock}).Run(context.Background())
	require.NoError(t, err)
	require.True(t, report.Ran)
	assert.Equal(t, map[string]int64{
		"LoginThrottles": 1, "RunnerThrottles": 1, "AuthChallenges": 1, "OidcFlows": 1, "Invitations": 2, "SetupTokens": 1, "RateLimits": 1, "NotificationOutbox": 0, "SecretVersions": 0,
	}, report.Deleted)

	keys := func(table string) []string {
		rows, err := pool.Query(context.Background(), `SELECT "Key" FROM "`+table+`" ORDER BY "Key"`)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var k string
			require.NoError(t, rows.Scan(&k))
			out = append(out, k)
		}
		return out
	}
	assert.Equal(t, []string{"ip:recent", "ip:still-locked"}, keys("LoginThrottles"), "a key that is still locked stays")
	assert.Equal(t, []string{"register:ip:recent"}, keys("RunnerThrottles"))
	assert.Equal(t, []string{"ip-oidc:b"}, keys("RateLimits"))
	assert.Equal(t, 1, count(t, pool, `SELECT count(*) FROM "AuthChallenges"`))
	assert.Equal(t, 1, count(t, pool, `SELECT count(*) FROM "OidcFlows"`))
	assert.Equal(t, 1, count(t, pool, `SELECT count(*) FROM "SetupTokens"`))
	assert.Equal(t, 0, count(t, pool, `SELECT count(*) FROM "Invitations" WHERE "Email" IN ('expired-long-ago@example.com', 'revoked-long-ago@example.com')`))
	assert.Equal(t, 3, count(t, pool, `SELECT count(*) FROM "Invitations" WHERE "Email" IN ('expired-lately@example.com', 'accepted-long-ago@example.com', 'open@example.com')`),
		"an accepted invitation is history and stays")
	assert.Equal(t, audited, count(t, pool, `SELECT count(*) FROM "AuditEvents"`), "the audit log is never touched")
}

func TestRetentionDeletesInSmallBatches(t *testing.T) {
	pool := dbtest.NewPool(t)
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		throttle(t, pool, "LoginThrottles", "ip:"+k, now.Add(-48*time.Hour), nil)
	}
	r := retention.New(retention.Options{Pool: pool, Log: quiet(), Now: clock, Batch: 2, MaxBatches: 2})
	report, err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(4), report.Deleted["LoginThrottles"], "two batches of two, the rest waits for the next run")
	assert.Equal(t, 2, report.Batches["LoginThrottles"])
	assert.Equal(t, 1, count(t, pool, `SELECT count(*) FROM "LoginThrottles"`))

	report, err = r.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Deleted["LoginThrottles"])
	assert.Equal(t, 1, report.Batches["LoginThrottles"], "a short batch ends the table")
	assert.Equal(t, 1, report.Batches["RateLimits"], "an empty table costs one statement")
}

func TestOnlyOneInstanceRunsRetentionAtATime(t *testing.T) {
	pool := dbtest.NewPool(t)
	throttle(t, pool, "LoginThrottles", "ip:old", now.Add(-48*time.Hour), nil)
	other, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	defer other.Release()
	var locked bool
	require.NoError(t, other.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, config.RetentionLockKey).Scan(&locked))
	require.True(t, locked)

	r := retention.New(retention.Options{Pool: pool, Log: quiet(), Now: clock})
	report, err := r.Run(context.Background())
	require.NoError(t, err)
	assert.False(t, report.Ran, "another instance holds the lock")
	assert.Equal(t, 1, count(t, pool, `SELECT count(*) FROM "LoginThrottles"`))

	_, err = other.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, config.RetentionLockKey)
	require.NoError(t, err)
	report, err = r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, report.Ran)
	assert.Zero(t, count(t, pool, `SELECT count(*) FROM "LoginThrottles"`))
}

func TestParallelRunsNeverOverlap(t *testing.T) {
	pool := dbtest.NewPool(t)
	for i := range 30 {
		throttle(t, pool, "LoginThrottles", "ip:"+uuid.NewString()+string(rune('a'+i%26)), now.Add(-48*time.Hour), nil)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ran int
	var deleted int64
	for range 8 {
		wg.Go(func() {
			r := retention.New(retention.Options{Pool: pool, Log: quiet(), Now: clock, Batch: 1})
			report, err := r.Run(context.Background())
			assert.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			if report.Ran {
				ran++
			}
			deleted += report.Deleted["LoginThrottles"]
		})
	}
	wg.Wait()
	assert.GreaterOrEqual(t, ran, 1)
	assert.Equal(t, int64(30), deleted, "every row is deleted exactly once")
	assert.Zero(t, count(t, pool, `SELECT count(*) FROM "LoginThrottles"`))
}

func TestARowInUseIsSkipped(t *testing.T) {
	pool := dbtest.NewPool(t)
	throttle(t, pool, "LoginThrottles", "ip:busy", now.Add(-48*time.Hour), nil)
	tx, err := pool.Begin(context.Background())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(context.Background(), `SELECT 1 FROM "LoginThrottles" WHERE "Key" = 'ip:busy' FOR UPDATE`)
	require.NoError(t, err)

	report, err := retention.New(retention.Options{Pool: pool, Log: quiet(), Now: clock}).Run(context.Background())
	require.NoError(t, err)
	assert.Zero(t, report.Deleted["LoginThrottles"], "a sign-in holding the row keeps it")
}

func TestRetentionCoversTheFixedTableList(t *testing.T) {
	assert.Equal(t, []string{"LoginThrottles", "RunnerThrottles", "AuthChallenges", "OidcFlows", "Invitations", "SetupTokens", "RateLimits", "NotificationOutbox", "SecretVersions"}, retention.Tables())
}

func TestWithoutAClockRetentionUsesTheDatabaseTime(t *testing.T) {
	pool := dbtest.NewPool(t)
	var dbNow time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT now()`).Scan(&dbNow))
	exec(t, pool, `INSERT INTO "RateLimits" ("Key", "WindowStart", "Hits", "ExpiresAt") VALUES ('old', now() - interval '3 hours', 1, now() - interval '2 hours')`)
	exec(t, pool, `INSERT INTO "RateLimits" ("Key", "WindowStart", "Hits", "ExpiresAt") VALUES ('new', now() - interval '1 minute', 1, now() + interval '9 minutes')`)

	report, err := retention.New(retention.Options{Pool: pool, Log: quiet()}).Run(context.Background())
	require.NoError(t, err)
	assert.WithinDuration(t, dbNow, report.Now, 5*time.Second)
	assert.Equal(t, int64(1), report.Deleted["RateLimits"])
}

func outboxRow(t *testing.T, pool *pgxpool.Pool, s seeded, state string, updated time.Time) {
	t.Helper()
	exec(t, pool, `INSERT INTO "NotificationOutbox" ("OrganizationId", "RecipientUserId", "Kind", "EventId", "EventName", "Payload", "State", "UpdatedAt")
		VALUES ($1, $2, 'internal-email', gen_random_uuid(), 'x', '{}', $3, $4)`, s.org, s.user, state, updated)
}

func TestRetentionKeepsTheNotificationLogForTheConfiguredDays(t *testing.T) {
	pool := dbtest.NewPool(t)
	s := seedOwner(t, pool)
	day := 24 * time.Hour
	outboxRow(t, pool, s, "sent", now.Add(-31*day))
	outboxRow(t, pool, s, "dead", now.Add(-31*day))
	outboxRow(t, pool, s, "sent", now.Add(-29*day))
	outboxRow(t, pool, s, "pending", now.Add(-90*day))

	report, err := retention.New(retention.Options{Pool: pool, Log: quiet(), Now: clock}).Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(2), report.Deleted["NotificationOutbox"])
	assert.Equal(t, 2, count(t, pool, `SELECT count(*) FROM "NotificationOutbox"`), "a recent row and a waiting row stay, however old")

	report, err = retention.New(retention.Options{Pool: pool, Log: quiet(), Now: clock, NotifyDays: 7}).Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Deleted["NotificationOutbox"], "a shorter setting cleans the 29 day old row")
	assert.Equal(t, 1, count(t, pool, `SELECT count(*) FROM "NotificationOutbox"`))
}
