package e2e_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	hookHost   = "example.com"
	hookPath   = "/hooks/T0123/B4567/SECRETPATHTOKEN9d8c7b6a"
	hookURL    = "https://" + hookHost + hookPath
	publicAddr = "203.0.113.50"
	allowedNet = "10.0.0.0/8"
	allowedIP  = "10.20.30.40"
)

type received struct {
	Header http.Header
	Body   []byte
}

type receiver struct {
	srv      *httptest.Server
	mu       sync.Mutex
	got      []received
	statuses []int
	location string
}

func newReceiver(t *testing.T, statuses ...int) *receiver {
	t.Helper()
	r := &receiver{statuses: statuses}
	r.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.got = append(r.got, received{Header: req.Header.Clone(), Body: body})
		status := http.StatusOK
		if len(r.statuses) > 0 {
			status = r.statuses[0]
			if len(r.statuses) > 1 {
				r.statuses = r.statuses[1:]
			}
		}
		location := r.location
		r.mu.Unlock()
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"echo":"SECRETPATHTOKEN9d8c7b6a"}`))
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) requests() []received {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]received(nil), r.got...)
}

func (r *receiver) script(statuses ...int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = statuses
}

type resolver struct {
	mu    sync.Mutex
	hosts map[string][]netip.Addr
}

func (l *resolver) set(host string, addrs ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, netip.MustParseAddr(a))
	}
	l.hosts[host] = out
}

func (l *resolver) lookup(_ context.Context, host string) ([]netip.Addr, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if addrs, ok := l.hosts[host]; ok {
		return addrs, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

type world struct {
	t        *testing.T
	env      *apptest.Env
	owner    *apptest.Client
	password string
	email    string
	ownerID  uuid.UUID
	org      uuid.UUID
	project  uuid.UUID
	envID    uuid.UUID
	run      uuid.UUID
	recv     *receiver
	dns      *resolver
	dials    atomic.Int32
	mail     *apptest.FakeMailer
}

func newWorld(t *testing.T, statuses ...int) *world {
	t.Helper()
	w := &world{t: t, recv: newReceiver(t, statuses...), dns: &resolver{hosts: map[string][]netip.Addr{}}, mail: &apptest.FakeMailer{}}
	w.dns.set(hookHost, allowedIP)
	pool := x509.NewCertPool()
	pool.AddCert(w.recv.srv.Certificate())
	target := w.recv.srv.Listener.Addr().String()
	w.env = apptest.NewWithOptions(t, apptest.Options{
		Mailer:       w.mail,
		NotifyLookup: w.dns.lookup,
		NotifyTLS:    &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		NotifyDial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			w.dials.Add(1)
			var d net.Dialer
			return d.DialContext(ctx, network, target)
		},
		Profile: func(c *config.Config) {
			c.Notify = config.DefaultNotify()
			c.Notify.AllowPrivate = []netip.Prefix{netip.MustParsePrefix(allowedNet)}
		},
	})
	var d apptest.SetupDetails
	w.owner, d = w.env.SetupAndLogin()
	w.password, w.email = d.Password, strings.ToLower(d.Email)
	w.org = w.env.OrganizationID(d.OrgSlug)
	w.ownerID = uuid.MustParse(w.owner.UserID)
	w.project = w.env.SeedProject(w.org, w.ownerID)
	w.envID = w.env.SeedEnvironment(w.project, "production")
	w.run = w.env.SeedRun(w.project, "pending")
	return w
}

func (w *world) relogin() {
	w.t.Helper()
	c, res := w.env.Login(w.email, w.password)
	require.Equal(w.t, http.StatusOK, res.Status, string(res.Raw))
	w.owner = c
}

func (w *world) factor() map[string]any { return apptest.PasswordFactor(w.password) }

func (w *world) projectPath(rest string) string {
	return apptest.OrgPath(w.org, "/projects/"+w.project.String()+rest)
}

func (w *world) orgPath(rest string) string { return apptest.OrgPath(w.org, rest) }

type channelView struct {
	Id                  string
	ProjectId           string
	Kind, Name, Locale  string
	Enabled             bool
	DisabledReason      string
	Host                string
	HasSecret           bool
	Version             int64
	ConsecutiveFailures int
}

type createdView struct {
	Channel channelView
	Secret  string
}

func (w *world) create(c *apptest.Client, path string, body map[string]any) (createdView, apptest.Response) {
	w.t.Helper()
	res := c.Do(apptest.Request{Method: http.MethodPost, Path: path, Body: body})
	var out createdView
	if res.Status == http.StatusCreated {
		res.Decode(w.t, &out)
	}
	return out, res
}

func (w *world) webhook(name, url string) createdView {
	w.t.Helper()
	out, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "webhook", "Name": name, "Url": url, "CurrentFactor": w.factor(),
	})
	require.Equal(w.t, http.StatusCreated, res.Status, string(res.Raw))
	return out
}

func (w *world) emailChannel(name, locale string) createdView {
	w.t.Helper()
	out, res := w.create(w.owner, w.projectPath("/notification-channels"), map[string]any{
		"Kind": "email", "Name": name, "Locale": locale, "CurrentFactor": w.factor(),
	})
	require.Equal(w.t, http.StatusCreated, res.Status, string(res.Raw))
	return out
}

func (w *world) deploymentEvent(name, state string) uuid.UUID {
	w.t.Helper()
	deployment := uuid.New()
	w.publish(eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{w.org}, TargetType: "deployment", TargetID: deployment.String(),
		Data: map[string]string{
			"EnvironmentId": w.envID.String(), "State": state, "RunId": w.run.String(), "ProjectId": w.project.String(),
		},
	}})
	return deployment
}

func (w *world) publish(ev eventbus.Event) {
	w.t.Helper()
	a, ok := ev.Payload.(eventbus.Audited)
	require.True(w.t, ok)
	err := dbtx.Run(context.Background(), w.env.Pool, func(q *sqlc.Queries) error {
		a.Tx = q
		return w.env.Wired.Notifications.Handle(context.Background(), eventbus.Event{Name: ev.Name, Payload: a})
	})
	require.NoError(w.t, err)
}

func (w *world) drain() {
	w.t.Helper()
	require.NoError(w.t, w.env.Wired.Notifications.Drain(context.Background()))
}

func (w *world) count(query string, args ...any) int {
	w.t.Helper()
	var n int
	require.NoError(w.t, w.env.Pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

type deliveryView struct {
	Id, ChannelId, ChannelName, Event, State, LastError string
	Attempts                                            int
	LastStatus                                          *int
}

func (w *world) deliveries(path string) []deliveryView {
	w.t.Helper()
	res := w.owner.Get(path)
	require.Equal(w.t, http.StatusOK, res.Status, string(res.Raw))
	var out []deliveryView
	res.Decode(w.t, &out)
	return out
}
