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
}

func (w world) slotIn(t *testing.T, org uuid.UUID, n int) slot {
	t.Helper()
	project := w.env.SeedProject(org, w.ownerID)
	return slot{
		token: w.env.SeedIngestToken(project, w.ownerID, apptest.IngestSeed{}),
		body:  apptest.TarGz(map[string]string{"n.txt": strings.Repeat("a", n+1)}),
	}
}

func (w world) putSlot(s slot, after func()) apptest.Response {
	return w.env.IngestPut(apptest.IngestUpload{
		Token: s.token.Value, Body: s.body, Name: "web", Ref: "main", Sha: shaNext, NoTrigger: true, AfterBody: after,
	})
}

func TestTheGlobalLimitOfConcurrentUploadsRefusesTheFifth(t *testing.T) {
	w := newWorld(t)
	orgB := w.env.SeedOrganization("Second", "second-org")
	orgC := w.env.SeedOrganization("Third", "third-org")
	flying := []slot{w.slotIn(t, w.org, 0), w.slotIn(t, w.org, 1), w.slotIn(t, orgB, 2), w.slotIn(t, orgB, 3)}
	require.Len(t, flying, config.IngestConcurrent)
	fifth := w.slotIn(t, orgC, 4)

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
	for range flying {
		select {
		case <-entered:
		case <-time.After(gateTimeout):
			close(release)
			require.Fail(t, "the uploads did not all reach the point where they hold a slot")
		}
	}

	refused := w.putSlot(fifth, nil)
	close(release)
	wg.Wait()
	close(results)

	assert.Equal(t, http.StatusServiceUnavailable, refused.Status, string(refused.Raw))
	require.NotNil(t, refused.Error, string(refused.Raw))
	assert.Equal(t, "SERVICE_UNAVAILABLE", refused.Error.Code)
	assert.Equal(t, fmt.Sprint(int(config.IngestBusyRetry.Seconds())), refused.Header.Get("Retry-After"))
	for res := range results {
		assert.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	}

	again := w.putSlot(fifth, nil)
	assert.Equal(t, http.StatusCreated, again.Status, "the slot is free again once the others finish: %s", again.Raw)
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
