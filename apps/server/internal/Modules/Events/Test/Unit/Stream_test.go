package unit_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	events "github.com/Khaymah/dhow/apps/server/internal/Modules/Events"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
	"github.com/Khaymah/dhow/internal/logging"
)

const wait = 3 * time.Second

type member struct {
	mu   sync.Mutex
	role policy.Role
	gone bool
}

func (m *member) set(role policy.Role, gone bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.role, m.gone = role, gone
}

func (m *member) access(_ context.Context, _ uuid.UUID) (events.Access, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gone {
		return nil, apperrors.NotFound()
	}
	return roleAccess{role: m.role}, nil
}

type roleAccess struct{ role policy.Role }

func (a roleAccess) Can(_ context.Context, action policy.Action, project uuid.UUID) (bool, error) {
	role := a.role
	if project != uuid.Nil {
		role = policy.ProjectRole(role, nil)
	}
	return policy.Allows(role, action), nil
}

type frame struct {
	Name string
	ID   string
	Data string
}

type client struct {
	frames chan frame
	closed chan struct{}
	status int
	header http.Header
	cancel context.CancelFunc
}

func (c *client) next(t *testing.T) frame {
	t.Helper()
	for {
		select {
		case f := <-c.frames:
			if f.Name == "comment" {
				continue
			}
			return f
		case <-time.After(wait):
			require.FailNow(t, "no event in time")
		}
	}
}

func (c *client) silent(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case f := <-c.frames:
		if f.Name != "comment" {
			require.FailNow(t, "an event came that must not", f.Data)
		}
	case <-time.After(d):
	}
}

func (c *client) waitClosed() bool {
	select {
	case <-c.closed:
		return true
	case <-time.After(wait):
		return false
	}
}

type rig struct {
	t       *testing.T
	service *events.Service
	srv     *httptest.Server
	org     uuid.UUID
	user    uuid.UUID
	who     *member
}

func newRig(t *testing.T, o events.Options) *rig {
	t.Helper()
	if o.Log == nil {
		o.Log = logging.New(&strings.Builder{}, slog.LevelDebug)
	}
	if o.Now == nil {
		o.Now = clock
	}
	r := &rig{t: t, service: events.NewService(o), org: uuid.New(), user: uuid.New(), who: &member{role: policy.RoleDeveloper}}
	r.srv = httptest.NewServer(http.HandlerFunc(r.handle))
	t.Cleanup(r.srv.Close)
	t.Cleanup(r.service.Drain)
	return r
}

func (r *rig) handle(w http.ResponseWriter, req *http.Request) {
	last, err := events.ParseLastEventID(req.Header.Get("Last-Event-ID"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	st, err := r.service.Open(req.Context(), events.OpenRequest{Organization: r.org, User: r.user, Last: last, Access: r.who.access})
	var app *apperrors.AppError
	if errors.As(err, &app) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Code", app.Code)
		w.WriteHeader(app.Status)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = st.Serve(w)
}

func (r *rig) open(lastID string) *client {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.srv.URL, http.NoBody)
	require.NoError(r.t, err)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := r.srv.Client().Do(req)
	require.NoError(r.t, err)
	c := &client{frames: make(chan frame, 256), closed: make(chan struct{}), status: res.StatusCode, header: res.Header, cancel: cancel}
	r.t.Cleanup(cancel)
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		close(c.closed)
		return c
	}
	go func() {
		defer close(c.closed)
		defer func() { _ = res.Body.Close() }()
		sc := bufio.NewScanner(res.Body)
		var f frame
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if f.Name != "" {
					c.frames <- f
				}
				f = frame{}
			case strings.HasPrefix(line, "event: "):
				f.Name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				f.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				f.Data = strings.TrimPrefix(line, "data: ")
			case strings.HasPrefix(line, ":"):
				f.Name = "comment"
			}
		}
	}()
	return c
}

func (r *rig) publish(typ, status string, project *uuid.UUID) {
	r.service.Hub().Append(models.Notice{Organization: r.org, Type: typ, Status: status, ProjectID: project, At: base})
}

func decode(t *testing.T, f frame) models.Event {
	t.Helper()
	var ev models.Event
	require.NoError(t, json.Unmarshal([]byte(f.Data), &ev))
	return ev
}

func TestTheStreamSendsSmallEventsWithAnIdAndNoBuffering(t *testing.T) {
	r := newRig(t, events.Options{})
	c := r.open("")
	require.Equal(t, http.StatusOK, c.status)
	assert.Equal(t, "text/event-stream; charset=utf-8", c.header.Get("Content-Type"))
	assert.Equal(t, "no", c.header.Get("X-Accel-Buffering"))
	assert.Equal(t, "no-store", c.header.Get("Cache-Control"))

	project, env := uuid.New(), uuid.New()
	r.service.Hub().Append(models.Notice{
		Organization: r.org, Type: models.TypeDeploymentFinished, Status: "failed", ProjectID: &project, EnvironmentID: &env, At: base,
	})
	f := c.next(t)
	assert.Equal(t, "change", f.Name)
	assert.Regexp(t, `^[0-9]+-1$`, f.ID)
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.Data), &raw))
	for key := range raw {
		assert.Contains(t, []string{"Seq", "Type", "Status", "ProjectId", "EnvironmentId", "TargetId", "SubjectId", "At"}, key,
			"an event carries ids and a status, never data")
	}
	assert.Equal(t, "failed", raw["Status"])
}

