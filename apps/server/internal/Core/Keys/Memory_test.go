package keys_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

type memoryStore struct {
	mu      sync.Mutex
	active  int32
	rows    map[uuid.UUID]keys.DataKey
	inserts int
}

func newMemoryStore(active int32) *memoryStore {
	return &memoryStore{active: active, rows: map[uuid.UUID]keys.DataKey{}}
}

func (m *memoryStore) ActiveKekVersion(context.Context) (int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active, nil
}

func (m *memoryStore) DataKey(_ context.Context, id uuid.UUID) (keys.DataKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dk, ok := m.rows[id]
	if !ok {
		return keys.DataKey{}, keys.ErrNoDataKey
	}
	dk.Wrapped = slices.Clone(dk.Wrapped)
	return dk, nil
}

func (m *memoryStore) ActiveDataKey(_ context.Context, owner keys.Owner) (keys.DataKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, dk := range m.rows {
		if dk.Owner == owner && !dk.Destroyed {
			return dk, nil
		}
	}
	return keys.DataKey{}, keys.ErrNoDataKey
}

func (m *memoryStore) InsertDataKey(_ context.Context, dk keys.DataKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, other := range m.rows {
		if other.Owner == dk.Owner && !other.Destroyed {
			return nil
		}
	}
	m.inserts++
	m.rows[dk.ID] = dk
	return nil
}

func (m *memoryStore) DestroyOwner(_ context.Context, owner keys.Owner) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, dk := range m.rows {
		if dk.Owner == owner && !dk.Destroyed {
			dk.Destroyed, dk.Wrapped = true, nil
			m.rows[id] = dk
			n++
		}
	}
	return n, nil
}

func (m *memoryStore) activeRows() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, dk := range m.rows {
		if !dk.Destroyed {
			n++
		}
	}
	return n
}

func newMemoryEnvelope(t testing.TB) (*keys.Envelope, *memoryStore) {
	t.Helper()
	provider, err := keys.NewFake(1)
	require.NoError(t, err)
	store := newMemoryStore(1)
	return keys.NewEnvelope(store, provider), store
}
