package events

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"

	"github.com/google/uuid"

	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

type entry struct {
	org    uuid.UUID
	action policy.Action
	event  models.Event
}

type Hub struct {
	mu    sync.Mutex
	ring  []entry
	seq   uint64
	epoch int64
	subs  map[uuid.UUID]map[chan struct{}]struct{}
	down  bool
}

func NewHub(size int) *Hub {
	return &Hub{ring: make([]entry, size), epoch: randomEpoch(), subs: map[uuid.UUID]map[chan struct{}]struct{}{}}
}

func randomEpoch() int64 {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return time.Now().UnixNano()
	}
	return int64(binary.BigEndian.Uint64(raw[:]) >> 1)
}

func (h *Hub) Append(n models.Notice) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	h.ring[h.seq%uint64(len(h.ring))] = entry{
		org: n.Organization, action: ActionOf(n.Type),
		event: models.Event{
			Seq: h.seq, Type: n.Type, Status: n.Status, ProjectID: n.ProjectID, EnvironmentID: n.EnvironmentID,
			TargetID: n.TargetID, SubjectID: n.SubjectID, At: n.At,
		},
	}
	poke(h.subs[n.Organization])
}

func (h *Hub) SetHealthy(ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.down = !ok
	for _, chans := range h.subs {
		poke(chans)
	}
}

func (h *Hub) Healthy() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.down
}

func (h *Hub) Restart() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.epoch = randomEpoch()
	for _, chans := range h.subs {
		poke(chans)
	}
}

func poke(chans map[chan struct{}]struct{}) {
	for ch := range chans {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *Hub) Subscribe(org uuid.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.subs[org] == nil {
		h.subs[org] = map[chan struct{}]struct{}{}
	}
	h.subs[org][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs[org], ch)
		if len(h.subs[org]) == 0 {
			delete(h.subs, org)
		}
	}
}

type Position struct {
	Epoch int64
	Seq   uint64
}

func (h *Hub) Now() Position {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Position{Epoch: h.epoch, Seq: h.seq}
}

func (h *Hub) oldest() uint64 {
	return h.seq - min(h.seq, uint64(len(h.ring))) + 1
}

func (h *Hub) Resumable(p Position) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return p.Epoch == h.epoch && p.Seq <= h.seq && p.Seq+1 >= h.oldest()
}

type Batch struct {
	Entries []entry
	Cursor  Position
	Lost    bool
	More    bool
}

func (b Batch) Events() []models.Event {
	out := make([]models.Event, 0, len(b.Entries))
	for _, e := range b.Entries {
		out = append(out, e.event)
	}
	return out
}

func (h *Hub) Since(org uuid.UUID, after Position, limit int) Batch {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := Batch{Cursor: Position{Epoch: h.epoch, Seq: h.seq}}
	if after.Epoch != h.epoch || after.Seq > h.seq || after.Seq+1 < h.oldest() {
		out.Lost = true
		return out
	}
	size := uint64(len(h.ring))
	for s := after.Seq + 1; s <= h.seq; s++ {
		e := h.ring[s%size]
		if e.org != org {
			continue
		}
		if len(out.Entries) == limit {
			out.More = true
			out.Cursor.Seq = s - 1
			return out
		}
		out.Entries = append(out.Entries, e)
	}
	return out
}
