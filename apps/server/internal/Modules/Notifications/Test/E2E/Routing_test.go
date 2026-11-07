package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
)

func recipients(messages []mailer.Message) []string {
	var out []string
	for _, m := range messages {
		out = append(out, strings.ToLower(m.To))
	}
	return out
}

func TestAnEmailChannelMailsTheMembersOfTheChosenRolesInTheLanguageOfTheChannel(t *testing.T) {
	w := newWorld(t)
	w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.env.SignedIn(w.org, "maint@example.com", "maintainer")
	w.env.SignedIn(w.org, "dev@example.com", "developer")
	w.emailChannel("team", "en")

	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	sent := w.env.SentMail(w.mail)
	assert.ElementsMatch(t, []string{w.email, "admin@example.com", "maint@example.com"}, recipients(sent), "owner, admin and maintainer; the developer is not on the default roles")
	require.NotEmpty(t, sent)
	assert.Contains(t, sent[0].Subject, "Deployment failed", "the language of the channel, not of the member")
	assert.Contains(t, sent[0].Body, "Environment: production")
	assert.Contains(t, sent[0].Body, apptest.PublicOrigin+"/projects/"+w.project.String()+"/runs/1")
	assert.NotContains(t, sent[0].Body, "{")
}

func TestAMemberWhoMutedTheNotificationsGetsNoChannelEmailButAccountMailStillArrives(t *testing.T) {
	w := newWorld(t)
	admin, _ := w.env.SignedIn(w.org, "admin@example.com", "admin")
	w.emailChannel("team", "ar")
	res := admin.Do(apptest.Request{Method: http.MethodPut, Path: "/me/notification-mute", Body: map[string]any{"Muted": true}})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	got := admin.Get("/me/notification-mute")
	var mute struct{ Muted bool }
	got.Decode(t, &mute)
	assert.True(t, mute.Muted)

	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	sent := w.env.SentMail(w.mail)
	assert.Equal(t, []string{w.email}, recipients(sent))
	assert.Contains(t, sent[0].Subject, "فشل النشر")

	ok := admin.Do(apptest.Request{Method: http.MethodPut, Path: "/me/notification-mute", Body: map[string]any{"Muted": false}})
	require.Equal(t, http.StatusOK, ok.Status)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Contains(t, recipients(w.env.SentMail(w.mail)), "admin@example.com")
}

func TestATestMessageGoesOnlyToTheCallerAndIsRateLimited(t *testing.T) {
	w := newWorld(t)
	w.env.SignedIn(w.org, "admin@example.com", "admin")
	channel := w.emailChannel("team", "en")
	path := w.projectPath("/notification-channels/" + channel.Channel.Id + "/test")
	for range 5 {
		res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path})
		require.Equal(t, http.StatusAccepted, res.Status, string(res.Raw))
	}
	limited := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path})
	assert.Equal(t, http.StatusTooManyRequests, limited.Status)
	require.NotNil(t, limited.Error)
	assert.Equal(t, "TEST_RATE_LIMITED", limited.Error.Code)
	w.drain()
	sent := w.env.SentMail(w.mail)
	assert.Equal(t, []string{w.email, w.email, w.email, w.email, w.email}, recipients(sent), "a test goes to the person who asked, not to the roles")
	assert.Contains(t, sent[0].Subject, "Test message")
	w.env.Clock.Advance(2 * time.Minute)
	w.relogin()
	again := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path})
	assert.Equal(t, http.StatusAccepted, again.Status, "a minute later the limit has reset")
}

func TestARuleChoosesTheEventsTheEnvironmentsAndTheStatuses(t *testing.T) {
	w := newWorld(t)
	w.env.SeedEnvironment(w.project, "staging")
	hook := w.webhook("ops", hookURL)
	rules := w.owner.Get(w.projectPath("/notification-rules"))
	require.Equal(t, http.StatusOK, rules.Status, string(rules.Raw))
	var current []struct {
		ChannelId                             string
		Events, Environments, Statuses, Roles []string
		Version                               int64
	}
	rules.Decode(t, &current)
	require.Len(t, current, 1)
	assert.ElementsMatch(t, notifications.DefaultStatuses(), current[0].Statuses)
	assert.ElementsMatch(t, notifications.DefaultEvents(), current[0].Events)
	assert.Empty(t, current[0].Environments)

	put := w.owner.Do(apptest.Request{Method: http.MethodPut, Path: w.projectPath("/notification-rules"), Body: map[string]any{"Rules": []map[string]any{{
		"ChannelId": hook.Channel.Id, "Events": []string{"deployment.finished"}, "Environments": []string{"staging"},
		"Statuses": []string{"succeeded", "failed"},
	}}}})
	require.Equal(t, http.StatusOK, put.Status, string(put.Raw))
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Empty(t, w.recv.requests(), "the rule is for staging; the event is production")
	assert.Equal(t, 1, w.env.AuditCount("notification.rule_updated"))

	bad := w.owner.Do(apptest.Request{Method: http.MethodPut, Path: w.projectPath("/notification-rules"), Body: map[string]any{"Rules": []map[string]any{{
		"ChannelId": hook.Channel.Id, "Events": []string{"deployment.finished"}, "Statuses": []string{"exploded"},
	}}}})
	assert.Equal(t, http.StatusUnprocessableEntity, bad.Status)
	roles := w.owner.Do(apptest.Request{Method: http.MethodPut, Path: w.projectPath("/notification-rules"), Body: map[string]any{"Rules": []map[string]any{{
		"ChannelId": hook.Channel.Id, "Events": []string{"deployment.finished"}, "Statuses": []string{"failed"}, "Roles": []string{"owner"},
	}}}})
	assert.Equal(t, http.StatusUnprocessableEntity, roles.Status, "roles belong to email channels only")
}

