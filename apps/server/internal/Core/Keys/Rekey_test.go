package keys_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

const labelSecret = "dhow/test-secret/v1"

type tableLocation struct {
	spec      keys.Spec
	failAfter *atomic.Int64
}

var errInjected = errors.New("injected failure")

func newTableLocation(t *testing.T, pool *pgxpool.Pool) tableLocation {
	t.Helper()
	_, err := pool.Exec(t.Context(), `CREATE TABLE "TestSecrets" ("Id" uuid PRIMARY KEY, "ProjectId" uuid NOT NULL, "Sealed" bytea NOT NULL)`)
	require.NoError(t, err)
	return tableLocation{spec: keys.Spec{
		Table: "TestSecrets", Column: "Sealed", Label: labelSecret, Owner: keys.OwnerProject, MaxPlaintext: 64, MaxSealed: 256,
	}, failAfter: &atomic.Int64{}}
}

func rowContext(id string) []byte { return []byte("test-secret:" + id) }

func (l tableLocation) Spec() keys.Spec { return l.spec }

func (l tableLocation) Scan(ctx context.Context, db sqlc.DBTX, after string, limit int32, legacyOnly bool) ([]keys.Value, error) {
	start := uuid.Nil
	if after != "" {
		start = uuid.MustParse(after)
	}
	rows, err := db.Query(ctx, `SELECT "Id", "ProjectId", "Sealed" FROM "TestSecrets"
		WHERE "Id" > $1 AND (NOT $2 OR get_byte("Sealed", 0) = 1) ORDER BY "Id" LIMIT $3 FOR UPDATE SKIP LOCKED`, start, legacyOnly, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []keys.Value
	for rows.Next() {
		var id, project uuid.UUID
		var sealed []byte
		if err := rows.Scan(&id, &project, &sealed); err != nil {
			return nil, err
		}
		out = append(out, keys.Value{Key: id.String(), Owner: keys.ProjectOwner(project), Context: rowContext(id.String()), Sealed: sealed})
	}
	return out, rows.Err()
}

func (l tableLocation) Replace(ctx context.Context, db sqlc.DBTX, v keys.Value, sealed []byte) (bool, error) {
	if l.failAfter.Load() > 0 && l.failAfter.Add(-1) == 0 {
		return false, errInjected
	}
	tag, err := db.Exec(ctx, `UPDATE "TestSecrets" SET "Sealed" = $1 WHERE "Id" = $2 AND "Sealed" = $3`, sealed, uuid.MustParse(v.Key), v.Sealed)
	return tag.RowsAffected() == 1, err
}

func (l tableLocation) CountLegacy(ctx context.Context, db sqlc.DBTX) (int64, error) {
	var n int64
	err := db.QueryRow(ctx, `SELECT count(*) FROM "TestSecrets" WHERE get_byte("Sealed", 0) = 1`).Scan(&n)
	return n, err
}

func (l tableLocation) CountUnder(ctx context.Context, db sqlc.DBTX, ids [][]byte) (int64, error) {
	var n int64
	err := db.QueryRow(ctx, `SELECT count(*) FROM "TestSecrets"
		WHERE get_byte("Sealed", 0) = 3 AND substring("Sealed" FROM 2 FOR 16) = ANY($1::bytea[])`, ids).Scan(&n)
	return n, err
}

type rekeyFixture struct {
	pool     *pgxpool.Pool
	manager  *keys.Manager
	column   *keys.Column
	location tableLocation
	registry *keys.Registry
	values   *keys.Values
	plain    map[uuid.UUID][]byte
}

func newRekeyFixture(t *testing.T, provider keys.KeyProvider, projects, perProject int) *rekeyFixture {
	t.Helper()
	pool := dbtest.NewPool(t)
	m := keys.NewManager(pool, provider, nil)
	_, err := m.Register(t.Context())
	require.NoError(t, err)
	loc := newTableLocation(t, pool)
	reg, err := keys.NewRegistry(loc)
	require.NoError(t, err)
	values := keys.NewValues(m.Envelope(), nil)
	col, err := values.Column(loc.Spec())
	require.NoError(t, err)
	f := &rekeyFixture{pool: pool, manager: m, column: col, location: loc, registry: reg, values: values, plain: map[uuid.UUID][]byte{}}
	for range projects {
		project := uuid.New()
		for range perProject {
			id := uuid.New()
			plain := fmt.Appendf(nil, "secret of %s", id)
			sealed, err := col.Seal(t.Context(), keys.ProjectOwner(project), rowContext(id.String()), plain)
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO "TestSecrets" ("Id", "ProjectId", "Sealed") VALUES ($1, $2, $3)`, id, project, sealed)
			require.NoError(t, err)
			f.plain[id] = plain
		}
	}
	return f
}

func (f *rekeyFixture) rows(t *testing.T) map[uuid.UUID][]byte {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT "Id", "Sealed" FROM "TestSecrets"`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[uuid.UUID][]byte{}
	for rows.Next() {
		var id uuid.UUID
		var sealed []byte
		require.NoError(t, rows.Scan(&id, &sealed))
		out[id] = sealed
	}
	require.NoError(t, rows.Err())
	return out
}

func (f *rekeyFixture) owners(t *testing.T) map[uuid.UUID]uuid.UUID {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT "Id", "ProjectId" FROM "TestSecrets"`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[uuid.UUID]uuid.UUID{}
	for rows.Next() {
		var id, project uuid.UUID
		require.NoError(t, rows.Scan(&id, &project))
		out[id] = project
	}
	return out
}

func (f *rekeyFixture) openAll(t *testing.T, col *keys.Column) {
	t.Helper()
	owners := f.owners(t)
	for id, sealed := range f.rows(t) {
		got, err := col.Open(t.Context(), keys.ProjectOwner(owners[id]), rowContext(id.String()), sealed)
		require.NoError(t, err, "row %s", id)
		require.Equal(t, f.plain[id], got)
	}
}

func dataKeyIDs(t *testing.T, rows map[uuid.UUID][]byte) map[uuid.UUID]bool {
	t.Helper()
	out := map[uuid.UUID]bool{}
	for _, sealed := range rows {
		id, ok := keys.DataKeyIDOf(sealed)
		require.True(t, ok)
		out[id] = true
	}
	return out
}

func noWait(context.Context, time.Duration) bool { return true }

func TestRekeyGivesEveryOwnerANewDataKeyAndDestroysTheOldOnes(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 3, 4)
	before := dataKeyIDs(t, f.rows(t))
	require.Len(t, before, 3)

	res, err := f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Grace: 0, Batch: 5, Wait: noWait})
	require.NoError(t, err)
	assert.False(t, res.Resumed)
	assert.EqualValues(t, 3, res.Superseded)
	assert.EqualValues(t, 12, res.Resealed)
	assert.EqualValues(t, 3, res.Destroyed)

	after := dataKeyIDs(t, f.rows(t))
	require.Len(t, after, 3)
	for id := range after {
		assert.False(t, before[id], "no value keeps an old data key")
	}
	f.openAll(t, f.column)
	var destroyed int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM "DataKeys" WHERE "State" = 'destroyed' AND "Wrapped" IS NULL`).Scan(&destroyed))
	assert.Equal(t, 3, destroyed)
	var open int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM "KeyRekeys" WHERE "DoneAt" IS NULL`).Scan(&open))
	assert.Zero(t, open)

	again, err := f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Wait: noWait})
	require.NoError(t, err)
	assert.EqualValues(t, 3, again.Superseded, "a second rekey replaces the keys again")
}

func TestAnOldKeyringWithACopyFromBeforeTheRekeyOpensNoCurrentValue(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	oldProvider, err := keys.NewFakeFrom(ring)
	require.NoError(t, err)
	both, _, err := ring.WithNextVersion()
	require.NoError(t, err)
	provider, err := keys.NewFakeFrom(both)
	require.NoError(t, err)
	f := newRekeyFixture(t, oldProvider, 2, 3)
	copyOfDataKeys := snapshotDataKeys(t, f.pool)

	m := keys.NewManager(f.pool, provider, nil)
	_, err = m.Register(t.Context())
	require.NoError(t, err)
	_, err = m.Activate(t.Context(), 2, 90)
	require.NoError(t, err)
	_, err = m.Rewrap(t.Context())
	require.NoError(t, err)
	values := keys.NewValues(m.Envelope(), nil)
	_, err = m.Rekey(t.Context(), f.pool, values, f.registry, keys.RekeyOptions{Wait: noWait})
	require.NoError(t, err)
	current, err := values.Column(f.location.Spec())
	require.NoError(t, err)
	f.openAll(t, current)

	stolen := keys.NewEnvelope(copyOfDataKeys, oldProvider)
	stolenColumn, err := keys.NewValues(stolen, nil).Column(f.location.Spec())
	require.NoError(t, err)
	withCurrentKeys := keys.NewEnvelope(keys.NewPostgresStore(f.pool), oldProvider)
	currentColumn, err := keys.NewValues(withCurrentKeys, nil).Column(f.location.Spec())
	require.NoError(t, err)
	owners := f.owners(t)
	for id, sealed := range f.rows(t) {
		_, err := stolenColumn.Open(t.Context(), keys.ProjectOwner(owners[id]), rowContext(id.String()), sealed)
		require.ErrorIs(t, err, keys.ErrSealed, "the old copy does not hold the new data key")
		_, err = currentColumn.Open(t.Context(), keys.ProjectOwner(owners[id]), rowContext(id.String()), sealed)
		require.Error(t, err, "the old keyring does not unwrap a data key wrapped with the new KEK")
	}
}

func TestAnInterruptedRekeyResumesWhereItStopped(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 4, 5)
	before := dataKeyIDs(t, f.rows(t))
	f.location.failAfter.Store(7)

	res, err := f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Batch: 3, Wait: noWait})
	require.ErrorIs(t, err, errInjected)
	assert.EqualValues(t, 4, res.Superseded)
	f.openAll(t, f.column)
	var superseded int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM "DataKeys" WHERE "State" = 'superseded'`).Scan(&superseded))
	assert.Equal(t, 4, superseded, "the old keys stay readable until every value moved")

	res, err = f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Batch: 3, Wait: noWait})
	require.NoError(t, err)
	assert.True(t, res.Resumed)
	assert.Zero(t, res.Superseded, "the resumed run does not replace the new keys again")
	assert.EqualValues(t, 4, res.Destroyed)
	for id := range dataKeyIDs(t, f.rows(t)) {
		assert.False(t, before[id])
	}
	f.openAll(t, f.column)
}

