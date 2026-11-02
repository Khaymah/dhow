package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
)

const (
	fileVersion    = 1
	MaxOutputs     = 64
	MaxOutputBytes = 128 << 10
)

var (
	ErrCorrupt = errors.New("the lease journal is damaged; it lists lease and attempt IDs and unreported results, so inspect it and remove it to start over")
	ErrInvalid = errors.New("a journal entry needs a lease id, an attempt id, a job id and a positive fencing token")
	ErrReplay  = errors.New("the fencing token is not above the highest one this runner has seen, so the grant is a replay")
	ErrExists  = errors.New("the lease is already in the journal")
)

type Completion struct {
	Result        int32             `json:"Result"`
	ExitCode      int32             `json:"ExitCode"`
	FailureReason int32             `json:"FailureReason"`
	Outputs       map[string]string `json:"Outputs,omitempty"`
}

type Entry struct {
	LeaseID      string      `json:"LeaseId"`
	AttemptID    string      `json:"AttemptId"`
	JobID        string      `json:"JobId"`
	FencingToken int64       `json:"FencingToken"`
	Completion   *Completion `json:"Completion,omitempty"`
}

type fileFormat struct {
	Version   int     `json:"Version"`
	HighWater int64   `json:"HighWater"`
	Leases    []Entry `json:"Leases"`
}

type Journal struct {
	path      string
	mu        sync.Mutex
	entries   map[string]Entry
	highWater int64
}

func Open(path string) (*Journal, error) {
	j := &Journal{path: path, entries: map[string]Entry{}}
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return j, nil
	}
	raw, err := config.ReadPrivateFile(path)
	if err != nil {
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != fileVersion {
		return nil, ErrCorrupt
	}
	if f.HighWater < 0 {
		return nil, ErrCorrupt
	}
	j.highWater = f.HighWater
	for _, e := range f.Leases {
		if e.validate() != nil {
			return nil, ErrCorrupt
		}
		j.entries[e.LeaseID] = e
		j.highWater = max(j.highWater, e.FencingToken)
	}
	return j, nil
}

func (e Entry) validate() error {
	for _, id := range []string{e.LeaseID, e.AttemptID, e.JobID} {
		if _, err := uuid.Parse(id); err != nil {
			return ErrInvalid
		}
	}
	if e.FencingToken < 1 {
		return ErrInvalid
	}
	if c := e.Completion; c != nil && (len(c.Outputs) > MaxOutputs || outputBytes(c.Outputs) > MaxOutputBytes) {
		return ErrInvalid
	}
	return nil
}

func outputBytes(outputs map[string]string) int {
	n := 0
	for k, v := range outputs {
		n += len(k) + len(v)
	}
	return n
}

func (j *Journal) Add(e Entry) error {
	if err := e.validate(); err != nil {
		return err
	}
	e.Completion = nil
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.entries[e.LeaseID]; ok {
		return ErrExists
	}
	if e.FencingToken <= j.highWater {
		return ErrReplay
	}
	next := j.copyEntries()
	next[e.LeaseID] = e
	return j.writeWith(next, e.FencingToken)
}

func (j *Journal) HighWater() int64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.highWater
}

func (j *Journal) SetCompletion(leaseID string, c Completion) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[leaseID]
	if !ok {
		return nil
	}
	c.Outputs = maps.Clone(c.Outputs)
	e.Completion = &c
	if err := e.validate(); err != nil {
		return err
	}
	next := j.copyEntries()
	next[leaseID] = e
	return j.write(next)
}

func (j *Journal) Remove(leaseID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.entries[leaseID]; !ok {
		return nil
	}
	next := j.copyEntries()
	delete(next, leaseID)
	return j.write(next)
}

func (j *Journal) Entries() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]Entry, 0, len(j.entries))
	for _, e := range j.entries {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Entry) int { return strings.Compare(a.LeaseID, b.LeaseID) })
	return out
}

func (j *Journal) copyEntries() map[string]Entry {
	out := make(map[string]Entry, len(j.entries)+1)
	for k, v := range j.entries {
		out[k] = v
	}
	return out
}

func (j *Journal) write(next map[string]Entry) error { return j.writeWith(next, j.highWater) }

func (j *Journal) writeWith(next map[string]Entry, highWater int64) error {
	f := fileFormat{Version: fileVersion, HighWater: highWater, Leases: make([]Entry, 0, len(next))}
	for _, e := range next {
		f.Leases = append(f.Leases, e)
	}
	slices.SortFunc(f.Leases, func(a, b Entry) int { return strings.Compare(a.LeaseID, b.LeaseID) })
	raw, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode the lease journal: %w", err)
	}
	if err := config.WriteFileAtomic(j.path, raw); err != nil {
		return err
	}
	j.entries, j.highWater = next, highWater
	return nil
}
