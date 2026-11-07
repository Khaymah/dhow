package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
)

func TestAFailedDeploymentIsSentOnceWithASignatureTheReceiverCanVerify(t *testing.T) {
	w := newWorld(t)
	created := w.webhook("ops", hookURL)
	require.NotEmpty(t, created.Secret)
	assert.True(t, notifications.ValidSecret(created.Secret))
	assert.True(t, created.Channel.HasSecret)
	assert.Equal(t, hookHost, created.Channel.Host)

	deployment := w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	w.drain()

	got := w.recv.requests()
	require.Len(t, got, 1, "sent once")
	r := got[0]
	assert.Equal(t, "deployment.finished", r.Header.Get(notifications.HeaderEvent))
	_, err := uuid.Parse(r.Header.Get(notifications.HeaderDelivery))
	require.NoError(t, err)
	assert.Equal(t, deployment.String(), r.Header.Get(notifications.HeaderIdempKey))
	ts := r.Header.Get(notifications.HeaderTimestamp)
	seconds, err := strconv.ParseInt(ts, 10, 64)
	require.NoError(t, err)
	assert.True(t, notifications.Verify(created.Secret, r.Header.Get(notifications.HeaderSignature), ts, r.Body, time.Unix(seconds, 0)),
		"the receiver verifies the signature with the secret it was shown once")
	assert.False(t, notifications.Verify(created.Secret, r.Header.Get(notifications.HeaderSignature), ts, append(r.Body, ' '), time.Unix(seconds, 0)))
	assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(r.Body, &body))
	assert.Equal(t, "failed", body["Status"])
	assert.Equal(t, "production", body["Environment"])
	assert.EqualValues(t, 1, body["Run"])
	assert.Equal(t, "deployment.finished", body["Event"])
	assert.Equal(t, apptest.PublicOrigin+"/projects/"+w.project.String()+"/runs/1", body["Url"])
	assert.Contains(t, body, "EventId")
	for _, forbidden := range []string{"Token", "Secret", "Password", "Log", "Env", "Variables"} {
		assert.NotContains(t, body, forbidden)
	}
}

func TestASuccessfulDeploymentIsNotSentByTheDefaultRule(t *testing.T) {
	w := newWorld(t)
	w.webhook("ops", hookURL)
	w.deploymentEvent(eventbus.DeploymentFinished, "succeeded")
	w.deploymentEvent(eventbus.DeploymentFinished, "canceled")
	w.drain()
	assert.Empty(t, w.recv.requests())
	assert.Zero(t, w.count(`SELECT count(*) FROM "NotificationOutbox"`))

	w.deploymentEvent(eventbus.DeploymentApprovalRequested, "")
	w.deploymentEvent(eventbus.DeploymentReleaseRolledBack, "")
	w.drain()
	got := w.recv.requests()
	require.Len(t, got, 1, "the approval request is sent; a release event carries no deployment to look up here")
	assert.Equal(t, "deployment.approval.requested", got[0].Header.Get(notifications.HeaderEvent))
}

func TestTheNotificationIsWrittenInTheSameTransactionAsTheEvent(t *testing.T) {
	w := newWorld(t)
	w.webhook("ops", hookURL)
	ctx := context.Background()
	event := eventbus.Event{Name: eventbus.DeploymentFinished, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, TargetType: "deployment", TargetID: uuid.NewString(),
		Data: map[string]string{"EnvironmentId": w.envID.String(), "State": "failed", "RunId": w.run.String(), "ProjectId": w.project.String()},
	}}

	rolledBack := dbtx.Run(ctx, w.env.Pool, func(q *sqlc.Queries) error {
		a := event.Payload.(eventbus.Audited)
		a.Tx = q
		if err := w.env.Wired.Notifications.Handle(ctx, eventbus.Event{Name: event.Name, Payload: a}); err != nil {
			return err
		}
		return assert.AnError
	})
	require.ErrorIs(t, rolledBack, assert.AnError)
	assert.Zero(t, w.count(`SELECT count(*) FROM "NotificationOutbox"`), "no commit, no notification")

	w.publish(event)
	assert.Equal(t, 1, w.count(`SELECT count(*) FROM "NotificationOutbox" WHERE "State" = 'pending'`), "the commit brought the row with it")
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	assert.Equal(t, 2, w.count(`SELECT count(*) FROM "NotificationOutbox"`), "another event id is another notification")
}

