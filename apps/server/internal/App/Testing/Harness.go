package apptest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	dbtest "github.com/Khaymah/dhow/apps/server/internal/Database/Testing"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	"github.com/Khaymah/dhow/internal/buildinfo"
	"github.com/Khaymah/dhow/internal/logging"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	PublicOrigin = "https://dhow.example.com"
	UserAgent    = "dhow-test-agent"
)

type SyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *SyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type Clock struct{ nanos atomic.Int64 }

func NewClock(start time.Time) *Clock {
	c := &Clock{}
	c.nanos.Store(start.UnixNano())
	return c
}

func (c *Clock) Now() time.Time { return time.Unix(0, c.nanos.Load()).UTC() }

func (c *Clock) Advance(d time.Duration) { c.nanos.Add(int64(d)) }

type FakeBreach struct {
	mu       sync.Mutex
	breached map[string]bool
	err      error
	calls    int
}

func (f *FakeBreach) Add(password string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.breached == nil {
		f.breached = map[string]bool{}
	}
	f.breached[password] = true
}

func (f *FakeBreach) FailWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *FakeBreach) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *FakeBreach) IsBreached(_ context.Context, password string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.breached[password], f.err
}

type fakeMigrations struct{}

func (fakeMigrations) Current(context.Context) (bool, error) { return true, nil }

type Env struct {
	T       *testing.T
	Secrets *FakeSecrets
	Pool    *pgxpool.Pool
	Wired   *app.Wired
	Handler http.Handler
	Logs    *SyncBuffer
	Clock   *Clock
	Breach  *FakeBreach
	Hasher  *auth.Hasher
	Config  config.Config
}

func TestParams() auth.Params {
	return auth.Params{MemoryKiB: 8, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
}

func New(t *testing.T) *Env {
	t.Helper()
	pool := dbtest.NewPool(t)
	return NewWithPool(t, pool)
}

func NewWithPool(t *testing.T, pool *pgxpool.Pool) *Env {
	t.Helper()
	return NewWithPoolAndParams(t, pool, TestParams())
}

type Options struct {
	Pool          *pgxpool.Pool
	Params        *auth.Params
	Mailer        mailer.Sender
	Profile       func(*config.Config)
	Clock         *Clock
	OidcTransport http.RoundTripper
	Secrets       *FakeSecrets
	Triggers      triggers.Sink
	GitLookup     netguard.Lookup
	GitDial       netguard.DialFunc
	Definitions   pipelines.Definitions
	Images        pipelines.ImageResolver
	CheckoutImage string
	RegistryTLS   *tls.Config
	NoManualJobs  bool
	PinBudget     time.Duration

	LogStreamWriteWait time.Duration
	LogReadWait        time.Duration
}

func NewWithPoolAndParams(t *testing.T, pool *pgxpool.Pool, params auth.Params) *Env {
	t.Helper()
	return NewWithOptions(t, Options{Pool: pool, Params: &params})
}

func NewConfigured(t *testing.T, pool *pgxpool.Pool, params auth.Params, configure func(*config.Config)) *Env {
	t.Helper()
	return NewWithOptions(t, Options{Pool: pool, Params: &params, Profile: configure})
}

func NewWithSecrets(t *testing.T, secrets *FakeSecrets) *Env {
	t.Helper()
	return NewWithOptions(t, Options{Secrets: secrets})
}

func NewWithOptions(t *testing.T, o Options) *Env {
	t.Helper()
	pool := o.Pool
	if pool == nil {
		pool = dbtest.NewPool(t)
	}
	params := TestParams()
	if o.Params != nil {
		params = *o.Params
	}
	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	require.NoError(t, err)
	publicURL, err := url.Parse(PublicOrigin)
	require.NoError(t, err)
	cfg := config.Config{
		SecretKey: secret, PublicURL: publicURL, ListenAddr: config.DefaultListenAddr, Artifacts: config.DefaultArtifacts(),
	}
	cfg.Artifacts.BlobDir = PrivateDir(t)
	if o.Profile != nil {
		o.Profile(&cfg)
	}
	logs := &SyncBuffer{}
	log := logging.New(logs, slog.LevelDebug)
	t.Cleanup(func() {
		if strings.Contains(logs.String(), AuditFieldDropped) {
			t.Errorf("the audit log dropped a field of a real event; add it to the allowlist in Modules/Audit or stop sending it:\n%s", droppedLines(logs.String()))
		}
	})
	clock := o.Clock
	if clock == nil {
		clock = NewClock(time.Now().UTC().Truncate(time.Second))
	}
	breach := &FakeBreach{}
	infra := app.Infra{
		Config: cfg, Log: log, Pool: pool, Migrations: fakeMigrations{}, Build: buildinfo.Info{Version: "test", Commit: "test"},
		Breach: breach, HashParams: &params, Now: clock.Now, Mailer: o.Mailer, OidcTransport: o.OidcTransport,
		LogStreamWriteWait: o.LogStreamWriteWait, LogReadWait: o.LogReadWait, Triggers: o.Triggers, GitLookup: o.GitLookup, GitDial: o.GitDial,
		Definitions: o.Definitions, Images: o.Images, CheckoutImage: o.CheckoutImage, RegistryTLS: o.RegistryTLS, ManualJobs: !o.NoManualJobs,
		PinBudget: o.PinBudget,
	}
	if o.Secrets != nil {
		infra.Secrets = o.Secrets
	}
	wired, err := app.Build(t.Context(), infra)
	require.NoError(t, err)
	t.Cleanup(wired.Close)
	startListener(t, wired)
	hasher, err := auth.NewHasher(t.Context(), params, 4)
	require.NoError(t, err)
	return &Env{
		T: t, Pool: pool, Wired: wired, Handler: wired.Handler, Logs: logs, Clock: clock, Breach: breach, Hasher: hasher, Config: cfg,
		Secrets: o.Secrets,
	}
}

func PrivateDir(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700)) //nolint:gosec // A directory needs its execute bit; 0700 keeps it private.
	return dir
}

