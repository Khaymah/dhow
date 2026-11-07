package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

const gateTimeout = 30 * time.Second

type slot struct {
	token apptest.IngestToken
	body  []byte
	addr  string
}

var slotCounter atomic.Int64

func (w world) slotIn(t *testing.T, org uuid.UUID, n int) slot {
	t.Helper()
	creator := w.ownerID
	if org != w.org {
		creator = w.env.SeedUser(org, fmt.Sprintf("slot-%s@example.com", uuid.NewString()[:8]), "a long password for a slot", "owner")
	}
	project := w.env.SeedProject(org, creator)
	return w.slotFrom(t, project, creator, n, fmt.Sprintf("198.51.100.%d:4444", 30+slotCounter.Add(1)))
}

func (w world) slotFrom(t *testing.T, project, creator uuid.UUID, n int, addr string) slot {
	t.Helper()
	return slot{
		token: w.env.SeedIngestToken(project, creator, apptest.IngestSeed{}),
		body:  apptest.TarGz(map[string]string{"n.txt": strings.Repeat("a", n+1)}),
		addr:  addr,
	}
}

func (w world) putSlot(s slot, after func()) apptest.Response {
	return w.env.IngestPut(apptest.IngestUpload{
		Token: s.token.Value, Body: s.body, Name: "web", Ref: "main", Sha: shaNext, NoTrigger: true, AfterBody: after, RemoteAddr: s.addr,
	})
}

type held struct {
	release func()
	wait    func() []apptest.Response
}

func (w world) hold(t *testing.T, flying []slot) held {
	t.Helper()
	entered := make(chan struct{}, len(flying))
	release := make(chan struct{})
	results := make(chan apptest.Response, len(flying))
	var wg sync.WaitGroup
	for _, s := range flying {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- w.putSlot(s, func() {
				entered <- struct{}{}
				<-release
			})
		}()
	}
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	for range flying {
		select {
		case <-entered:
		case <-time.After(gateTimeout):
			free()
			require.Fail(t, "the uploads did not all reach the point where they hold a slot")
		}
	}
	return held{release: free, wait: func() []apptest.Response {
		free()
		wg.Wait()
		close(results)
		var out []apptest.Response
		for res := range results {
			out = append(out, res)
		}
		return out
	}}
}

func TestTheGlobalLimitOfConcurrentUploadsRefusesTheNext(t *testing.T) {
	w := newWorld(t)
	var flying []slot
	for i := range config.IngestConcurrent / config.IngestPerOrgInflight {
		org := w.env.SeedOrganization(fmt.Sprintf("Org %d", i), fmt.Sprintf("org-%d", i))
		for n := range config.IngestPerOrgInflight {
			flying = append(flying, w.slotIn(t, org, i*10+n))
		}
	}
	require.Len(t, flying, config.IngestConcurrent)
	extra := w.env.SeedOrganization("Extra", "extra-org")
	next := w.slotIn(t, extra, 99)

	h := w.hold(t, flying)
	refused := w.putSlot(next, nil)
	results := h.wait()

	assert.Equal(t, http.StatusServiceUnavailable, refused.Status, string(refused.Raw))
	require.NotNil(t, refused.Error, string(refused.Raw))
	assert.Equal(t, "SERVICE_UNAVAILABLE", refused.Error.Code)
	assert.Equal(t, fmt.Sprint(int(config.IngestBusyRetry.Seconds())), refused.Header.Get("Retry-After"))
	for _, res := range results {
		assert.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	}

	again := w.putSlot(next, nil)
	assert.Equal(t, http.StatusCreated, again.Status, "the slot is free again once the others finish: %s", again.Raw)
}

func TestTwoOrganizationsCannotFillTheGlobalSlotsAndAThirdStillUploads(t *testing.T) {
	w := newWorld(t)
	orgB := w.env.SeedOrganization("Second", "second-org")
	orgC := w.env.SeedOrganization("Third", "third-org")
	var flying []slot
	for _, org := range []uuid.UUID{w.org, orgB} {
		for n := range config.IngestPerOrgInflight {
			flying = append(flying, w.slotIn(t, org, n))
		}
	}
	third := w.slotIn(t, orgC, 7)
	extra := w.slotIn(t, w.org, 8)

	h := w.hold(t, flying)
	sameOrgAgain := w.putSlot(extra, nil)
	free := w.putSlot(third, nil)
	h.wait()

	assert.Equal(t, http.StatusServiceUnavailable, sameOrgAgain.Status, "one organization never takes more than its share: %s", sameOrgAgain.Raw)
	assert.Equal(t, http.StatusCreated, free.Status, "two busy organizations leave room for a third: %s", free.Raw)
}

func TestOneAddressHoldsOnlyItsShareOfUploadsWhateverTheOrganization(t *testing.T) {
	w := newWorld(t)
	addr := "203.0.113.77:5555"
	var flying []slot
	for i := range config.IngestPerNetworkInflight {
		org := w.env.SeedOrganization(fmt.Sprintf("Net %d", i), fmt.Sprintf("net-%d", i))
		s := w.slotIn(t, org, i)
		s.addr = addr
		flying = append(flying, s)
	}
	orgX := w.env.SeedOrganization("Another", "another-org")
	blocked := w.slotIn(t, orgX, 40)
	blocked.addr = addr
	elsewhere := w.slotIn(t, orgX, 41)

	h := w.hold(t, flying)
	refused := w.putSlot(blocked, nil)
	ok := w.putSlot(elsewhere, nil)
	h.wait()

	assert.Equal(t, http.StatusServiceUnavailable, refused.Status, string(refused.Raw))
	require.NotNil(t, refused.Error, string(refused.Raw))
	assert.Equal(t, "SERVICE_UNAVAILABLE", refused.Error.Code)
	assert.Equal(t, http.StatusCreated, ok.Status, "another address is not held back: %s", ok.Raw)
}

type tripwire struct{ reads atomic.Int64 }

func (b *tripwire) Read([]byte) (int, error) {
	b.reads.Add(1)
	return 0, fmt.Errorf("the handler read a body that it should have refused by its length")
}

func (b *tripwire) Close() error { return nil }

func TestALengthOverTheLimitIsRefusedBeforeTheBodyIsRead(t *testing.T) {
	w := newWorld(t)
	body := &tripwire{}
	digest := apptest.DigestHex([]byte("whatever"))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/ingest/v1/artifacts/"+digest, body)
	r.ContentLength = w.env.Config.Artifacts.IngestMaxBytes + 1
	r.RemoteAddr = "198.51.100.20:4444"
	r.Header.Set("Authorization", "Bearer "+w.token.Value)
	r.Header.Set("Content-Type", "application/gzip")
	r.Header.Set(config.IngestHeaderName, "web")
	r.Header.Set(config.IngestHeaderRef, "main")
	r.Header.Set(config.IngestHeaderSha, shaNext)
	rec := httptest.NewRecorder()
	w.env.Handler.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ARTIFACT_TOO_LARGE")
	assert.Zero(t, body.reads.Load(), "not one byte of the body was read")
	assert.Zero(t, w.env.IngestedCount(w.project))
}