func TestTheSameEventPublishedTwiceIsQueuedOnce(t *testing.T) {
	w := newWorld(t)
	w.webhook("ops", hookURL)
	deployment := uuid.NewString()
	ev := eventbus.Event{Name: eventbus.DeploymentFinished, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, TargetType: "deployment", TargetID: deployment,
		Data: map[string]string{"EnvironmentId": w.envID.String(), "State": "failed", "RunId": w.run.String(), "ProjectId": w.project.String()},
	}}
	w.publish(ev)
	w.publish(ev)
	assert.Equal(t, 1, w.count(`SELECT count(*) FROM "NotificationOutbox"`))
}

func TestAReceiverThatFailsThreeTimesGetsTheFourthAttemptOnTheBackoffSchedule(t *testing.T) {
	w := newWorld(t, 500, 502, 503, 200)
	w.webhook("ops", hookURL)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")

	w.drain()
	require.Len(t, w.recv.requests(), 1)
	w.env.Clock.Advance(time.Minute - time.Second)
	w.drain()
	require.Len(t, w.recv.requests(), 1, "one second early")
	w.env.Clock.Advance(time.Second)
	w.drain()
	require.Len(t, w.recv.requests(), 2)
	w.env.Clock.Advance(5 * time.Minute)
	w.drain()
	require.Len(t, w.recv.requests(), 3)
	w.env.Clock.Advance(30*time.Minute - time.Second)
	w.drain()
	require.Len(t, w.recv.requests(), 3)
	w.env.Clock.Advance(time.Second)
	w.drain()
	require.Len(t, w.recv.requests(), 4)

	list := w.deliveries(w.projectPath("/notification-deliveries"))
	require.Len(t, list, 1)
	assert.Equal(t, "sent", list[0].State)
	assert.Equal(t, 4, list[0].Attempts)
	require.NotNil(t, list[0].LastStatus)
	assert.Equal(t, 200, *list[0].LastStatus)
	ids := map[string]bool{}
	for _, r := range w.recv.requests() {
		ids[r.Header.Get(notifications.HeaderDelivery)] = true
	}
	assert.Len(t, ids, 1, "every attempt carries the same delivery id, so the receiver can drop a repeat")
}

func TestADeliveryThatFailsFiveTimesIsDeadUntilSomeoneRetriesIt(t *testing.T) {
	w := newWorld(t, 500)
	w.webhook("ops", hookURL)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	for _, wait := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour} {
		w.drain()
		w.env.Clock.Advance(wait)
	}
	assert.Len(t, w.recv.requests(), 5, "five attempts and no more")
	w.relogin()

	list := w.deliveries(w.projectPath("/notification-deliveries"))
	require.Len(t, list, 1)
	assert.Equal(t, "dead", list[0].State)
	assert.Equal(t, "http_status", list[0].LastError)
	assert.Equal(t, 5, list[0].Attempts)

	retry := w.projectPath("/notification-deliveries/" + list[0].Id + "/retry")
	w.recv.script(200)
	res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: retry})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	w.drain()
	assert.Len(t, w.recv.requests(), 6)
	again := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: retry})
	assert.Equal(t, http.StatusConflict, again.Status)
	require.NotNil(t, again.Error)
	assert.Equal(t, "DELIVERY_NOT_RETRYABLE", again.Error.Code)
	assert.Equal(t, "sent", w.deliveries(w.projectPath("/notification-deliveries"))[0].State)
}

func TestAClientErrorIsFinalAndARedirectIsNeverFollowed(t *testing.T) {
	w := newWorld(t, 404)
	w.webhook("ops", hookURL)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	list := w.deliveries(w.projectPath("/notification-deliveries"))
	assert.Equal(t, "dead", list[0].State, "a 404 will not change by waiting")
	assert.Equal(t, 1, list[0].Attempts)

	w.recv.mu.Lock()
	w.recv.location = "https://example.com/latest/meta-data/"
	w.recv.mu.Unlock()
	w.recv.script(302)
	before := len(w.recv.requests())
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Len(t, w.recv.requests(), before+1, "the redirect target was not requested")
	var code string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "LastError" FROM "NotificationOutbox" ORDER BY "CreatedAt" DESC LIMIT 1`).Scan(&code))
	assert.Equal(t, "redirect", code)
}

func TestEveryChannelOfTheProjectGetsTheEvent(t *testing.T) {
	w := newWorld(t)
	first := w.webhook("first", hookURL)
	second := w.webhook("second", hookURL+"2")
	assert.NotEqual(t, first.Channel.Id, second.Channel.Id)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Len(t, w.recv.requests(), 2, "both channels get the event")
}
