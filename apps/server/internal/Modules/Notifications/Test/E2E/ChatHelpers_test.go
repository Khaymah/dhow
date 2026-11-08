package e2e_test

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	notifyreceiver "github.com/Khaymah/dhow/deploy/e2e-targets/notify-receiver/receiver"
)

const (
	slackPath     = "/services/T0123ABCD/B4567EFGH/xoxbSLACKTOKEN0123456789ab"
	slackURL      = "https://hooks.slack.com" + slackPath
	discordPath   = "/api/webhooks/123456789012345678/DISCORDTOKEN_abc-123_xyz"
	discordURL    = "https://discord.com" + discordPath
	telegramToken = "123456789" + ":AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
	telegramChat  = "-1001234567890"
	hostileRef    = "refs/heads/<!channel> @everyone @here <@123456789> *x* [y](https://evil.example)"
)

type chatWorld struct {
	*world
	rec *notifyreceiver.Receiver
	srv *httptest.Server
}

func newChatWorld(t *testing.T) *chatWorld {
	t.Helper()
	rec := notifyreceiver.New()
	cfg, caPEM, err := notifyreceiver.NewTLS(time.Now())
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(rec.Handler())
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caPEM))

	w := &world{t: t, dns: &resolver{hosts: map[string][]netip.Addr{}}, mail: &apptest.FakeMailer{}}
	w.start(pool, srv.Listener.Addr().String(), "hooks.slack.com", "discord.com", "discordapp.com", "api.telegram.org")
	return &chatWorld{world: w, rec: rec, srv: srv}
}

func (c *chatWorld) channel(kind, name, locale string) createdView {
	c.t.Helper()
	body := map[string]any{"Kind": kind, "Name": name, "Locale": locale, "CurrentFactor": c.factor()}
	switch kind {
	case "slack":
		body["Url"] = slackURL
	case "discord":
		body["Url"] = discordURL
	case "telegram":
		body["BotToken"], body["ChatId"] = telegramToken, telegramChat
	}
	out, res := c.create(c.owner, c.projectPath("/notification-channels"), body)
	require.Equal(c.t, http.StatusCreated, res.Status, string(res.Raw))
	return out
}

func (c *chatWorld) hostileRun() {
	c.t.Helper()
	_, err := c.env.Pool.Exec(context.Background(), `UPDATE "PipelineRuns" SET "Ref" = $1 WHERE "Id" = $2`, hostileRef, c.run)
	require.NoError(c.t, err)
}

func (c *chatWorld) put(path string, body map[string]any) apptest.Response {
	c.t.Helper()
	body["CurrentFactor"] = c.factor()
	return c.owner.Do(apptest.Request{Method: http.MethodPut, Path: path, Body: body})
}
