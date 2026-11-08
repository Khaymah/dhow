package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	events "github.com/Khaymah/dhow/apps/server/internal/Modules/Events"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

const wait = 5 * time.Second

type world struct {
	env     *apptest.Env
	org     uuid.UUID
	owner   *apptest.Client
	project uuid.UUID
}

func newWorld(t *testing.T) world {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	require.Eventually(t, env.Wired.Options.Events.Hub().Healthy, wait, 10*time.Millisecond, "the listener starts with the app")
	return world{env: env, org: org, owner: owner, project: env.SeedProject(org, uuid.MustParse(owner.UserID))}
}

func (w world) path() string {
	return config.APIPrefix + "/organizations/" + w.org.String() + "/events"
}

func next(t *testing.T, s *apptest.Stream) apptest.Event {
	t.Helper()
	for {
		ev, err := s.Next(wait)
		require.NoError(t, err)
		if ev.Name != "comment" {
			return ev
		}
	}
}

func notify(t *testing.T, w world, n models.Notice) {
	t.Helper()
	raw, err := json.Marshal(n)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(context.Background(), `SELECT pg_notify($1, $2)`, config.OrgEventsChannel, string(raw))
	require.NoError(t, err)
}

func TestAnEventPostedToTheDatabaseReachesAnOpenStream(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	s := w.owner.OpenStream(t, srv, w.path(), nil)
	require.Equal(t, http.StatusOK, s.Status, s.Body)
	assert.Equal(t, "no", s.Header.Get("X-Accel-Buffering"))

	env := uuid.New()
	notify(t, w, models.Notice{
		Organization: w.org, Type: models.TypeDeploymentFinished, Status: "succeeded", ProjectID: &w.project, EnvironmentID: &env, At: time.Now().UTC(),
	})
	ev := next(t, s)
	assert.Equal(t, "change", ev.Name)
	var got models.Event
	require.NoError(t, json.Unmarshal([]byte(ev.Data), &got))
	assert.Equal(t, models.TypeDeploymentFinished, got.Type)
	assert.Equal(t, "succeeded", got.Status)
	assert.Equal(t, w.project, *got.ProjectID)
}

func TestAReleaseEventCarriesTheProjectAndEnvironmentOfItsTarget(t *testing.T) {
	w := newWorld(t)
	target, _ := w.env.SeedDeployTarget(w.project, "production", "web")
	var environment uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "EnvironmentId" FROM "DeployTargets" WHERE "Id" = $1`, target).Scan(&environment))
	srv := w.env.Serve()
	s := w.owner.OpenStream(t, srv, w.path(), nil)
	require.Equal(t, http.StatusOK, s.Status, s.Body)

	require.NoError(t, w.env.Wired.Options.Events.Handle(context.Background(), eventbus.Event{
		Name: eventbus.DeploymentReleaseRollbackFailed,
		Payload: eventbus.Audited{
			OrganizationIDs: []uuid.UUID{w.org}, TargetType: "deployment_release", TargetID: uuid.NewString(),
			Data: map[string]string{"TargetId": target.String(), "Reason": "the secret value must never travel"},
		},
	}))
	ev := next(t, s)
	var got models.Event
	require.NoError(t, json.Unmarshal([]byte(ev.Data), &got))
	assert.Equal(t, models.TypeReleaseRollbackFail, got.Type)
	assert.Equal(t, "rollback_failed", got.Status)
	assert.Equal(t, w.project, *got.ProjectID)
	assert.Equal(t, environment, *got.EnvironmentID)
	assert.Equal(t, target, *got.TargetID)
	assert.NotContains(t, ev.Data, "secret value")
}

func TestARemovedMemberStopsReceivingOnTheSameConnection(t *testing.T) {
	w := newWorld(t)
	dev, devID := w.env.SignedIn(w.org, "dev@example.com", "developer")
	srv := w.env.Serve()
	s := dev.OpenStream(t, srv, w.path(), nil)
	require.Equal(t, http.StatusOK, s.Status, s.Body)

	notify(t, w, models.Notice{Organization: w.org, Type: models.TypeOverviewChanged, At: time.Now().UTC()})
	assert.Equal(t, "change", next(t, s).Name)

	time.Sleep(config.OrgEventsAccessTTL + 200*time.Millisecond)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Memberships" SET "IsDeleted" = true WHERE "UserId" = $1 AND "OrganizationId" = $2`, devID, w.org)
	require.NoError(t, err)
	notify(t, w, models.Notice{Organization: w.org, Type: models.TypeDeploymentStarted, Status: "running", ProjectID: &w.project, At: time.Now().UTC()})

	assert.True(t, s.WaitClosed(wait), "the stream ends when the member is removed")
	select {
	case ev, ok := <-chanOf(s):
		if ok {
			assert.NotEqual(t, "change", ev.Name, "no event reaches a removed member: %s", ev.Data)
		}
	default:
	}
}