const AuditFieldDropped = "audit data field dropped"

func droppedLines(logs string) string {
	var out []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, AuditFieldDropped) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

type Response struct {
	Status  int
	Header  http.Header
	Raw     []byte
	Success bool
	Data    json.RawMessage
	Message string
	Meta    *struct{ Page, Limit, Total int }
	Error   *struct {
		Code    string
		Message string
		Details []struct{ Field, Message string }
	}
}

func (r Response) Decode(t testing.TB, into any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(r.Data, into), "data: %s", r.Data)
}

type Request struct {
	Method     string
	Path       string
	Body       any
	Headers    map[string]string
	RemoteAddr string
}

func (e *Env) Do(req Request) Response {
	e.T.Helper()
	var body io.Reader = http.NoBody
	if req.Body != nil {
		switch b := req.Body.(type) {
		case string:
			body = strings.NewReader(b)
		case []byte:
			body = bytes.NewReader(b)
		default:
			raw, err := json.Marshal(b)
			require.NoError(e.T, err)
			body = bytes.NewReader(raw)
		}
	}
	r := httptest.NewRequestWithContext(context.Background(), req.Method, config.APIPrefix+req.Path, body)
	r.RemoteAddr = "203.0.113.7:4444"
	if req.RemoteAddr != "" {
		r.RemoteAddr = req.RemoteAddr
	}
	r.Header.Set("User-Agent", UserAgent)
	if req.Body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range req.Headers {
		if v == "" {
			r.Header.Del(k)
			continue
		}
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.Handler.ServeHTTP(rec, r)
	res := Response{Status: rec.Code, Header: rec.Header(), Raw: rec.Body.Bytes()}
	if len(res.Raw) > 0 && strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		require.NoError(e.T, json.Unmarshal(res.Raw, &res), "response body: %s", res.Raw)
	}
	return res
}

func (e *Env) SetupToken() string {
	e.T.Helper()
	link, created, err := e.Wired.Identity.EnsureSetupToken(context.Background())
	require.NoError(e.T, err)
	require.True(e.T, created)
	_, token, ok := strings.Cut(link, "#")
	require.True(e.T, ok)
	return token
}

type SetupDetails struct {
	OwnerName, Email, Password, OrgName, OrgSlug string
	Token                                        string
}

func DefaultSetup() SetupDetails {
	return SetupDetails{
		OwnerName: "Mona Owner", Email: "Owner@Example.com", Password: "correct horse battery staple",
		OrgName: "Dhow Co", OrgSlug: "dhow-co",
	}
}

func (e *Env) Setup(d SetupDetails) Response {
	e.T.Helper()
	if d.Token == "" {
		d.Token = e.SetupToken()
	}
	return e.Do(Request{Method: http.MethodPost, Path: "/setup", Headers: map[string]string{"Origin": PublicOrigin}, Body: map[string]any{
		"SetupToken": d.Token, "OwnerName": d.OwnerName, "Email": d.Email, "Password": d.Password,
		"OrganizationName": d.OrgName, "OrganizationSlug": d.OrgSlug,
	}})
}

type Client struct {
	Env       *Env
	Cookie    string
	CSRFToken string
	UserID    string
}

