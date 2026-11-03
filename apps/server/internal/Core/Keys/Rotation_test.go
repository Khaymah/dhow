package keys_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const (
	rotationOwners = 10_000
	rotationLabel  = "dhow/secret/v1"
	staleSeconds   = 90
)

var errKilled = errors.New("killed in the middle of the re-wrap")

type sealedValue struct {
	owner keys.Owner
	value []byte
}

func sealMany(t *testing.T, env *keys.Envelope, n int) []sealedValue {
	t.Helper()
	out := make([]sealedValue, n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for i := range jobs {
				owner := keys.ProjectOwner(uuid.New())
				s, err := env.Seal(context.Background(), owner, rotationLabel, fmt.Appendf(nil, "row:%d", i), fmt.Appendf(nil, "value %d", i))
				if !assert.NoError(t, err) {
					continue
				}
				out[i] = sealedValue{owner: owner, value: s}
			}
		})
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

func openAll(t *testing.T, env *keys.Envelope, values []sealedValue) {
	t.Helper()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	for range 16 {
		wg.Go(func() {
			for i := range jobs {
				got, err := env.Open(context.Background(), values[i].owner, rotationLabel, fmt.Appendf(nil, "row:%d", i), values[i].value)
				if err != nil || string(got) != fmt.Sprintf("value %d", i) {
					mu.Lock()
					failed++
					mu.Unlock()
				}
			}
		})
	}
	for i := range values {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	require.Zero(t, failed, "every value opens at every point of the rotation")
}

func registerInstance(t *testing.T, pool *pgxpool.Pool, key string, versions []int32, lastSeen time.Duration) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `INSERT INTO "ServerInstances" ("InstanceKey", "Hostname", "Version", "SchemaVersion", "KekVersions", "LastSeenAt")
		VALUES ($1, $1, 'test', 52, $2, now() - make_interval(secs => $3))`, key, versions, lastSeen.Seconds())
	require.NoError(t, err)
}

