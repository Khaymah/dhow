//go:build dhowtest

package runnertest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

const (
	Origin          = "https://dhow.test"
	DefaultLeaseTTL = 45 * time.Second
	sessionTTL      = 15 * time.Minute
	pollStep        = 20 * time.Millisecond
)

type Job struct {
	ID      string
	Spec    *runnerv1.JobSpec
	Secrets map[string][]byte
}

type Lease struct {
	ID, AttemptID, JobID string
	Fencing              int64
	TTL                  time.Duration
	ExpiresAt            time.Time
	Started              bool
	Closed               bool
	Cancel               bool
	Steps                map[int32]string
	Completes            []*runnerv1.CompleteRequest
	Chunks               []*runnerv1.LogChunk
	LastSeq              int64
	LogBytes             int64
	Truncated            bool
	Job                  *Job
	GrantBytes           []byte
	Signed               *runnerv1.SignedLeaseGrant
}

type Knobs struct {
	WrongKey            bool
	LeaseTTL            time.Duration
	LogCapBytes         int64
	AppendUnavailable   int
	AppendUnimplemented bool
	CompleteUnavailable int
	CompleteFailLease   string
	GrantDelay          time.Duration
	StartDelay          time.Duration
	HeartbeatFail       bool
	EndStepFail         bool
	RotateLoseReply     bool
}

type runnerRecord struct {
	id, org    string
	signing    ed25519.PublicKey
	encryption []byte
	keyVersion int32
	revoked    bool
	features   []string
	registered []string
}

type Server struct {
	T        testing.TB
	HTTP     *httptest.Server
	URL      string
	CAFile   string
	Token    string
	OrgID    string
	signing  ed25519.PrivateKey
	wrongKey ed25519.PrivateKey

	mu       sync.Mutex
	knobs    Knobs
	runner   *runnerRecord
	sessions map[string]time.Time
	nonces   map[string]bool
	queue    []*Job
	leases   map[string]*Lease
	order    []*Lease
	fencing  int64
	calls    map[string]int
	replays  []*runnerv1.SignedLeaseGrant
}

func NewServer(t testing.TB) *Server {
	t.Helper()
	_, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, wrong, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	s := &Server{
		T: t, Token: config.RegistrationTokenPrefix + base64.RawURLEncoding.EncodeToString(raw), OrgID: uuid.NewString(),
		signing: signing, wrongKey: wrong, sessions: map[string]time.Time{}, nonces: map[string]bool{},
		leases: map[string]*Lease{}, calls: map[string]int{}, knobs: Knobs{LeaseTTL: DefaultLeaseTTL},
	}
	mux := http.NewServeMux()
	opts := connect.WithInterceptors(s.gate())
	h := handlers{s: s}
	mux.Handle(runnerv1connect.NewRegistrationServiceHandler(h, opts))
	mux.Handle(runnerv1connect.NewAuthServiceHandler(h, opts))
	mux.Handle(runnerv1connect.NewLeaseServiceHandler(h, opts))
	mux.Handle(runnerv1connect.NewLogServiceHandler(h, opts))
	root := http.NewServeMux()
	root.Handle(config.APIPrefix+"/", http.StripPrefix(config.APIPrefix, mux))
	s.HTTP = httptest.NewUnstartedServer(root)
	s.HTTP.EnableHTTP2 = true
	s.HTTP.StartTLS()
	t.Cleanup(s.HTTP.Close)
	s.URL = s.HTTP.URL
	s.CAFile = filepath.Join(t.TempDir(), "ca.pem")
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.HTTP.Certificate().Raw})
	if err := os.WriteFile(s.CAFile, block, 0o600); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *Server) SigningKey() ed25519.PublicKey {
	pub, _ := s.signing.Public().(ed25519.PublicKey)
	return pub
}

func (s *Server) Configure(fn func(*Knobs)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.knobs)
}

func (s *Server) Enqueue(spec *runnerv1.JobSpec, secrets map[string][]byte) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := &Job{ID: uuid.NewString(), Spec: spec, Secrets: secrets}
	s.queue = append(s.queue, j)
	return j
}

func (s *Server) Calls(procedure string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[procedure]
}

func (s *Server) Leases() []Lease {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Lease, 0, len(s.order))
	for _, l := range s.order {
		c := *l
		c.Chunks = append([]*runnerv1.LogChunk(nil), l.Chunks...)
		c.Completes = append([]*runnerv1.CompleteRequest(nil), l.Completes...)
		out = append(out, c)
	}
	return out
}

func (s *Server) RequestCancel(leaseID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.leases[leaseID]; l != nil {
		l.Cancel = true
	}
}

func (s *Server) RunnerKey() (ed25519.PublicKey, []byte, int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runner == nil {
		return nil, nil, 0
	}
	return s.runner.signing, s.runner.encryption, s.runner.keyVersion
}

func (s *Server) RunnerFeatures() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runner == nil {
		return nil, nil
	}
	return append([]string(nil), s.runner.registered...), append([]string(nil), s.runner.features...)
}

func (s *Server) Revoked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runner != nil && s.runner.revoked
}

func (s *Server) RunnerID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runner == nil {
		return ""
	}
	return s.runner.id
}

func rpcError(code connect.Code, msg string) error { return connect.NewError(code, errors.New(msg)) }

func (s *Server) gate() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure
			s.mu.Lock()
			s.calls[procedure]++
			s.mu.Unlock()
			if req.Header().Get(config.ProtocolHeader) != config.ProtocolVersion {
				return nil, rpcError(connect.CodeFailedPrecondition, "RUNNER_UPGRADE_REQUIRED")
			}
			if strings.Contains(procedure, "LeaseService") || strings.Contains(procedure, "LogService") {
				token := strings.TrimPrefix(req.Header().Get("Authorization"), "Bearer ")
				s.mu.Lock()
				expiry, ok := s.sessions[token]
				s.mu.Unlock()
				if !ok || time.Now().After(expiry) {
					return nil, rpcError(connect.CodeUnauthenticated, "UNAUTHENTICATED")
				}
			}
			return next(ctx, req)
		}
	})
}

func (s *Server) Replay(leaseID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.leases[leaseID]; l != nil {
		s.replays = append(s.replays, l.Signed)
	}
}

func (s *Server) ExpireLease(leaseID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.leases[leaseID]; l != nil {
		l.ExpiresAt = time.Now().Add(-time.Second)
	}
}

func (s *Server) Lease(leaseID string) (Lease, bool) {
	for _, l := range s.Leases() {
		if l.ID == leaseID {
			return l, true
		}
	}
	return Lease{}, false
}

func (s *Server) WaitLease(t testing.TB, within time.Duration) Lease {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ls := s.Leases(); len(ls) > 0 {
			return ls[len(ls)-1]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no lease within %s", within)
	return Lease{}
}
