package streams_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	streams "github.com/Khaymah/dhow/apps/server/internal/Core/Streams"
)

func tooMany(err error) bool {
	var ae *apperrors.AppError
	return errors.As(err, &ae) && ae.Code == apperrors.CodeTooManyStreams
}

func TestTheLimiterHoldsTheUserTheOrganizationAndTheInstance(t *testing.T) {
	limiter := streams.NewLimiter(streams.Limits{Total: 5, PerUser: 2, PerOrg: 3, RetryAfter: time.Second})
	orgA, orgB := uuid.New(), uuid.New()
	alice, bob, carol := uuid.New(), uuid.New(), uuid.New()

	a1, err := limiter.Acquire(alice, orgA)
	require.NoError(t, err)
	_, err = limiter.Acquire(alice, orgA)
	require.NoError(t, err)
	_, err = limiter.Acquire(alice, orgB)
	assert.True(t, tooMany(err), "a user has two at most, in any organization")

	_, err = limiter.Acquire(bob, orgA)
	require.NoError(t, err)
	_, err = limiter.Acquire(carol, orgA)
	assert.True(t, tooMany(err), "an organization has three at most")

	_, err = limiter.Acquire(carol, orgB)
	require.NoError(t, err)
	_, err = limiter.Acquire(uuid.New(), orgB)
	require.NoError(t, err)
	_, err = limiter.Acquire(uuid.New(), orgB)
	assert.True(t, tooMany(err), "the instance has five at most")
	assert.Equal(t, 5, limiter.Open())

	a1()
	a1()
	assert.Equal(t, 4, limiter.Open(), "giving a place back twice gives it back once")
	_, err = limiter.Acquire(alice, orgB)
	require.NoError(t, err)
}

func TestTheRefusalCarriesRetryAfter(t *testing.T) {
	limiter := streams.NewLimiter(streams.Limits{Total: 1, PerUser: 1, PerOrg: 1, RetryAfter: 7 * time.Second})
	_, err := limiter.Acquire(uuid.New(), uuid.New())
	require.NoError(t, err)
	_, err = limiter.Acquire(uuid.New(), uuid.New())
	var ae *apperrors.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, http.StatusTooManyRequests, ae.Status)
	assert.Equal(t, 7*time.Second, ae.RetryAfter)
}

func TestOpeningStreamsQuicklyIsRefusedPerUserAndRefillsWithTime(t *testing.T) {
	rate := streams.NewOpenRate(3, time.Second, 100)
	user, other := uuid.New(), uuid.New()
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	for range 3 {
		require.NoError(t, rate.Allow(user, now))
	}
	err := rate.Allow(user, now)
	var ae *apperrors.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeRateLimited, ae.Code)
	require.NoError(t, rate.Allow(other, now), "another user has its own bucket")
	require.NoError(t, rate.Allow(user, now.Add(1100*time.Millisecond)), "one place comes back each second")
	require.Error(t, rate.Allow(user, now.Add(1100*time.Millisecond)))
}

func TestTheWriterStartsWithRetryAndWritesNamedEventsWithIDs(t *testing.T) {
	rec := httptest.NewRecorder()
	w := streams.NewWriter(rec, time.Second, 3*time.Second)
	require.NoError(t, w.Start())
	require.NoError(t, w.Event("change", "1-2", map[string]string{"Type": "x"}))
	require.NoError(t, w.Event("reset", "", map[string]int{"Epoch": 4}))
	require.NoError(t, w.Comment())

	res := rec.Result()
	t.Cleanup(func() { _ = res.Body.Close() })
	assert.Equal(t, "text/event-stream; charset=utf-8", res.Header.Get("Content-Type"))
	assert.Equal(t, "no", res.Header.Get("X-Accel-Buffering"))
	assert.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	text := rec.Body.String()
	assert.True(t, strings.HasPrefix(text, "retry: 3000\n\n"))
	assert.Contains(t, text, "event: change\nid: 1-2\ndata: {\"Type\":\"x\"}\n\n")
	assert.Contains(t, text, "event: reset\ndata: {\"Epoch\":4}\n\n", "an event without an id has no id line")
	assert.True(t, strings.HasSuffix(text, ": keep-alive\n\n"))
}
