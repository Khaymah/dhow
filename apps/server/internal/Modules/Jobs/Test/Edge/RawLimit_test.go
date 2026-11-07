package edge_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func TestRawDownloadsShareTheStreamBudget(t *testing.T) {
	w := setup(t, apptest.NewWithOptions(t, apptest.Options{Profile: func(c *config.Config) {
		c.Logs = config.Logs{MaxStreams: config.LogStreamsPerUser + 2}
	}}))
	id, r, g := runningJob(t, w)
	r.MustAppend(g.Ref, apptest.Chunk(1, "one line\n"))
	srv := w.env.Serve()
	raw := "/organizations/" + w.org.String() + "/jobs/" + id.String() + "/logs/raw"

	done := w.owner.Get(raw)
	require.Equal(t, http.StatusOK, done.Status, string(done.Raw))
	assert.Equal(t, "one line\n", string(done.Raw))
	assert.Zero(t, w.env.Wired.Jobs.OpenLogStreams(), "a finished download frees its slot")

	path := apptest.LogStreamPath(w.org, id)
	for range config.LogStreamsPerUser {
		s := w.owner.OpenStream(t, srv, path, nil)
		require.Equal(t, http.StatusOK, s.Status, s.Body)
		skipEvent(t, s, "status")
	}
	refused := w.owner.Get(raw)
	assert.Equal(t, http.StatusTooManyRequests, refused.Status)
	require.NotNil(t, refused.Error)
	assert.Equal(t, "TOO_MANY_STREAMS", refused.Error.Code)
}
