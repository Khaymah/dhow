package e2e_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestTheRawLogIsTheWholeTextAsAnAttachment(t *testing.T) {
	w := newWorld(t)
	id, r, g := w.running(t)
	var want strings.Builder
	for from := 1; from <= 200; from += 50 {
		batch := make([]*runnerv1.LogChunk, 0, 50)
		for seq := from; seq < from+50; seq++ {
			text := "line " + strconv.Itoa(seq) + " مرحبا\n"
			want.WriteString(text)
			batch = append(batch, apptest.Chunk(int64(seq), text))
		}
		r.MustAppend(g.Ref, batch...)
	}
	r.MustAppend(g.Ref, &runnerv1.LogChunk{Seq: 201, Stream: runnerv1.LogStream_LOG_STREAM_SYSTEM, Data: []byte{'o', 'k', 0xff, '\n'}})
	want.WriteString("ok�\n")

	res := w.owner.Get(w.path("/jobs/" + id.String() + "/logs/raw"))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, want.String(), string(res.Raw))
	assert.Equal(t, `attachment; filename="job-`+id.String()+`-attempt-1.log"`, res.Header.Get("Content-Disposition"))
	assert.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "text/plain; charset=utf-8", res.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", res.Header.Get("Cache-Control"))
}

func TestTheRawLogOfAJobWithoutChunksIsEmpty(t *testing.T) {
	w := newWorld(t)
	id, _, _ := w.running(t)
	res := w.owner.Get(w.path("/jobs/" + id.String() + "/logs/raw"))
	require.Equal(t, http.StatusOK, res.Status)
	assert.Empty(t, res.Raw)
}

func TestTheRawLogHasTheSameAuthorizationAsReadingTheLog(t *testing.T) {
	w := newWorld(t)
	id, r, g := w.running(t)
	r.MustAppend(g.Ref, apptest.Chunk(1, "secret-free text\n"))
	raw := "/jobs/" + id.String() + "/logs/raw"

	other := w.env.SeedOrganization("Other", "other-raw")
	outsider := w.env.SeedSession(w.env.SeedUser(other, "outsider-raw@example.com", "a long password for outsider", "owner"))
	assert.Equal(t, http.StatusNotFound, outsider.Get("/organizations/"+other.String()+raw).Status)
	assert.Equal(t, http.StatusNotFound, outsider.Get(w.path(raw)).Status)

	viewer := w.env.SeedSession(w.env.SeedUser(w.org, "viewer-raw@example.com", "a long password for viewer", "viewer"))
	assert.Equal(t, http.StatusForbidden, viewer.Get(w.path(raw)).Status)

	anonymous := &apptest.Client{Env: w.env}
	assert.Equal(t, http.StatusUnauthorized, anonymous.Get(w.path(raw)).Status)

	missing := w.owner.Get(w.path(raw + "?Attempt=7"))
	assert.Equal(t, http.StatusNotFound, missing.Status)
	require.NotNil(t, missing.Error)
	assert.Equal(t, "ATTEMPT_NOT_FOUND", missing.Error.Code)

	bad := w.owner.Get(w.path(raw + "?Attempt=0"))
	assert.Equal(t, http.StatusUnprocessableEntity, bad.Status)
}