func TestOnlyFailedRunsAreAnnounced(t *testing.T) {
	w := newWorld(t)
	w.webhook("ops", hookURL)
	for _, state := range []string{"succeeded", "canceled", "failed"} {
		w.publish(eventbus.Event{Name: eventbus.RunFinished, Payload: eventbus.Audited{
			OrganizationIDs: []uuid.UUID{w.org}, TargetType: "run", TargetID: uuid.NewString(),
			Data: map[string]string{"State": state, "Reason": "JOBS_FAILED", "ProjectId": w.project.String(), "Number": "7", "Ref": "refs/heads/main"},
		}})
	}
	w.drain()
	got := w.recv.requests()
	require.Len(t, got, 1)
	var body map[string]any
	require.NoError(t, json.Unmarshal(got[0].Body, &body))
	assert.Equal(t, "run.finished", body["Event"])
	assert.Equal(t, "JOBS_FAILED", body["Reason"])
	assert.EqualValues(t, 7, body["Run"])
}

func TestARunnerThatStopsAnsweringIsAnnouncedOncePerOutage(t *testing.T) {
	w := newWorld(t)
	orgHook, res := w.create(w.owner, w.orgPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "org-ops", "Url": hookURL, "CurrentFactor": w.factor(),
	})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	put := w.owner.Do(apptest.Request{Method: http.MethodPut, Path: w.orgPath("/notification-rules"), Body: map[string]any{"Rules": []map[string]any{{
		"ChannelId": orgHook.Channel.Id, "Events": []string{"runner.offline", "deployment.finished"}, "Statuses": []string{"failed"},
	}}}})
	require.Equal(t, http.StatusOK, put.Status, string(put.Raw))

	runner := w.env.SeedRunner(w.org, w.ownerID)
	setSeen := func(ago time.Duration) {
		_, err := w.env.Pool.Exec(context.Background(), `UPDATE "Runners" SET "LastSeenAt" = $2 WHERE "Id" = $1`, runner, w.env.Clock.Now().Add(-ago))
		require.NoError(t, err)
	}
	scan := func() {
		require.NoError(t, w.env.Wired.Notifications.ScanRunners(context.Background()))
		w.drain()
	}
	setSeen(time.Minute)
	scan()
	assert.Empty(t, w.recv.requests(), "a runner seen a minute ago is online")
	setSeen(10 * time.Minute)
	scan()
	scan()
	require.Len(t, w.recv.requests(), 1, "one notice for the whole outage")
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.recv.requests()[0].Body, &body))
	assert.Equal(t, "runner.offline", body["Event"])
	assert.Equal(t, "seeded", body["Runner"])
	setSeen(time.Second)
	scan()
	setSeen(10 * time.Minute)
	scan()
	assert.Len(t, w.recv.requests(), 2, "a new outage after it came back is a new notice")

	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	assert.Len(t, w.recv.requests(), 3, "the channel of the organization also gets the events of every project")
}

func TestAnOrganizationChannelIsKeptApartFromTheProjectChannels(t *testing.T) {
	w := newWorld(t)
	_, res := w.create(w.owner, w.orgPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "same-name", "Url": hookURL, "CurrentFactor": w.factor(),
	})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	w.webhook("same-name", hookURL)
	_, dup := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "SAME-name", "Url": hookURL, "CurrentFactor": w.factor(),
	})
	require.NotNil(t, dup.Error)
	assert.Equal(t, "CHANNEL_EXISTS", dup.Error.Code)
	list := w.owner.Get(w.projectPath("/notification-channels"))
	assert.Equal(t, 1, list.Meta.Total)
	orgList := w.owner.Get(w.orgPath("/notification-channels"))
	assert.Equal(t, 1, orgList.Meta.Total)
}