func chanOf(s *apptest.Stream) <-chan apptest.Event {
	out := make(chan apptest.Event, 1)
	if ev, err := s.Next(10 * time.Millisecond); err == nil {
		out <- ev
	}
	close(out)
	return out
}

func TestAMemberWhoseRoleIsLoweredNoLongerGetsApprovalRequests(t *testing.T) {
	w := newWorld(t)
	dev, devID := w.env.SignedIn(w.org, "dev@example.com", "developer")
	srv := w.env.Serve()
	s := dev.OpenStream(t, srv, w.path(), nil)
	require.Equal(t, http.StatusOK, s.Status, s.Body)

	notify(t, w, models.Notice{Organization: w.org, Type: models.TypeApprovalRequested, Status: "pending", ProjectID: &w.project, At: time.Now().UTC()})
	var got models.Event
	require.NoError(t, json.Unmarshal([]byte(next(t, s).Data), &got))
	assert.Equal(t, models.TypeApprovalRequested, got.Type, "a developer may decide, so the request arrives")

	time.Sleep(config.OrgEventsAccessTTL + 200*time.Millisecond)
	_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Memberships" SET "Role" = 'viewer' WHERE "UserId" = $1 AND "OrganizationId" = $2`, devID, w.org)
	require.NoError(t, err)
	notify(t, w, models.Notice{Organization: w.org, Type: models.TypeApprovalRequested, Status: "pending", ProjectID: &w.project, At: time.Now().UTC()})
	notify(t, w, models.Notice{Organization: w.org, Type: models.TypeApprovalDecided, Status: "approved", ProjectID: &w.project, At: time.Now().UTC()})

	require.NoError(t, json.Unmarshal([]byte(next(t, s).Data), &got))
	assert.Equal(t, models.TypeApprovalDecided, got.Type, "the lowered role skips what only an approver may see")
}

func TestTheSixthStreamOfOneUserAnswers429(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	for range config.OrgEventsPerUser {
		s := w.owner.OpenStream(t, srv, w.path(), nil)
		require.Equal(t, http.StatusOK, s.Status, s.Body)
	}
	s := w.owner.OpenStream(t, srv, w.path(), nil)
	assert.Equal(t, http.StatusTooManyRequests, s.Status)
	assert.Contains(t, s.Body, "TOO_MANY_STREAMS")
	assert.NotEmpty(t, s.Header.Get("Retry-After"))
}

func TestAnAnonymousCallerAndAForeignOrganizationAreRefused(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	anon := (&apptest.Client{Env: w.env}).OpenStream(t, srv, w.path(), nil)
	assert.Equal(t, http.StatusUnauthorized, anon.Status)

	other := w.env.SeedOrganization("Other Org", "other-org")
	foreign := w.owner.OpenStream(t, srv, config.APIPrefix+"/organizations/"+other.String()+"/events", nil)
	assert.Equal(t, http.StatusNotFound, foreign.Status)
}

