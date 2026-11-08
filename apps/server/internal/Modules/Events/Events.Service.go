package events

import (
	"log/slog"
	"sync"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	streams "github.com/Khaymah/dhow/apps/server/internal/Core/Streams"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type Subscriber interface {
	Subscribe(name string, h eventbus.Handler)
}

type Options struct {
	DB            sqlc.DBTX
	Listen        Listener
	Limits        config.Logs
	WriteWait     time.Duration
	Tick          time.Duration
	Log           *slog.Logger
	Now           func() time.Time
	RingSize      int
	PerUserStream int
	AccessTTL     time.Duration
	CanaryEvery   time.Duration
	SilenceLimit  time.Duration
}

type Service struct {
	db       sqlc.DBTX
	listen   Listener
	hub      *Hub
	rules    map[string]rule
	limiter  *streams.Limiter
	openRate *streams.OpenRate
	wait     time.Duration
	tick     time.Duration
	ttl      time.Duration
	canary   time.Duration
	silence  time.Duration
	known    map[string]bool
	log      *slog.Logger
	now      func() time.Time
	stop     chan struct{}
	stopOnce sync.Once
}

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	size := o.RingSize
	if size < 1 {
		size = config.OrgEventsRing
	}
	perUser := o.PerUserStream
	if perUser < 1 {
		perUser = config.OrgEventsPerUser
	}
	limits := o.Limits
	if limits.MaxStreams < 1 {
		limits = config.DefaultLogs()
	}
	return &Service{
		db: o.DB, listen: o.Listen, hub: NewHub(size), rules: rules(), log: o.Log, now: now,
		limiter: streams.NewLimiter(streams.Limits{
			Total: limits.MaxStreams, PerUser: perUser, PerOrg: max(2*perUser, limits.MaxStreams/config.OrgEventsOrgShare),
			RetryAfter: config.OrgEventsRetryAfter,
		}),
		openRate: streams.NewOpenRate(config.OrgEventsOpenBurst, config.OrgEventsOpenRefill, config.OrgEventsTracked),
		wait:     durationOr(o.WriteWait, config.OrgEventsWriteWait), tick: durationOr(o.Tick, config.OrgEventsTick),
		ttl: ttlOf(o.AccessTTL), canary: durationOr(o.CanaryEvery, config.OrgEventsCanary), silence: durationOr(o.SilenceLimit, config.OrgEventsSilence),
		known: knownTypes(),
		stop:  make(chan struct{}),
	}
}

func ttlOf(v time.Duration) time.Duration {
	if v == 0 {
		return config.OrgEventsAccessTTL
	}
	return max(v, 0)
}

func durationOr(v, fallback time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return fallback
}

func Subscribe(bus Subscriber, s *Service) {
	for _, name := range HandledEvents() {
		bus.Subscribe(name, s.Handle)
	}
}

func (s *Service) Hub() *Hub { return s.hub }

func (s *Service) Drain() { s.stopOnce.Do(func() { close(s.stop) }) }

func (s *Service) OpenStreams() int { return s.limiter.Open() }
