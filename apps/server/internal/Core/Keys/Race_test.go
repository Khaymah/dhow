package keys_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
)

func TestANewDataKeyIsNeverLeftOnAVersionRetiredWhileItWasMade(t *testing.T) {
	pool := dbtest.NewPool(t)
	v1, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	_, err = managerFor(t, pool, v1).Register(t.Context())
	require.NoError(t, err)
	both, _, err := v1.WithNextVersion()
	require.NoError(t, err)
	fake, err := keys.NewFakeFrom(both)
	require.NoError(t, err)
	server := keys.NewManager(pool, fake, slog.New(slog.DiscardHandler))
	_, err = server.Register(t.Context())
	require.NoError(t, err)
	other := managerFor(t, pool, both)

	var once sync.Once
	var raced error
	fake.BeforeWrap(func() error {
		once.Do(func() {
			if _, err := other.Activate(t.Context(), 2, staleSeconds); err != nil {
				raced = err
				return
			}
			res, err := other.Rewrap(t.Context())
			if err == nil && !assert.Equal(t, []int32{1}, res.Retired) {
				err = errors.New("version 1 did not retire")
			}
			raced = err
		})
		return nil
	})
	owner := keys.EnvironmentOwner(uuid.New())
	sealed, err := server.Envelope().Seal(t.Context(), owner, "dhow/secret/v1", []byte("row"), []byte("value"))
	require.NoError(t, raced)
	require.NoError(t, err)

	var version int32
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "KekVersion" FROM "DataKeys" WHERE "OwnerId" = $1`, owner.ID).Scan(&version))
	assert.Equal(t, int32(2), version, "the data key is wrapped with the version that is active when it is stored")
	assert.Equal(t, []kekRow{{1, keys.KekRetired}, {2, keys.KekActive}}, kekRows(t, pool))
	got, err := managerFor(t, pool, keyringWithout(t, both, 1)).Envelope().Open(t.Context(), owner, "dhow/secret/v1", []byte("row"), sealed)
	require.NoError(t, err, "opens without the retired version")
	assert.Equal(t, "value", string(got))

	st, err := server.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int32{1}, st.Removable())
}

func TestARetiredVersionThatStillWrapsADataKeyIsNotRemovable(t *testing.T) {
	st := keys.Status{Versions: []keys.VersionStatus{
		{Version: 1, State: keys.KekRetired, Local: true, DataKeys: 3},
		{Version: 2, State: keys.KekRetired, Local: true},
		{Version: 3, State: keys.KekActive, Local: true},
	}}
	assert.Equal(t, []int32{2}, st.Removable())
}

func TestOneUnreadableDataKeyDoesNotStallTheRewrap(t *testing.T) {
	pool := dbtest.NewPool(t)
	v1, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	old := managerFor(t, pool, v1)
	_, err = old.Register(t.Context())
	require.NoError(t, err)
	values := sealMany(t, old.Envelope(), 20)
	var broken uuid.UUID
	require.NoError(t, pool.QueryRow(t.Context(), `UPDATE "DataKeys" SET "Wrapped" = decode(repeat('ab', 60), 'hex')
		WHERE "Id" = (SELECT "Id" FROM "DataKeys" ORDER BY "Id" LIMIT 1) RETURNING "Id"`).Scan(&broken))

	both, _, err := v1.WithNextVersion()
	require.NoError(t, err)
	var logs syncBuffer
	provider, err := keys.NewFileProvider(both)
	require.NoError(t, err)
	server := keys.NewManager(pool, provider, slog.New(slog.NewJSONHandler(&logs, nil)))
	_, err = server.Register(t.Context())
	require.NoError(t, err)
	_, err = server.Activate(t.Context(), 2, staleSeconds)
	require.NoError(t, err)

	res, err := server.Rewrap(t.Context())
	require.NoError(t, err, "a bad row is recorded and skipped")
	assert.Equal(t, 19, res.Rewrapped)
	assert.Equal(t, []uuid.UUID{broken}, res.Unreadable)
	assert.Empty(t, res.Retired, "a version that still has an unreadable data key never retires")
	again, err := server.Rewrap(t.Context())
	require.NoError(t, err)
	assert.Zero(t, again.Rewrapped)
	assert.Empty(t, again.Unreadable, "the bad row is not retried in a loop")
	assert.Equal(t, []kekRow{{1, keys.KekRetiring}, {2, keys.KekActive}}, kekRows(t, pool))

	st, err := server.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), st.Unreadable)
	assert.Equal(t, int64(1), st.Versions[0].Unreadable)
	assert.Empty(t, st.Removable())
	assert.Contains(t, logs.String(), broken.String())
	for _, l := range splitLines(both.Marshal()) {
		assert.NotContains(t, logs.String(), string(l.text[len(l.text)-44:len(l.text)-1]))
	}
	opened := 0
	for i, v := range values {
		if _, err := server.Envelope().Open(t.Context(), v.owner, rotationLabel, fmt.Appendf(nil, "row:%d", i), v.value); err == nil {
			opened++
		}
	}
	assert.Equal(t, 19, opened)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
