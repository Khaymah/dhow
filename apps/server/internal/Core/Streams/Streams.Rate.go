package streams

import (
	"sync"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
)

type bucket struct {
	tokens float64
	at     time.Time
}

type OpenRate struct {
	mu      sync.Mutex
	burst   float64
	refill  time.Duration
	tracked int
	buckets map[uuid.UUID]bucket
}

func NewOpenRate(burst int, refill time.Duration, tracked int) *OpenRate {
	return &OpenRate{burst: float64(burst), refill: refill, tracked: tracked, buckets: map[uuid.UUID]bucket{}}
}

func (r *OpenRate) Allow(key uuid.UUID, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[key]
	if !ok {
		b = bucket{tokens: r.burst, at: now}
	}
	b.tokens = min(r.burst, b.tokens+now.Sub(b.at).Seconds()/r.refill.Seconds())
	b.at = now
	if b.tokens < 1 {
		r.buckets[key] = b
		return apperrors.RateLimited(r.refill)
	}
	b.tokens--
	r.buckets[key] = b
	if len(r.buckets) > r.tracked {
		full := time.Duration(r.burst) * r.refill
		for id, other := range r.buckets {
			if now.Sub(other.at) >= full {
				delete(r.buckets, id)
			}
		}
	}
	return nil
}