func countOn(t *testing.T, pool *pgxpool.Pool, version int32) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT count(*) FROM "DataKeys" WHERE "State" = 'active' AND "KekVersion" = $1`, version).Scan(&n))
	return n
}

func TestRotationReWrapsTenThousandDataKeysAcrossKillsAndRetiresTheOldVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("rotates 10,000 data keys")
	}
	pool := dbtest.NewPool(t)
	v1, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	old := managerFor(t, pool, v1)
	_, err = old.Register(t.Context())
	require.NoError(t, err)
	values := sealMany(t, old.Envelope(), rotationOwners)
	require.Equal(t, rotationOwners, countOn(t, pool, 1))

	both, version, err := v1.WithNextVersion()
	require.NoError(t, err)
	require.Equal(t, int32(2), version)
	fake, err := keys.NewFakeFrom(both)
	require.NoError(t, err)
	server := keys.NewManager(pool, fake, slog.New(slog.DiscardHandler))
	_, err = server.Register(t.Context())
	require.NoError(t, err)
	registerInstance(t, pool, "server-a", []int32{1, 2}, 0)
	registerInstance(t, pool, "server-b", []int32{1, 2}, 0)

	act, err := server.Activate(t.Context(), 2, staleSeconds)
	require.NoError(t, err)
	assert.Equal(t, keys.Activation{From: 1, To: 2}, act)
	assert.Equal(t, []kekRow{{1, keys.KekRetiring}, {2, keys.KekActive}}, kekRows(t, pool))
	openAll(t, server.Envelope(), values)

	for kill := 1; kill <= 3; kill++ {
		ctx, cancel := context.WithCancelCause(t.Context())
		start := fake.WrapCalls()
		fake.BeforeWrap(func() error {
			if fake.WrapCalls()-start >= keys.RewrapBatch+keys.RewrapBatch/2 {
				cancel(errKilled)
				return errKilled
			}
			return nil
		})
		_, err := server.Rewrap(ctx)
		require.Error(t, err, "kill %d", kill)
		cancel(nil)
		fake.BeforeWrap(nil)
		moved := countOn(t, pool, 2)
		assert.Equal(t, kill*keys.RewrapBatch, moved, "only whole batches commit (kill %d)", kill)
		assert.Equal(t, rotationOwners-moved, countOn(t, pool, 1))
		openAll(t, server.Envelope(), values)
	}

	res, err := server.Rewrap(t.Context())
	require.NoError(t, err)
	assert.Equal(t, rotationOwners-3*keys.RewrapBatch, res.Rewrapped)
	assert.Equal(t, []int32{1}, res.Retired)
	assert.Equal(t, []kekRow{{1, keys.KekRetired}, {2, keys.KekActive}}, kekRows(t, pool))
	assert.Zero(t, countOn(t, pool, 1))
	openAll(t, server.Envelope(), values)

	onlyV2 := keyringWithout(t, both, 1)
	fresh := managerFor(t, pool, onlyV2)
	_, err = fresh.Register(t.Context())
	require.NoError(t, err, "the retired version may leave the keyring file")
	openAll(t, fresh.Envelope(), values)

	st, err := fresh.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(2), st.Active)
	assert.Zero(t, st.ToRewrap)
	require.Len(t, st.Versions, 2)
	assert.Equal(t, int64(rotationOwners), st.Versions[1].DataKeys)
	assert.NotNil(t, st.LastRotatedAt)
	assert.Nil(t, st.LastExportedAt)
}

func TestActivationNeedsTheVersionOnEveryRunningServer(t *testing.T) {
	pool := dbtest.NewPool(t)
	v1, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	_, err = managerFor(t, pool, v1).Register(t.Context())
	require.NoError(t, err)
	both, _, err := v1.WithNextVersion()
	require.NoError(t, err)
	server := managerFor(t, pool, both)
	_, err = server.Register(t.Context())
	require.NoError(t, err)
	registerInstance(t, pool, "fresh", []int32{1, 2}, 0)
	registerInstance(t, pool, "old-binary", []int32{1}, 10*time.Second)
	registerInstance(t, pool, "long-gone", []int32{1}, 10*time.Minute)

	_, err = server.Activate(t.Context(), 2, staleSeconds)
	require.Error(t, err)
	assert.Equal(t, keys.CodeKekNotEverywhere, keys.CodeOf(err))
	var ne *keys.NotEverywhereError
	require.ErrorAs(t, err, &ne)
	assert.Equal(t, []string{"old-binary (old-binary)"}, ne.Instances)
	assert.NotContains(t, err.Error(), "long-gone")
	assert.Equal(t, []kekRow{{1, keys.KekActive}, {2, keys.KekPending}}, kekRows(t, pool), "nothing changes")

	_, err = pool.Exec(t.Context(), `UPDATE "ServerInstances" SET "KekVersions" = '{1,2}' WHERE "InstanceKey" = 'old-binary'`)
	require.NoError(t, err)
	_, err = server.Activate(t.Context(), 2, staleSeconds)
	require.NoError(t, err)
	assert.Equal(t, []kekRow{{1, keys.KekRetiring}, {2, keys.KekActive}}, kekRows(t, pool))
}

func TestActivationRefusesUnknownActiveRetiredAndUnloadedVersions(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1, 2)
	require.NoError(t, err)
	server := managerFor(t, pool, ring)
	_, err = server.Register(t.Context())
	require.NoError(t, err)

	_, err = server.Activate(t.Context(), 7, staleSeconds)
	assert.Equal(t, keys.CodeKekNotFound, keys.CodeOf(err))
	_, err = server.Activate(t.Context(), 2, staleSeconds)
	assert.Equal(t, keys.CodeKekAlreadyActive, keys.CodeOf(err))
	_, err = managerFor(t, pool, keyringWithout(t, ring, 1)).Activate(t.Context(), 1, staleSeconds)
	assert.Equal(t, keys.CodeKeyringMissingVersion, keys.CodeOf(err))

	_, err = server.Activate(t.Context(), 1, staleSeconds)
	require.NoError(t, err)
	res, err := server.Rewrap(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int32{2}, res.Retired)
	_, err = server.Activate(t.Context(), 2, staleSeconds)
	assert.Equal(t, keys.CodeKekRetired, keys.CodeOf(err))
}

func TestRewrapSkipsWhileAnotherServerHoldsTheLock(t *testing.T) {
	pool := dbtest.NewPool(t)
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	server := managerFor(t, pool, ring)
	_, err = server.Register(t.Context())
	require.NoError(t, err)

	tx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(t.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('dhow.keyring-rewrap', 0))`)
	require.NoError(t, err)
	res, err := server.Rewrap(t.Context())
	require.NoError(t, err)
	assert.True(t, res.Skipped)
}
