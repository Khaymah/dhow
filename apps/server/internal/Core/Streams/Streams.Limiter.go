package streams

import (
	"sync"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
)

type Limits struct {
	Total      int
	PerUser    int
	PerOrg     int
	RetryAfter time.Duration
}

type Limiter struct {
	mu     sync.Mutex
	limits Limits
	total  int
	users  map[uuid.UUID]int
	orgs   map[uuid.UUID]int
}

func NewLimiter(limits Limits) *Limiter {
	return &Limiter{limits: limits, users: map[uuid.UUID]int{}, orgs: map[uuid.UUID]int{}}
}

func (l *Limiter) Acquire(user, org uuid.UUID) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= l.limits.Total || l.users[user] >= l.limits.PerUser || l.orgs[org] >= l.limits.PerOrg {
		return nil, apperrors.TooManyStreams(l.limits.RetryAfter)
	}
	l.total++
	l.users[user]++
	l.orgs[org]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.total--
			drop(l.users, user)
			drop(l.orgs, org)
		})
	}, nil
}

func drop(counts map[uuid.UUID]int, key uuid.UUID) {
	if counts[key]--; counts[key] <= 0 {
		delete(counts, key)
	}
}

func (l *Limiter) Open() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}
