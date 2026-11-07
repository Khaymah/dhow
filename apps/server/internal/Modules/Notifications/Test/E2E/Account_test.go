package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
)

const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0"

func (w *world) orgChannelFor(events ...string) {
	w.t.Helper()
	hook, res := w.create(w.owner, w.orgPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": "org-account", "Url": hookURL, "CurrentFactor": w.factor(),
	})
	require.Equal(w.t, http.StatusCreated, res.Status, string(res.Raw))
	put := w.owner.Do(apptest.Request{Method: http.MethodPut, Path: w.orgPath("/notification-rules"), Body: map[string]any{"Rules": []map[string]any{{
		"ChannelId": hook.Channel.Id, "Events": events, "Statuses": []string{"failed"},
	}}}})
	require.Equal(w.t, http.StatusOK, put.Status, string(put.Raw))
}

func (w *world) signIn(userAgent string) apptest.Response {
	w.t.Helper()
	return w.env.Do(apptest.Request{
		Method: http.MethodPost, Path: "/sessions", Headers: map[string]string{"Origin": apptest.PublicOrigin, "User-Agent": userAgent},
		Body: map[string]string{"Email": w.email, "Password": w.password},
	})
}

func (w *world) bodies() []map[string]any {
	w.t.Helper()
	var out []map[string]any
	for _, r := range w.recv.requests() {
		var m map[string]any
		require.NoError(w.t, json.Unmarshal(r.Body, &m))
		out = append(out, m)
	}
	return out
}

func TestASignInFromANewDeviceMailsTheUserAndReachesTheOrganizationChannelWithoutAnyAddress(t *testing.T) {
	w := newWorld(t)
	w.orgChannelFor("account.new_device_login")
	w.env.SentMail(w.mail)
	before := len(w.mail.Messages())

	require.Equal(t, http.StatusOK, w.signIn(apptest.UserAgent).Status)
	w.drain()
	assert.Empty(t, w.recv.requests(), "the browser of the first sign-in is not new")
	assert.Equal(t, before, len(w.env.SentMail(w.mail)))

	require.Equal(t, http.StatusOK, w.signIn(firefox).Status)
	w.drain()
	require.Len(t, w.recv.requests(), 1)
	body := w.bodies()[0]
	assert.Equal(t, "account.new_device_login", body["Event"])
	assert.Equal(t, "new_device_login", body["Change"])
	assert.Equal(t, "Firefox · Linux", body["Browser"])
	assert.Equal(t, "203.0.113.0/24", body["Network"], "the network part only, never the whole address")
	raw := string(w.recv.requests()[0].Body)
	for _, forbidden := range []string{"203.0.113.7", w.password, "Mozilla", "Cookie", "CsrfToken"} {
		assert.NotContains(t, raw, forbidden)
	}

	mails := w.env.SentMail(w.mail)
	require.Greater(t, len(mails), before)
	last := mails[len(mails)-1]
	assert.Equal(t, w.email, strings.ToLower(last.To))
	assert.Contains(t, last.Body, "Firefox · Linux")
	assert.Contains(t, last.Body, "203.0.113.0/24")
	assert.NotContains(t, last.Body, "203.0.113.7")

	require.Equal(t, http.StatusOK, w.signIn(firefox).Status)
	w.drain()
	assert.Len(t, w.recv.requests(), 1, "the same browser again is not new")
}

func TestAChangeOfTheSecondFactorAndOfMembersReachesTheOrganizationChannel(t *testing.T) {
	w := newWorld(t)
	w.orgChannelFor("account.second_factor_changed", "member.removed", "member.role_changed")
	w.owner.EnableTotp(apptest.PasswordFactor(w.password))
	w.drain()
	require.Len(t, w.recv.requests(), 1)
	assert.Equal(t, "account.second_factor_changed", w.bodies()[0]["Event"])
	assert.Equal(t, "totp_enabled", w.bodies()[0]["Change"])

	other, otherID := w.env.SignedIn(w.org, "member@example.com", "developer")
	_ = other
	w.publish(eventbus.Event{Name: eventbus.IdentityMemberRoleChanged, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, ActorUserID: &w.ownerID, TargetType: "user", TargetID: otherID.String(), Data: map[string]string{"To": "viewer"},
	}})
	w.publish(eventbus.Event{Name: eventbus.IdentityMemberRemoved, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, ActorUserID: &w.ownerID, TargetType: "user", TargetID: otherID.String(),
	}})
	w.drain()
	bodies := w.bodies()
	require.Len(t, bodies, 3)
	assert.Equal(t, "member.role_changed", bodies[1]["Event"])
	assert.Equal(t, "viewer", bodies[1]["Reason"])
	assert.Equal(t, "member.removed", bodies[2]["Event"])
	for _, b := range bodies {
		assert.NotContains(t, b, "UserId")
		assert.NotContains(t, b, "Email")
	}
}

func TestTheAccountEventsAreNotInTheDefaultRule(t *testing.T) {
	w := newWorld(t)
	w.webhook("ops", hookURL)
	w.publish(eventbus.Event{Name: eventbus.AuthTotpEnabled, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, ActorUserID: &w.ownerID, TargetType: "user", TargetID: w.ownerID.String(),
	}})
	w.drain()
	assert.Empty(t, w.recv.requests())
	assert.Zero(t, w.count(`SELECT count(*) FROM "NotificationOutbox" WHERE "ChannelId" IS NOT NULL`))
}

func TestAFailedRunnerRevokeLeavesTheRunnerActiveBecauseTheAuditIsInTheSameTransaction(t *testing.T) {
	w := newWorld(t)
	runner := w.env.SeedRunner(w.org, w.ownerID)
	_, err := w.env.Pool.Exec(context.Background(), `
		CREATE FUNCTION refuse_runner_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit is down'; END $$;
		CREATE TRIGGER refuse_runner_audit BEFORE INSERT ON "AuditEvents" FOR EACH ROW WHEN (NEW."Action" = 'runner.revoked')
		EXECUTE FUNCTION refuse_runner_audit()`)
	require.NoError(t, err)

	path := w.orgPath("/runners/" + runner.String() + "/revoke")
	res := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path})
	assert.GreaterOrEqual(t, res.Status, http.StatusInternalServerError, string(res.Raw))
	assert.Equal(t, "active", w.state(runner), "no audit row, no revoke: the two commit together or not at all")
	assert.Zero(t, w.env.AuditCount("runner.revoked"))

	_, err = w.env.Pool.Exec(context.Background(), `DROP TRIGGER refuse_runner_audit ON "AuditEvents"`)
	require.NoError(t, err)
	ok := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: path})
	require.Equal(t, http.StatusOK, ok.Status, string(ok.Raw))
	assert.Equal(t, "revoked", w.state(runner))
	assert.Equal(t, 1, w.env.AuditCount("runner.revoked"))
}

func (w *world) state(runner uuid.UUID) string {
	w.t.Helper()
	var s string
	require.NoError(w.t, w.env.Pool.QueryRow(context.Background(), `SELECT "State" FROM "Runners" WHERE "Id" = $1`, runner).Scan(&s))
	return s
}