func (e *Env) Login(email, password string) (*Client, Response) {
	e.T.Helper()
	res := e.Do(Request{Method: http.MethodPost, Path: "/sessions", Headers: map[string]string{"Origin": PublicOrigin}, Body: map[string]string{
		"Email": email, "Password": password,
	}})
	c := &Client{Env: e}
	if res.Status == http.StatusOK {
		var data struct {
			CsrfToken string
			User      struct{ Id string }
		}
		res.Decode(e.T, &data)
		c.CSRFToken, c.UserID = data.CsrfToken, data.User.Id
		c.Cookie = CookieValue(e.T, res.Header.Get("Set-Cookie"))
	}
	return c, res
}

func CookieValue(t *testing.T, setCookie string) string {
	t.Helper()
	name, rest, ok := strings.Cut(setCookie, "=")
	require.True(t, ok, "Set-Cookie: %q", setCookie)
	require.Equal(t, config.SessionCookieName, name)
	value, _, _ := strings.Cut(rest, ";")
	return value
}

func (c *Client) Do(req Request) Response {
	c.Env.T.Helper()
	headers := map[string]string{"Origin": PublicOrigin}
	if c.Cookie != "" {
		headers["Cookie"] = config.SessionCookieName + "=" + c.Cookie
	}
	if c.CSRFToken != "" {
		headers[config.CSRFHeaderName] = c.CSRFToken
	}
	for k, v := range req.Headers {
		headers[k] = v
	}
	req.Headers = headers
	return c.Env.Do(req)
}

func (c *Client) Get(path string) Response {
	c.Env.T.Helper()
	return c.Do(Request{Method: http.MethodGet, Path: path})
}

func (e *Env) SeedUser(orgID uuid.UUID, email, password, role string) uuid.UUID {
	e.T.Helper()
	ctx := context.Background()
	hash, err := e.Hasher.Hash(ctx, password)
	require.NoError(e.T, err)
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ($1, $2, $3) RETURNING "Id"`, strings.ToLower(email), "Seed "+role, hash).Scan(&id))
	_, err = e.Pool.Exec(ctx, `INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role") VALUES ($1, $2, $3)`, id, orgID, role)
	require.NoError(e.T, err)
	return id
}

func (e *Env) SeedOrganization(name, slug string) uuid.UUID {
	e.T.Helper()
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "Organizations" ("Name", "Slug") VALUES ($1, $2) RETURNING "Id"`, name, slug).Scan(&id))
	return id
}

func (e *Env) Logout(c *Client) Response {
	e.T.Helper()
	return c.Do(Request{Method: http.MethodDelete, Path: "/sessions/current"})
}

func (e *Env) SetupAndLogin() (*Client, SetupDetails) {
	e.T.Helper()
	d := DefaultSetup()
	res := e.Setup(d)
	require.Equal(e.T, http.StatusOK, res.Status, string(res.Raw))
	c, login := e.Login(d.Email, d.Password)
	require.Equal(e.T, http.StatusOK, login.Status, string(login.Raw))
	return c, d
}

func (e *Env) OrganizationID(slug string) uuid.UUID {
	e.T.Helper()
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(), `SELECT "Id" FROM "Organizations" WHERE "Slug" = $1`, slug).Scan(&id))
	return id
}

func (e *Env) SetupAndLoginResponse() (*Client, Response) {
	e.T.Helper()
	d := DefaultSetup()
	require.Equal(e.T, http.StatusOK, e.Setup(d).Status)
	return e.Login(d.Email, d.Password)
}

func (e *Env) SeedUserWithoutMembership(email, password string) uuid.UUID {
	e.T.Helper()
	ctx := context.Background()
	hash, err := e.Hasher.Hash(ctx, password)
	require.NoError(e.T, err)
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "Users" ("Email", "Name", "PasswordHash") VALUES ($1, 'No membership', $2) RETURNING "Id"`, strings.ToLower(email), hash).Scan(&id))
	return id
}

func (e *Env) SeedSession(userID uuid.UUID) *Client {
	e.T.Helper()
	cookie, tokenHash, err := auth.NewSessionToken(config.SessionTokenBytes)
	require.NoError(e.T, err)
	csrf := e.Wired.Options.CSRF.Token(cookie)
	now := e.Clock.Now()
	_, err = e.Pool.Exec(context.Background(),
		`INSERT INTO "Sessions" ("UserId", "TokenHash", "CsrfHash", "LastSeenAt", "ExpiresAt", "IdleExpiresAt", "CreatedAt") VALUES ($1, $2, $3, $4, $5, $6, $4)`,
		userID, tokenHash, e.Wired.Options.CSRF.Hash(csrf), now, now.Add(config.SessionAbsoluteTimeout), now.Add(config.SessionIdleTimeout))
	require.NoError(e.T, err)
	return &Client{Env: e, Cookie: cookie, CSRFToken: csrf, UserID: userID.String()}
}
