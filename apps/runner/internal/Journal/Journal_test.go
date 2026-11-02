package journal_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	journal "github.com/Khaymah/dhow/apps/runner/internal/Journal"
)

var fencing atomic.Int64

func entry() journal.Entry {
	return journal.Entry{LeaseID: uuid.NewString(), AttemptID: uuid.NewString(), JobID: uuid.NewString(), FencingToken: 41 + fencing.Add(1)}
}

func TestEntriesSurviveAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	j, err := journal.Open(path)
	require.NoError(t, err)
	a, b := entry(), entry()
	require.NoError(t, j.Add(a))
	require.NoError(t, j.Add(b))
	require.NoError(t, j.SetCompletion(b.LeaseID, journal.Completion{Result: 1}))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	again, err := journal.Open(path)
	require.NoError(t, err)
	got := again.Entries()
	require.Len(t, got, 2)
	for _, e := range got {
		if e.LeaseID == b.LeaseID {
			require.NotNil(t, e.Completion)
			assert.EqualValues(t, 1, e.Completion.Result)
		} else {
			assert.Nil(t, e.Completion)
		}
	}
	require.NoError(t, again.Remove(a.LeaseID))
	require.NoError(t, again.Remove(a.LeaseID), "removing twice is fine")
	assert.Len(t, again.Entries(), 1)
}

func TestAResultKeepsItsOutputsWithinBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	j, err := journal.Open(path)
	require.NoError(t, err)
	e := entry()
	require.NoError(t, j.Add(e))
	outputs := map[string]string{"version": "1.2.3"}
	require.NoError(t, j.SetCompletion(e.LeaseID, journal.Completion{Result: 1, Outputs: outputs}))
	outputs["version"] = "changed"
	again, err := journal.Open(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"version": "1.2.3"}, again.Entries()[0].Completion.Outputs)

	many := map[string]string{}
	for i := range journal.MaxOutputs + 1 {
		many[uuid.NewString()] = strconv.Itoa(i)
	}
	require.ErrorIs(t, j.SetCompletion(e.LeaseID, journal.Completion{Result: 1, Outputs: many}), journal.ErrInvalid)
	big := map[string]string{"big": strings.Repeat("a", journal.MaxOutputBytes)}
	require.ErrorIs(t, j.SetCompletion(e.LeaseID, journal.Completion{Result: 1, Outputs: big}), journal.ErrInvalid)
}

func TestTheJournalHoldsOnlyIDsAndFencing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	j, err := journal.Open(path)
	require.NoError(t, err)
	e := entry()
	e.FencingToken = 1 << 40
	require.NoError(t, j.Add(e))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, key := range []string{"Version", "HighWater", "Leases", "LeaseId", "AttemptId", "JobId", "FencingToken"} {
		assert.Contains(t, string(raw), key)
	}
	stripped := string(raw)
	for _, v := range []string{e.LeaseID, e.AttemptID, e.JobID, "1099511627776"} {
		stripped = strings.ReplaceAll(stripped, v, "")
	}
	for _, key := range []string{"Version", "HighWater", "Leases", "LeaseId", "AttemptId", "JobId", "FencingToken", "1"} {
		stripped = strings.ReplaceAll(stripped, key, "")
	}
	assert.Equal(t, `{"":,"":,"":[{"":"","":"","":"","":}]}`, stripped)
}

func TestReplayedOrOlderFencingTokensAreRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	j, err := journal.Open(path)
	require.NoError(t, err)
	newer := entry()
	newer.FencingToken = 100
	require.NoError(t, j.Add(newer))
	assert.ErrorIs(t, j.Add(newer), journal.ErrExists, "the same lease twice")
	older := entry()
	older.FencingToken = 99
	assert.ErrorIs(t, j.Add(older), journal.ErrReplay)
	same := entry()
	same.FencingToken = 100
	assert.ErrorIs(t, j.Add(same), journal.ErrReplay)

	require.NoError(t, j.Remove(newer.LeaseID))
	again, err := journal.Open(path)
	require.NoError(t, err)
	assert.EqualValues(t, 100, again.HighWater(), "the mark survives removal and a reopen")
	assert.ErrorIs(t, again.Add(older), journal.ErrReplay)
	next := entry()
	next.FencingToken = 101
	assert.NoError(t, again.Add(next))
}

func TestInvalidEntriesAndDamageAreRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	j, err := journal.Open(path)
	require.NoError(t, err)
	bad := entry()
	bad.FencingToken = 0
	assert.ErrorIs(t, j.Add(bad), journal.ErrInvalid)
	bad = entry()
	bad.LeaseID = "../../etc"
	assert.ErrorIs(t, j.Add(bad), journal.ErrInvalid)

	require.NoError(t, os.WriteFile(path, []byte(`{"Version":1,"Leases":[{"LeaseId":"x"}]}`), 0o600))
	_, err = journal.Open(path)
	assert.ErrorIs(t, err, journal.ErrCorrupt)
	require.NoError(t, os.Chmod(path, 0o644))
	_, err = journal.Open(path)
	assert.ErrorIs(t, err, config.ErrOpenMode)
}

func FuzzJournalOpen(f *testing.F) {
	f.Add([]byte(`{"Version":1,"Leases":[]}`))
	f.Add([]byte(`{"Version":1,"Leases":[{"LeaseId":"0190d0f8-8a3f-7000-8000-000000000001"}]}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		path := filepath.Join(t.TempDir(), "journal")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		j, err := journal.Open(path)
		if err != nil {
			return
		}
		for _, e := range j.Entries() {
			if e.FencingToken < 1 {
				t.Fatalf("an invalid entry was accepted: %+v", e)
			}
		}
	})
}