func TestAMalformedLastEventIDIsRefused(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	s := w.owner.OpenStream(t, srv, w.path(), map[string]string{"Last-Event-ID": "not-an-id"})
	assert.Equal(t, http.StatusUnprocessableEntity, s.Status)
}

func TestAReasonTheRunnerSuppliedNeverReachesTheStream(t *testing.T) {
	w := newWorld(t)
	target, _ := w.env.SeedDeployTarget(w.project, "production", "web")
	srv := w.env.Serve()
	s := w.owner.OpenStream(t, srv, w.path(), nil)
	require.Equal(t, http.StatusOK, s.Status, s.Body)

	for _, reason := range []string{"hunter2-the-runner-wrote-this", "ok"} {
		require.NoError(t, w.env.Wired.Options.Events.Handle(context.Background(), eventbus.Event{
			Name: eventbus.DeploymentTargetChecked,
			Payload: eventbus.Audited{
				OrganizationIDs: []uuid.UUID{w.org}, TargetType: "deployment_check", TargetID: target.String(),
				Data: map[string]string{"Reason": reason},
			},
		}))
	}
	first, second := next(t, s), next(t, s)
	assert.NotContains(t, first.Data, "hunter2")
	assert.Contains(t, first.Data, `"Status":"unknown"`, "a value outside the fixed list becomes unknown")
	assert.Contains(t, second.Data, `"Status":"ok"`)
}

func TestAForgedNotificationOfAnUnknownKindIsDropped(t *testing.T) {
	w := newWorld(t)
	srv := w.env.Serve()
	s := w.owner.OpenStream(t, srv, w.path(), nil)
	require.Equal(t, http.StatusOK, s.Status, s.Body)

	notify(t, w, models.Notice{Organization: w.org, Type: "evil.type", Status: "x", At: time.Now().UTC()})
	notify(t, w, models.Notice{Organization: uuid.Nil, Type: models.TypeOverviewChanged, At: time.Now().UTC()})
	notify(t, w, models.Notice{Organization: w.org, Type: models.TypeOverviewChanged, At: time.Now().UTC()})
	var got models.Event
	require.NoError(t, json.Unmarshal([]byte(next(t, s).Data), &got))
	assert.Equal(t, models.TypeOverviewChanged, got.Type)
	assert.Equal(t, uint64(1), got.Seq, "the two forged ones never took a place in the ring")
}

func TestASilentListenerIsReplacedAndTheHubGoesUnhealthyMeanwhile(t *testing.T) {
	env := apptest.New(t)
	service := events.NewService(events.Options{
		Listen: events.Listener(func(ctx context.Context) (*pgx.Conn, error) {
			return pgx.ConnectConfig(ctx, env.Pool.Config().ConnConfig.Copy())
		}),
		SilenceLimit: 300 * time.Millisecond,
		Log:          slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	require.Eventually(t, service.Hub().Healthy, wait, 20*time.Millisecond)
	first := service.Hub().Now().Epoch
	require.Eventually(t, func() bool { return service.Hub().Now().Epoch != first }, 10*time.Second, 50*time.Millisecond,
		"with no canary and no event the connection is judged dead and a new one replaces it")
}

func TestTheCanaryKeepsAHealthyListenerAliveAndIsNeverAnEvent(t *testing.T) {
	env := apptest.New(t)
	service := events.NewService(events.Options{
		DB: env.Pool,
		Listen: events.Listener(func(ctx context.Context) (*pgx.Conn, error) {
			return pgx.ConnectConfig(ctx, env.Pool.Config().ConnConfig.Copy())
		}),
		CanaryEvery: 50 * time.Millisecond, SilenceLimit: 400 * time.Millisecond,
		Log: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	require.Eventually(t, service.Hub().Healthy, wait, 20*time.Millisecond)
	epoch := service.Hub().Now()
	time.Sleep(1500 * time.Millisecond)
	assert.True(t, service.Hub().Healthy())
	assert.Equal(t, epoch, service.Hub().Now(), "no restart and no event: the canary only proves the line is alive")
}
