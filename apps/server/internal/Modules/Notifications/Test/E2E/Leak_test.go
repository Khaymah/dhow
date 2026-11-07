package e2e_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications/Gen"
)

func TestNoResponseTypeCarriesAnAddressASecretOrASignature(t *testing.T) {
	forbidden := []string{"Url", "URL", "Secret", "Config", "Sealed", "Signature", "Token", "Password"}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[gen.NotificationChannel](), reflect.TypeFor[gen.NotificationRule](), reflect.TypeFor[gen.NotificationDelivery](),
		reflect.TypeFor[gen.NotificationMute](),
	} {
		for i := range typ.NumField() {
			field := typ.Field(i)
			if field.Name == "HasSecret" {
				assert.Equal(t, reflect.Bool, field.Type.Kind(), "HasSecret says only whether a secret exists")
				continue
			}
			for _, bad := range forbidden {
				assert.NotContains(t, field.Name, bad, "%s.%s", typ.Name(), field.Name)
			}
		}
	}
	created := reflect.TypeFor[gen.CreatedNotificationChannel]()
	var names []string
	for i := range created.NumField() {
		names = append(names, created.Field(i).Name)
	}
	assert.Equal(t, []string{"Channel", "Secret"}, names, "only the answer to a create or a rotate carries the secret, once")
}

func dumpEverything(t *testing.T, w *world) map[string][]byte {
	t.Helper()
	ctx := context.Background()
	rows, err := w.env.Pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY 1`)
	require.NoError(t, err)
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	conn, err := w.env.Pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	out := map[string][]byte{}
	for _, table := range tables {
		var buf bytes.Buffer
		_, err := conn.Conn().PgConn().CopyTo(ctx, &buf, "COPY (SELECT * FROM "+pgx.Identifier{table}.Sanitize()+") TO STDOUT")
		require.NoError(t, err, table)
		out[table] = buf.Bytes()
	}
	out["server.log"] = []byte(w.env.Logs.String())
	return out
}

func TestTheAddressTheSecretAndTheSignatureAreNowhereButInTheSealedColumnAndTheReceiversRequest(t *testing.T) {
	w := newWorld(t, 500, 500, 200)
	created := w.webhook("ops", hookURL)
	secret := created.Secret
	require.NotEmpty(t, secret)
	responses := [][]byte{}
	keep := func(res apptest.Response) apptest.Response {
		responses = append(responses, res.Raw)
		return res
	}
	channel := w.projectPath("/notification-channels/" + created.Channel.Id)

	keep(w.owner.Get(w.projectPath("/notification-channels")))
	keep(w.owner.Get(channel))
	keep(w.owner.Get(w.projectPath("/notification-rules")))
	rename := keep(w.owner.Do(apptest.Request{Method: http.MethodPut, Path: channel, Body: map[string]any{
		"Name": "ops", "Locale": "en", "Version": created.Channel.Version, "CurrentFactor": w.factor(),
	}}))
	require.Equal(t, http.StatusOK, rename.Status, string(rename.Raw))
	var renamed createdView
	rename.Decode(t, &renamed)
	assert.Empty(t, renamed.Secret, "a change that does not rotate shows no secret")

	test := keep(w.owner.Do(apptest.Request{Method: http.MethodPost, Path: channel + "/test"}))
	require.Equal(t, http.StatusAccepted, test.Status, string(test.Raw))
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	w.env.Clock.Advance(time.Minute)
	w.drain()
	w.env.Clock.Advance(5 * time.Minute)
	w.drain()
	keep(w.owner.Get(w.projectPath("/notification-deliveries")))

	rotate := keep(w.owner.Do(apptest.Request{Method: http.MethodPut, Path: channel, Body: map[string]any{
		"Name": "ops", "Locale": "en", "Version": renamed.Channel.Version, "RotateSecret": true, "Url": hookURL + "-v2", "CurrentFactor": w.factor(),
	}}))
	require.Equal(t, http.StatusOK, rotate.Status, string(rotate.Raw))
	var rotated createdView
	rotate.Decode(t, &rotated)
	require.NotEmpty(t, rotated.Secret)
	assert.NotEqual(t, secret, rotated.Secret)
	assert.Equal(t, 1, bytes.Count(rotate.Raw, []byte(rotated.Secret)), "the new secret is shown once")

	sent := w.recv.requests()
	require.NotEmpty(t, sent)
	signatures := []string{}
	for _, r := range sent {
		signatures = append(signatures, r.Header.Get(notifications.HeaderSignature))
	}

	needles := map[string][]byte{
		"address": []byte(hookURL), "path token": []byte("SECRETPATHTOKEN9d8c7b6a"), "first secret": []byte(secret),
		"second secret": []byte(rotated.Secret), "second address": []byte(hookURL + "-v2"),
	}
	for i, s := range signatures {
		needles["signature "+string(rune('a'+i))] = []byte(s)
	}
	for _, raw := range responses {
		for name, needle := range needles {
			assert.False(t, bytes.Contains(raw, needle) && name != "second secret", "a response carries the %s", name)
		}
	}
	for table, content := range dumpEverything(t, w) {
		for name, needle := range needles {
			assert.False(t, bytes.Contains(content, needle), "%s holds the %s in clear text", table, name)
			assert.False(t, bytes.Contains(content, []byte(hex.EncodeToString(needle))), "%s holds the %s as hex", table, name)
		}
	}
	for _, event := range []string{"notification.channel_created", "notification.channel_updated", "notification.test_sent"} {
		assert.Positive(t, w.env.AuditCount(event), event)
	}
}

func TestADeadDeliveryShowsACodeAndNeverTheAnswerOfTheReceiver(t *testing.T) {
	w := newWorld(t, 400)
	w.webhook("ops", hookURL)
	w.deploymentEvent(eventbus.DeploymentFinished, "failed")
	w.drain()
	list := w.deliveries(w.projectPath("/notification-deliveries"))
	require.Len(t, list, 1)
	assert.Equal(t, "dead", list[0].State)
	assert.Equal(t, "http_status", list[0].LastError)
	require.NotNil(t, list[0].LastStatus)
	assert.Equal(t, 400, *list[0].LastStatus)
	raw := w.owner.Get(w.projectPath("/notification-deliveries")).Raw
	assert.NotContains(t, string(raw), "SECRETPATHTOKEN", "the receiver echoed the path token in its answer; Dhow keeps none of it")
}
