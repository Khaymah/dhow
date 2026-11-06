package ingest

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
)

var errTooSlow = errors.New("the upload is slower than the minimum rate or past its deadline")

func extendDeadline(set func(time.Time) error, until time.Time) error {
	if err := set(until); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("extend the upload deadline: %w", err)
	}
	return nil
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func uploadDeadline(size int64) time.Duration {
	d := time.Duration(size/config.IngestMinRate) * time.Second
	return min(max(d, config.IngestDeadlineFloor), config.IngestDeadlineCap)
}

type idleReader struct {
	r        io.Reader
	rc       *http.ResponseController
	idle     time.Duration
	deadline time.Time
	started  time.Time
	total    int64
	now      func() time.Time
}

func newIdleReader(r io.Reader, rc *http.ResponseController, size int64, now func() time.Time) *idleReader {
	start := now()
	return &idleReader{r: r, rc: rc, idle: config.IngestIdleTimeout, started: start, deadline: start.Add(uploadDeadline(size)), now: now}
}

func (i *idleReader) Read(p []byte) (int, error) {
	now := i.now()
	if !now.Before(i.deadline) {
		return 0, errTooSlow
	}
	if err := extendDeadline(i.rc.SetReadDeadline, earlier(now.Add(i.idle), i.deadline)); err != nil {
		return 0, err
	}
	n, err := i.r.Read(p)
	i.total += int64(n)
	if err != nil {
		return n, err
	}
	elapsed := i.now().Sub(i.started)
	if elapsed > config.IngestRateGrace && float64(i.total) < elapsed.Seconds()*config.IngestMinRate {
		return n, errTooSlow
	}
	return n, nil
}

func tooSlow(err error) bool {
	return errors.Is(err, errTooSlow) || errors.Is(err, os.ErrDeadlineExceeded)
}

type inflight struct {
	mu      sync.Mutex
	byToken map[uuid.UUID]int
	byOrg   map[uuid.UUID]int
}

func newInflight() *inflight {
	return &inflight{byToken: map[uuid.UUID]int{}, byOrg: map[uuid.UUID]int{}}
}

func (f *inflight) acquire(token, org uuid.UUID) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byToken[token] >= config.IngestPerTokenInflight || f.byOrg[org] >= config.IngestPerOrgInflight {
		return nil, apperrors.IngestBusy(config.IngestBusyRetry)
	}
	f.byToken[token]++
	f.byOrg[org]++
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.byToken[token]--
		f.byOrg[org]--
		if f.byToken[token] <= 0 {
			delete(f.byToken, token)
		}
		if f.byOrg[org] <= 0 {
			delete(f.byOrg, org)
		}
	}, nil
}