func TestARekeyStopsBeforeDestroyingAKeyThatAnUnreadableValueUses(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 1, 2)
	var victim uuid.UUID
	for id := range f.plain {
		victim = id
		break
	}
	_, err = f.pool.Exec(t.Context(), `UPDATE "TestSecrets" SET "Sealed" = set_byte("Sealed", 80, get_byte("Sealed", 80) # 255) WHERE "Id" = $1`, victim)
	require.NoError(t, err)

	_, err = f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Wait: noWait})
	require.Error(t, err)
	assert.Equal(t, keys.CodeRekeyUnreadable, keys.CodeOf(err))
	assert.Contains(t, err.Error(), victim.String())
	var destroyed int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM "DataKeys" WHERE "State" = 'destroyed'`).Scan(&destroyed))
	assert.Zero(t, destroyed)
}

func TestOnlyOneRekeyRunsAtATime(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 1, 1)
	conn, err := f.pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	locked, err := sqlc.New(conn).TryLockKeyRekey(t.Context())
	require.NoError(t, err)
	require.True(t, locked)
	_, err = f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Wait: noWait})
	assert.Equal(t, keys.CodeRekeyRunning, keys.CodeOf(err))
	_, err = sqlc.New(conn).UnlockKeyRekey(t.Context())
	require.NoError(t, err)
}

type snapshotStore struct {
	keys.Store
	rows map[uuid.UUID]keys.DataKey
}

func (s snapshotStore) DataKey(_ context.Context, id uuid.UUID) (keys.DataKey, error) {
	dk, ok := s.rows[id]
	if !ok {
		return keys.DataKey{}, keys.ErrNoDataKey
	}
	return dk, nil
}

func snapshotDataKeys(t *testing.T, pool *pgxpool.Pool) snapshotStore {
	t.Helper()
	store := keys.NewPostgresStore(pool)
	rows, err := pool.Query(t.Context(), `SELECT "Id" FROM "DataKeys"`)
	require.NoError(t, err)
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	rows.Close()
	out := snapshotStore{Store: store, rows: map[uuid.UUID]keys.DataKey{}}
	for _, id := range ids {
		dk, err := store.DataKey(t.Context(), id)
		require.NoError(t, err)
		out.rows[id] = dk
	}
	return out
}

func TestAReplacedKeyIsRewrappedOnRotationAndShreddedWithItsOwner(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	oldProvider, err := keys.NewFakeFrom(ring)
	require.NoError(t, err)
	f := newRekeyFixture(t, oldProvider, 2, 2)
	f.location.failAfter.Store(1)
	_, err = f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Wait: noWait})
	require.ErrorIs(t, err, errInjected)

	both, _, err := ring.WithNextVersion()
	require.NoError(t, err)
	provider, err := keys.NewFakeFrom(both)
	require.NoError(t, err)
	m := keys.NewManager(f.pool, provider, nil)
	_, err = m.Register(t.Context())
	require.NoError(t, err)
	_, err = m.Activate(t.Context(), 2, 90)
	require.NoError(t, err)
	res, err := m.Rewrap(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 4, res.Rewrapped, "two replaced and two new data keys move to the new KEK")
	assert.Equal(t, []int32{1}, res.Retired)
	values := keys.NewValues(m.Envelope(), nil)
	current, err := values.Column(f.location.Spec())
	require.NoError(t, err)
	f.openAll(t, current)

	owners := f.owners(t)
	var victim uuid.UUID
	for _, project := range owners {
		victim = project
		break
	}
	n, err := m.Envelope().Destroy(t.Context(), keys.ProjectOwner(victim))
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "the replaced key goes with the new one")
	for id, sealed := range f.rows(t) {
		if owners[id] != victim {
			continue
		}
		_, err := current.Open(t.Context(), keys.ProjectOwner(victim), rowContext(id.String()), sealed)
		require.ErrorIs(t, err, keys.ErrShredded)
	}
}
