package keys_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

func TestASlowWriterThatTookTheOldKeyNeverEndsUpShredded(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 1, 1)
	project := uuid.Nil
	for _, p := range f.owners(t) {
		project = p
	}
	id := uuid.New()
	plain := []byte("written by a slow request")
	sealed := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		written <- pgx.BeginFunc(context.Background(), f.pool, func(tx pgx.Tx) error {
			value, err := f.column.SealIn(context.Background(), sqlc.New(tx), keys.ProjectOwner(project), rowContext(id.String()), plain)
			if err != nil {
				return err
			}
			close(sealed)
			time.Sleep(1500 * time.Millisecond)
			_, err = tx.Exec(context.Background(), `INSERT INTO "TestSecrets" ("Id", "ProjectId", "Sealed") VALUES ($1, $2, $3)`, id, project, value)
			return err
		})
	}()
	<-sealed
	_, err = f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Grace: 100 * time.Millisecond, Wait: waitShort})
	require.NoError(t, err)
	require.NoError(t, <-written)
	var value []byte
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT "Sealed" FROM "TestSecrets" WHERE "Id" = $1`, id).Scan(&value))
	got, err := f.column.Open(t.Context(), keys.ProjectOwner(project), rowContext(id.String()), value)
	require.NoError(t, err, "the value of the slow writer still opens after the rekey")
	assert.Equal(t, plain, got)
}

func waitShort(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func TestARekeySkipsAnOwnerThatIsDestroyedWhileItRuns(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 1, 3)
	var project uuid.UUID
	for _, p := range f.owners(t) {
		project = p
	}
	destroyed := make(chan struct{})
	destroy := func() {
		go func() {
			defer close(destroyed)
			_, err := f.manager.Envelope().Destroy(context.Background(), keys.ProjectOwner(project))
			assert.NoError(t, err)
		}()
	}
	wait := func() { <-destroyed }
	f.location.beforeReplace.Store(&destroy)
	f.location.beforeScan.Store(&wait)
	res, err := f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Batch: 1, Wait: noWait})
	require.NoError(t, err, "a value of a destroyed owner is skipped, not unreadable")
	assert.EqualValues(t, 2, res.Skipped)
}

func TestAMalformedValueUnderAReplacedKeyIsNamed(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 1, 2)
	var victim uuid.UUID
	for id := range f.plain {
		victim = id
		break
	}
	_, err = f.pool.Exec(t.Context(), `UPDATE "TestSecrets" SET "Sealed" = substring("Sealed" FROM 1 FOR 40) WHERE "Id" = $1`, victim)
	require.NoError(t, err)
	_, err = f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Wait: noWait})
	require.Error(t, err)
	assert.Equal(t, keys.CodeRekeyUnreadable, keys.CodeOf(err))
	assert.Contains(t, err.Error(), "TestSecrets.Sealed row "+victim.String())
}

func TestADataKeyCommittedAfterTheListingIsStillReplaced(t *testing.T) {
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	f := newRekeyFixture(t, provider, 1, 1)
	late := uuid.New()
	hook := func() {
		_, err := f.pool.Exec(context.Background(), `INSERT INTO "DataKeys" ("Id", "CreatedAt", "OwnerKind", "OwnerId", "KekVersion", "Wrapped")
			VALUES ($1, now() - interval '1 hour', 'project', $2, 1, $3)`, uuid.New(), late, make([]byte, 60))
		assert.NoError(t, err)
	}
	f.location.beforeReplace.Store(&hook)
	_, err = f.manager.Rekey(t.Context(), f.pool, f.values, f.registry, keys.RekeyOptions{Wait: noWait})
	require.NoError(t, err)
	var stale int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM "DataKeys" AS d, "KeyRekeys" AS r
		WHERE d."State" <> 'destroyed' AND d."CreatedAt" < r."Cutoff"`).Scan(&stale))
	assert.Zero(t, stale, "no data key from before the rekey survives it")
}
