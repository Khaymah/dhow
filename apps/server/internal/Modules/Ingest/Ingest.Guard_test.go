package ingest

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	"github.com/Khaymah/dhow/internal/archive"
)

type steppedClock struct {
	t    time.Time
	step time.Duration
}

func (c *steppedClock) now() time.Time {
	c.t = c.t.Add(c.step)
	return c.t
}

func readerFor(r io.Reader, size int64, clock *steppedClock) *idleReader {
	rc := http.NewResponseController(httptest.NewRecorder())
	return newIdleReader(r, rc, size, clock.now)
}

func TestASlowDripBodyIsCutOff(t *testing.T) {
	clock := &steppedClock{t: time.Unix(1_000_000, 0), step: 5 * time.Second}
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 1<<20))
	r := readerFor(body, 1<<20, clock)
	buf := make([]byte, 16)
	var err error
	for range 100 {
		if _, err = r.Read(buf); err != nil {
			break
		}
	}
	require.ErrorIs(t, err, errTooSlow, "16 bytes every 10 seconds is far below the minimum rate")
	assert.True(t, tooSlow(err))
	assert.Equal(t, http.StatusRequestTimeout, apperrors.From(ClassifyUpload(err, nil)).Status)
}

func TestAFastBodyIsNotCutOff(t *testing.T) {
	clock := &steppedClock{t: time.Unix(1_000_000, 0), step: time.Millisecond}
	data := bytes.Repeat([]byte("x"), 8<<20)
	r := readerFor(bytes.NewReader(data), int64(len(data)), clock)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Len(t, got, len(data))
}

func TestTheAbsoluteDeadlineEndsAnUpload(t *testing.T) {
	clock := &steppedClock{t: time.Unix(1_000_000, 0), step: time.Minute}
	r := readerFor(bytes.NewReader(bytes.Repeat([]byte("x"), 100<<20)), 1, clock)
	buf := make([]byte, 64<<20)
	var err error
	for range 10 {
		if _, err = r.Read(buf); err != nil {
			break
		}
	}
	require.ErrorIs(t, err, errTooSlow, "a tiny declared size gets the two minute floor, not more")
}

func TestTheDeadlineFollowsTheSizeWithinAFloorAndACap(t *testing.T) {
	assert.Equal(t, config.IngestDeadlineFloor, uploadDeadline(1))
	assert.Equal(t, config.IngestDeadlineCap, uploadDeadline(1<<40))
	mid := uploadDeadline(200 << 20)
	assert.Greater(t, mid, config.IngestDeadlineFloor)
	assert.Less(t, mid, config.IngestDeadlineCap)
}

func TestOneTokenUploadsOnceAndOneOrganizationTwiceAtATime(t *testing.T) {
	f := newInflight()
	orgA, orgB := uuid.New(), uuid.New()
	tokenA1, tokenA2, tokenA3, tokenB := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	releaseA1, err := f.acquire(tokenA1, orgA, "198.51.100.1")
	require.NoError(t, err)
	_, err = f.acquire(tokenA1, orgA, "198.51.100.2")
	var busy *apperrors.AppError
	require.ErrorAs(t, err, &busy, "a second upload of the same token waits")
	assert.Equal(t, http.StatusServiceUnavailable, busy.Status)

	releaseA2, err := f.acquire(tokenA2, orgA, "198.51.100.3")
	require.NoError(t, err)
	_, err = f.acquire(tokenA3, orgA, "198.51.100.4")
	require.Error(t, err, "a third upload of the same organization waits")

	releaseB, err := f.acquire(tokenB, orgB, "198.51.100.5")
	require.NoError(t, err, "another organization still uploads")

	releaseA1()
	again, err := f.acquire(tokenA3, orgA, "198.51.100.4")
	require.NoError(t, err, "a finished upload frees its place")
	again()
	releaseA2()
	releaseB()
	assert.Empty(t, f.byToken)
	assert.Empty(t, f.byOrg)
	assert.Empty(t, f.byNet)
}

func TestOneNetworkNeverHoldsMoreThanItsShareAcrossOrganizations(t *testing.T) {
	f := newInflight()
	const network = "203.0.113.9"
	releases := make([]func(), 0, config.IngestPerNetworkInflight)
	for range config.IngestPerNetworkInflight {
		release, err := f.acquire(uuid.New(), uuid.New(), network)
		require.NoError(t, err)
		releases = append(releases, release)
	}
	_, err := f.acquire(uuid.New(), uuid.New(), network)
	var busy *apperrors.AppError
	require.ErrorAs(t, err, &busy, "a third upload from one address waits, whichever organization it is for")
	assert.Equal(t, http.StatusServiceUnavailable, busy.Status)

	other, err := f.acquire(uuid.New(), uuid.New(), "203.0.113.10")
	require.NoError(t, err, "another address still uploads")
	other()
	releases[0]()
	again, err := f.acquire(uuid.New(), uuid.New(), network)
	require.NoError(t, err, "a finished upload frees its place")
	again()
	for _, release := range releases[1:] {
		release()
	}
	assert.Empty(t, f.byNet)
}

func TestNoSingleOrganizationOrPairOfThemCanTakeEveryGlobalSlot(t *testing.T) {
	assert.LessOrEqual(t, config.IngestPerOrgInflight*config.IngestOrgShareDivisor, config.IngestConcurrent)
	assert.Less(t, 2*config.IngestPerOrgInflight, config.IngestConcurrent, "two organizations always leave a slot for a third")
	assert.Less(t, config.IngestPerNetworkInflight, config.IngestConcurrent)
}

func TestTheLimitIsCheckedBeforeTheArchiveReason(t *testing.T) {
	broken := errors.Join(archive.ErrUnsafe, errors.New("tar"))
	cases := map[string]struct {
		stage, arch error
		code        string
	}{
		"too large first":  {blob.ErrTooLarge, broken, apperrors.CodeArtifactTooLarge},
		"no space first":   {blob.ErrNoSpace, broken, apperrors.CodeStorageFull},
		"a wrong digest":   {blob.ErrDigestMismatch, nil, apperrors.CodeDigestMismatch},
		"a bad archive":    {errors.New("pipe closed"), broken, apperrors.CodeArchiveUnsafe},
		"an archive alone": {nil, broken, apperrors.CodeArchiveUnsafe},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.code, apperrors.From(ClassifyUpload(tt.stage, tt.arch)).Code)
		})
	}
	assert.NoError(t, ClassifyUpload(nil, nil))
}

func TestTheServiceFailsClosedWithoutALimiter(t *testing.T) {
	_, err := NewService(Options{Store: stubStore{}})
	require.ErrorIs(t, err, ErrNoLimiter)
}

type stubStore struct{ blob.Store }

func TestWidePatternsCouldOverlapAndLiteralOnesOnlyWhenEqual(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"main", "main", true},
		{"main", "release", false},
		{"**", "main", true},
		{"release/*", "release/1.x", true},
		{"release/*", "main", false},
		{"release/**", "release/1.*", true},
		{"rel*", "release/**", true},
		{"feature/*", "release/*", false},
		{"v*", "v1.*", true},
		{"v*", "x*", false},
		{"**", "**", true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, CouldOverlap(tt.a, tt.b), "%s vs %s", tt.a, tt.b)
		assert.Equal(t, tt.want, CouldOverlap(tt.b, tt.a), "%s vs %s", tt.b, tt.a)
	}
}
