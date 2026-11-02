package apptest

import (
	"context"
	"crypto/ed25519"
	"crypto/hpke"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

const RunnerBaseURL = PublicOrigin + config.RunnerAPIPrefix

type handlerTransport struct {
	handler http.Handler
	remote  string
	headers http.Header
}

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.RemoteAddr = t.remote
	r.RequestURI = req.URL.RequestURI()
	for k, v := range t.headers {
		r.Header[k] = v
	}
	rec := httptest.NewRecorder()
	t.handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}

type RunnerClient struct {
	Registration runnerv1connect.RegistrationServiceClient
	Auth         runnerv1connect.AuthServiceClient
	Lease        runnerv1connect.LeaseServiceClient
	Logs         runnerv1connect.LogServiceClient
	HTTP         *http.Client
}

type RunnerClientOptions struct {
	Remote      string
	Protocol    string
	AccessToken string
	Headers     http.Header
}

func (e *Env) RunnerClient(o RunnerClientOptions) *RunnerClient {
	if o.Remote == "" {
		o.Remote = "198.51.100.10:5555"
	}
	if o.Protocol == "" {
		o.Protocol = config.RunnerProtocolVersion
	}
	headers := http.Header{}
	for k, v := range o.Headers {
		headers[http.CanonicalHeaderKey(k)] = v
	}
	if o.Protocol != "-" {
		headers.Set(config.RunnerProtocolHeader, o.Protocol)
	}
	headers.Set(config.RunnerVersionHeader, "test")
	if o.AccessToken != "" {
		headers.Set("Authorization", "Bearer "+o.AccessToken)
	}
	hc := &http.Client{Transport: handlerTransport{handler: e.Handler, remote: o.Remote, headers: headers}}
	return &RunnerClient{
		Registration: runnerv1connect.NewRegistrationServiceClient(hc, RunnerBaseURL),
		Auth:         runnerv1connect.NewAuthServiceClient(hc, RunnerBaseURL),
		Lease:        runnerv1connect.NewLeaseServiceClient(hc, RunnerBaseURL),
		Logs:         runnerv1connect.NewLogServiceClient(hc, RunnerBaseURL),
		HTTP:         hc,
	}
}

type RunnerIdentity struct {
	Signing    ed25519.PrivateKey
	Encryption hpke.PrivateKey
}

func NewRunnerIdentity(t testing.TB) *RunnerIdentity {
	t.Helper()
	_, signing, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	enc, err := hpke.MLKEM768X25519().GenerateKey()
	require.NoError(t, err)
	return &RunnerIdentity{Signing: signing, Encryption: enc}
}

func (r *RunnerIdentity) SigningPublicKey() []byte {
	pub, _ := r.Signing.Public().(ed25519.PublicKey)
	return pub
}

func (r *RunnerIdentity) EncryptionPublicKey() []byte { return r.Encryption.PublicKey().Bytes() }

func (r *RunnerIdentity) RegisterRequest(token string) *runnerv1.RegisterRequest {
	sk, ek := r.SigningPublicKey(), r.EncryptionPublicKey()
	return &runnerv1.RegisterRequest{
		Token: token, Name: "runner-1", Os: "linux", Arch: "amd64", Version: "0.1.0",
		IsolationLevels: []runnerv1.Isolation{runnerv1.Isolation_ISOLATION_CONTAINER}, Capacity: 2,
		SigningPublicKey: sk, EncryptionPublicKey: ek,
		Proof: ed25519.Sign(r.Signing, runnersig.RegisterMessage(PublicOrigin, sk, ek)),
	}
}

func (r *RunnerIdentity) SignWithFeatures(challenge *runnerv1.IssuedChallenge, features []string) []byte {
	return ed25519.Sign(r.Signing, runnersig.AuthFeaturesMessage(PublicOrigin, config.RunnerServerKeyVersion, challenge.GetChallengeBytes(), features))
}

func (r *RunnerIdentity) Sign(challenge *runnerv1.IssuedChallenge) []byte {
	return ed25519.Sign(r.Signing, runnersig.AuthMessage(PublicOrigin, config.RunnerServerKeyVersion, challenge.GetChallengeBytes()))
}

func (e *Env) Authenticate(rc *RunnerClient, id *RunnerIdentity, runnerID string) (string, error) {
	e.T.Helper()
	ctx := context.Background()
	ref := &runnerv1.RunnerRef{RunnerId: runnerID}
	ch, err := rc.Auth.Challenge(ctx, connect.NewRequest(&runnerv1.ChallengeRequest{Runner: ref}))
	if err != nil {
		return "", err
	}
	res, err := rc.Auth.Authenticate(ctx, connect.NewRequest(&runnerv1.AuthenticateRequest{
		Runner: ref, Challenge: ch.Msg.GetChallenge(), Signature: id.Sign(ch.Msg.GetChallenge()),
	}))
	if err != nil {
		return "", err
	}
	return res.Msg.GetAccessToken(), nil
}

type RegistrationTokenSpec struct {
	Labels    []string
	MaxUses   int
	ExpiresIn time.Duration
}

func (e *Env) CreateRegistrationToken(c *Client, org uuid.UUID, spec RegistrationTokenSpec) (string, string) {
	e.T.Helper()
	body := map[string]any{}
	if spec.Labels != nil {
		body["Labels"] = spec.Labels
	}
	if spec.MaxUses > 0 {
		body["MaxUses"] = spec.MaxUses
	}
	if spec.ExpiresIn > 0 {
		body["ExpiresInSeconds"] = int(spec.ExpiresIn.Seconds())
	}
	res := c.Do(Request{Method: http.MethodPost, Path: "/organizations/" + org.String() + "/runner-registration-tokens", Body: body})
	require.Equal(e.T, http.StatusCreated, res.Status, string(res.Raw))
	var data struct {
		Token             string
		RegistrationToken struct{ Id string }
	}
	res.Decode(e.T, &data)
	return data.Token, data.RegistrationToken.Id
}

func (e *Env) RegisterRunner(token string) (*RunnerIdentity, *runnerv1.RegisterResponse) {
	e.T.Helper()
	id := NewRunnerIdentity(e.T)
	res, err := e.RunnerClient(RunnerClientOptions{}).Registration.Register(context.Background(), connect.NewRequest(id.RegisterRequest(token)))
	require.NoError(e.T, err)
	return id, res.Msg
}

func (e *Env) SeedRegistrationToken(org, creator uuid.UUID) uuid.UUID {
	e.T.Helper()
	hash := make([]byte, 32)
	_, err := rand.Read(hash)
	require.NoError(e.T, err)
	now := e.Clock.Now()
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "RunnerRegistrationTokens" ("OrganizationId", "CreatedByUserId", "TokenHash", "ExpiresAt", "CreatedAt", "UpdatedAt")
		 VALUES ($1, $2, $3, $4, $5, $5) RETURNING "Id"`, org, creator, hash, now.Add(time.Hour), now).Scan(&id))
	return id
}

func (e *Env) SeedRunner(org, creator uuid.UUID) uuid.UUID {
	e.T.Helper()
	token := e.SeedRegistrationToken(org, creator)
	ident := NewRunnerIdentity(e.T)
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "Runners" ("OrganizationId", "RegistrationTokenId", "Name", "Os", "Arch", "Version", "IsolationLevels",
		 "Capacity", "SigningPublicKey", "EncryptionPublicKey") VALUES ($1, $2, 'seeded', 'linux', 'amd64', '0.1.0',
		 ARRAY['container'], 1, $3, $4) RETURNING "Id"`, org, token, ident.SigningPublicKey(), ident.EncryptionPublicKey()).Scan(&id))
	return id
}