func TestAnEventForAnotherOrganizationNeverArrives(t *testing.T) {
	r := newRig(t, events.Options{})
	c := r.open("")
	r.service.Hub().Append(models.Notice{Organization: uuid.New(), Type: models.TypeOverviewChanged, At: base})
	c.silent(t, 200*time.Millisecond)
	r.publish(models.TypeOverviewChanged, "", nil)
	assert.Equal(t, models.TypeOverviewChanged, decode(t, c.next(t)).Type)
}

func TestAMemberWhoIsRemovedStopsReceivingOnTheSameConnection(t *testing.T) {
	r := newRig(t, events.Options{})
	c := r.open("")
	project := uuid.New()
	r.publish(models.TypeDeploymentStarted, "running", &project)
	assert.Equal(t, models.TypeDeploymentStarted, decode(t, c.next(t)).Type)

	r.who.set(policy.RoleDeveloper, true)
	r.publish(models.TypeDeploymentFinished, "succeeded", &project)

	assert.True(t, c.waitClosed(), "the stream ends when the caller is no longer a member")
	select {
	case f := <-c.frames:
		assert.NotEqual(t, "change", f.Name, "the event must not be sent to a removed member: %s", f.Data)
	default:
	}
}

func TestAMemberWhoseRoleIsLoweredStopsReceivingWhatTheNewRoleCannotDo(t *testing.T) {
	r := newRig(t, events.Options{})
	c := r.open("")
	project := uuid.New()

	r.publish(models.TypeApprovalRequested, "pending", &project)
	assert.Equal(t, models.TypeApprovalRequested, decode(t, c.next(t)).Type, "a developer may decide, so the request arrives")

	r.who.set(policy.RoleViewer, false)
	r.publish(models.TypeApprovalRequested, "pending", &project)
	r.publish(models.TypeApprovalDecided, "approved", &project)
	got := decode(t, c.next(t))
	assert.Equal(t, models.TypeApprovalDecided, got.Type, "a viewer no longer gets what only an approver gets, on the same connection")
	c.silent(t, 200*time.Millisecond)

	r.who.set(policy.RoleDeveloper, false)
	r.publish(models.TypeApprovalRequested, "pending", &project)
	assert.Equal(t, models.TypeApprovalRequested, decode(t, c.next(t)).Type, "a raised role gets it again")
}

func TestAnIdleStreamChecksTheMemberOnEveryTick(t *testing.T) {
	r := newRig(t, events.Options{Tick: 30 * time.Millisecond})
	c := r.open("")
	require.Equal(t, http.StatusOK, c.status)
	r.who.set(policy.RoleDeveloper, true)
	assert.True(t, c.waitClosed(), "no event is needed to end the stream of a removed member")
}

func TestAStreamResumesAfterLastEventIDAndSendsNothingTwice(t *testing.T) {
	r := newRig(t, events.Options{})
	first := r.open("")
	for range 3 {
		r.publish(models.TypeOverviewChanged, "", nil)
	}
	var last string
	for range 3 {
		last = first.next(t).ID
	}
	first.cancel()
	r.publish(models.TypeDeploymentStarted, "running", nil)
	r.publish(models.TypeDeploymentFinished, "succeeded", nil)

	second := r.open(last)
	assert.Equal(t, models.TypeDeploymentStarted, decode(t, second.next(t)).Type)
	assert.Equal(t, models.TypeDeploymentFinished, decode(t, second.next(t)).Type)
	second.silent(t, 150*time.Millisecond)
}

func TestAnIdTheServerCannotResumeGetsOneResetAndNoReplay(t *testing.T) {
	r := newRig(t, events.Options{RingSize: 4})
	for range 10 {
		r.publish(models.TypeOverviewChanged, "", nil)
	}
	for _, id := range []string{"1-1", "7-3", "99999999999-2"} {
		c := r.open(id)
		f := c.next(t)
		assert.Equal(t, "reset", f.Name, id)
		c.silent(t, 100*time.Millisecond)
		c.cancel()
	}
}

func TestTheLimitsAnswer429WithRetryAfter(t *testing.T) {
	r := newRig(t, events.Options{PerUserStream: 2})
	a, b := r.open(""), r.open("")
	require.Equal(t, http.StatusOK, a.status)
	require.Equal(t, http.StatusOK, b.status)
	c := r.open("")
	assert.Equal(t, http.StatusTooManyRequests, c.status)
	assert.Equal(t, "TOO_MANY_STREAMS", c.header.Get("X-Code"))
	assert.Equal(t, 2, r.service.OpenStreams())

	a.cancel()
	require.True(t, a.waitClosed())
	assert.Eventually(t, func() bool { return r.service.OpenStreams() == 1 }, wait, 10*time.Millisecond, "a closed stream gives its place back")
	assert.Equal(t, http.StatusOK, r.open("").status)
}

func TestAStreamIsRefusedWhileTheListenerIsDown(t *testing.T) {
	r := newRig(t, events.Options{})
	r.service.Hub().SetHealthy(false)
	assert.Equal(t, http.StatusServiceUnavailable, r.open("").status)
	r.service.Hub().SetHealthy(true)
	assert.Equal(t, http.StatusOK, r.open("").status)
}

func TestOpenStreamsEndWhenTheListenerGoesDown(t *testing.T) {
	r := newRig(t, events.Options{})
	c := r.open("")
	require.Equal(t, http.StatusOK, c.status)
	r.service.Hub().SetHealthy(false)
	assert.True(t, c.waitClosed(), "the client falls back to polling instead of waiting on a stream that cannot get events")
}

func TestDrainEndsEveryStream(t *testing.T) {
	r := newRig(t, events.Options{})
	c := r.open("")
	require.Equal(t, http.StatusOK, c.status)
	r.service.Drain()
	assert.True(t, c.waitClosed())
}
