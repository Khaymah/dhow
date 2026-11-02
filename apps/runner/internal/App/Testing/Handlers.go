//go:build dhowtest

package runnertest

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type handlers struct{ s *Server }

func (h handlers) Register(_ context.Context, req *connect.Request[runnerv1.RegisterRequest]) (*connect.Response[runnerv1.RegisterResponse], error) {
	m := req.Msg
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.GetToken() != s.Token || s.runner != nil {
		return nil, rpcError(connect.CodeUnauthenticated, "INVALID_REGISTRATION_TOKEN")
	}
	msg := runnersig.RegisterMessage(Origin, m.GetSigningPublicKey(), m.GetEncryptionPublicKey())
	if !runnersig.Verify(m.GetSigningPublicKey(), msg, m.GetProof()) {
		return nil, rpcError(connect.CodeInvalidArgument, "VALIDATION_FAILED: Proof")
	}
	registered, err := runnersig.FeatureNames(m.GetFeatures())
	if err != nil {
		return nil, rpcError(connect.CodeInvalidArgument, "VALIDATION_FAILED: Features")
	}
	s.runner = &runnerRecord{
		id: uuid.NewString(), org: s.OrgID, signing: m.GetSigningPublicKey(), encryption: m.GetEncryptionPublicKey(), keyVersion: 1,
		registered: registered,
	}
	return connect.NewResponse(&runnerv1.RegisterResponse{
		RunnerId: s.runner.id, OrganizationId: s.OrgID, Labels: []string{"dhow.os=" + m.GetOs()},
		ServerSigningPublicKey: s.SigningKey(), ServerKeyVersion: 1,
	}), nil
}

func (h handlers) Challenge(_ context.Context, req *connect.Request[runnerv1.ChallengeRequest]) (*connect.Response[runnerv1.ChallengeResponse], error) {
	nonce := make([]byte, runnersig.NonceSize)
	_, _ = rand.Read(nonce)
	raw, err := proto.Marshal(&runnerv1.AuthChallenge{
		Nonce: nonce, RunnerId: req.Msg.GetRunner().GetRunnerId(), ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ChallengeResponse{Challenge: &runnerv1.IssuedChallenge{ChallengeBytes: raw, Mac: []byte("mac")}}), nil
}

func (h handlers) Authenticate(_ context.Context, req *connect.Request[runnerv1.AuthenticateRequest]) (*connect.Response[runnerv1.AuthenticateResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := req.Msg.GetChallenge().GetChallengeBytes()
	if s.runner == nil || req.Msg.GetRunner().GetRunnerId() != s.runner.id || s.nonces[string(raw)] {
		return nil, rpcError(connect.CodeUnauthenticated, "INVALID_RUNNER_CREDENTIALS")
	}
	names, err := runnersig.FeatureNames(req.Msg.GetFeatures())
	if err != nil {
		return nil, rpcError(connect.CodeUnauthenticated, "INVALID_RUNNER_CREDENTIALS")
	}
	message := runnersig.AuthMessage(Origin, 1, raw)
	if len(names) > 0 {
		message = runnersig.AuthFeaturesMessage(Origin, 1, raw, names)
	}
	if !ed25519.Verify(s.runner.signing, message, req.Msg.GetSignature()) {
		return nil, rpcError(connect.CodeUnauthenticated, "INVALID_RUNNER_CREDENTIALS")
	}
	s.runner.features = names
	if s.runner.revoked {
		return nil, rpcError(connect.CodePermissionDenied, "RUNNER_REVOKED")
	}
	s.nonces[string(raw)] = true
	token := make([]byte, 32)
	_, _ = rand.Read(token)
	access := config.AccessTokenPrefix + base64.RawURLEncoding.EncodeToString(token)
	s.sessions[access] = time.Now().Add(sessionTTL)
	return connect.NewResponse(&runnerv1.AuthenticateResponse{AccessToken: access, ExpiresInSeconds: int32(sessionTTL / time.Second)}), nil
}

func (h handlers) RotateKeys(_ context.Context, req *connect.Request[runnerv1.RotateKeysRequest]) (*connect.Response[runnerv1.RotateKeysResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.sessions[strings.TrimPrefix(req.Header().Get("Authorization"), "Bearer ")]
	if !ok || time.Now().After(expiry) || s.runner == nil {
		return nil, rpcError(connect.CodeUnauthenticated, "UNAUTHENTICATED")
	}
	m := req.Msg
	newSigning := ed25519.PublicKey(m.GetNewSigningPublicKey())
	if len(newSigning) != ed25519.PublicKeySize || bytes.Equal(newSigning, s.runner.signing) {
		return nil, rpcError(connect.CodeInvalidArgument, "VALIDATION_FAILED: NewSigningPublicKey")
	}
	msg := runnersig.RotateMessage(Origin, s.runner.id, s.runner.keyVersion, newSigning, m.GetNewEncryptionPublicKey())
	if !ed25519.Verify(s.runner.signing, msg, m.GetOldKeySignature()) || !ed25519.Verify(newSigning, msg, m.GetNewKeySignature()) {
		return nil, rpcError(connect.CodeUnauthenticated, "INVALID_RUNNER_CREDENTIALS")
	}
	s.runner.signing, s.runner.encryption = newSigning, m.GetNewEncryptionPublicKey()
	s.runner.keyVersion++
	s.sessions = map[string]time.Time{}
	if s.knobs.RotateLoseReply {
		return nil, rpcError(connect.CodeUnavailable, "UNAVAILABLE")
	}
	return connect.NewResponse(&runnerv1.RotateKeysResponse{KeyVersion: s.runner.keyVersion}), nil
}

func (h handlers) Revoke(_ context.Context, req *connect.Request[runnerv1.RevokeRequest]) (*connect.Response[runnerv1.RevokeResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.sessions[strings.TrimPrefix(req.Header().Get("Authorization"), "Bearer ")]
	if !ok || time.Now().After(expiry) || s.runner == nil {
		return nil, rpcError(connect.CodeUnauthenticated, "UNAUTHENTICATED")
	}
	if !ed25519.Verify(s.runner.signing, runnersig.RevokeMessage(Origin, s.runner.id, s.runner.keyVersion), req.Msg.GetSignature()) {
		return nil, rpcError(connect.CodeUnauthenticated, "INVALID_RUNNER_CREDENTIALS")
	}
	s.runner.revoked = true
	s.sessions = map[string]time.Time{}
	return connect.NewResponse(&runnerv1.RevokeResponse{}), nil
}

func (h handlers) Lease(ctx context.Context, req *connect.Request[runnerv1.LeaseRequest]) (*connect.Response[runnerv1.LeaseResponse], error) {
	deadline := time.Now().Add(min(time.Duration(req.Msg.GetWaitSeconds())*time.Second, 2*time.Second))
	for {
		if signed, err := h.s.claim(); signed != nil || err != nil {
			h.s.mu.Lock()
			delay := h.s.knobs.GrantDelay
			h.s.mu.Unlock()
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return connect.NewResponse(&runnerv1.LeaseResponse{Grant: signed}), err
		}
		if time.Now().After(deadline) {
			return connect.NewResponse(&runnerv1.LeaseResponse{}), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollStep):
		}
	}
}

func (s *Server) claim() (*runnerv1.SignedLeaseGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.replays) > 0 {
		replay := s.replays[0]
		s.replays = s.replays[1:]
		return replay, nil
	}
	if len(s.queue) == 0 || s.runner == nil {
		return nil, nil
	}
	j := s.queue[0]
	s.queue = s.queue[1:]
	s.fencing++
	l := &Lease{
		ID: uuid.NewString(), AttemptID: uuid.NewString(), JobID: j.ID, Fencing: s.fencing, TTL: s.knobs.LeaseTTL,
		ExpiresAt: time.Now().Add(s.knobs.LeaseTTL), Steps: map[int32]string{}, Job: j,
	}
	grant, err := proto.Marshal(&runnerv1.LeaseGrant{
		LeaseId: l.ID, AttemptId: l.AttemptID, JobId: j.ID, OrganizationId: s.runner.org, RunnerId: s.runner.id,
		FencingToken: l.Fencing, LeaseTtlSeconds: ttlSeconds(l.TTL), IssuedAt: timestamppb.Now(), Spec: j.Spec,
	})
	if err != nil {
		return nil, err
	}
	key := s.signing
	if s.knobs.WrongKey {
		key = s.wrongKey
	}
	l.GrantBytes = grant
	l.Signed = &runnerv1.SignedLeaseGrant{GrantBytes: grant, Signature: ed25519.Sign(key, runnersig.GrantMessage(grant)), KeyVersion: 1}
	s.leases[l.ID] = l
	s.order = append(s.order, l)
	return l.Signed, nil
}

func ttlSeconds(d time.Duration) int32 { return int32(min(d/time.Second, time.Hour/time.Second)) } //nolint:gosec // Capped at an hour.

func (s *Server) active(ref *runnerv1.LeaseRef) (*Lease, error) {
	l := s.leases[ref.GetLeaseId()]
	if l == nil || l.Fencing != ref.GetFencingToken() || l.Closed || time.Now().After(l.ExpiresAt) {
		return nil, rpcError(connect.CodeFailedPrecondition, "LEASE_LOST")
	}
	return l, nil
}

func (s *Server) control(l *Lease) *runnerv1.LeaseControl {
	return &runnerv1.LeaseControl{CancelRequested: l.Cancel, LeaseTtlSeconds: ttlSeconds(l.TTL)}
}

func (s *Server) pause(ctx context.Context, pick func(Knobs) time.Duration) error {
	s.mu.Lock()
	d := pick(s.knobs)
	s.mu.Unlock()
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h handlers) Start(ctx context.Context, req *connect.Request[runnerv1.StartRequest]) (*connect.Response[runnerv1.StartResponse], error) {
	s := h.s
	if err := s.pause(ctx, func(k Knobs) time.Duration { return k.StartDelay }); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.active(req.Msg.GetLease())
	if err != nil {
		return nil, err
	}
	if l.Started {
		return nil, rpcError(connect.CodeFailedPrecondition, "ATTEMPT_ALREADY_STARTED")
	}
	if l.Cancel {
		return nil, rpcError(connect.CodeFailedPrecondition, "CANCEL_REQUESTED")
	}
	grant := runnersig.GrantDigest(l.GrantBytes)
	if !ed25519.Verify(s.runner.signing, runnersig.StartMessage(l.ID, l.Fencing, grant), req.Msg.GetPopSignature()) {
		return nil, rpcError(connect.CodeUnauthenticated, "INVALID_RUNNER_CREDENTIALS")
	}
	l.Started = true
	l.ExpiresAt = time.Now().Add(l.TTL)
	res := &runnerv1.StartResponse{Control: s.control(l)}
	if len(l.Job.Secrets) > 0 {
		sealed, err := s.seal(l)
		if err != nil {
			return nil, err
		}
		res.Secrets = sealed
	}
	return connect.NewResponse(res), nil
}

func (s *Server) seal(l *Lease) (*runnerv1.SealedSecrets, error) {
	bundle := &runnerv1.SecretBundle{}
	for name, v := range l.Job.Secrets {
		bundle.Secrets = append(bundle.Secrets, &runnerv1.SecretValue{Name: name, Value: v})
	}
	plain, err := proto.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	enc, ct, err := runnersig.SealSecrets(s.runner.encryption, runnersig.SecretsAAD(l.ID, l.Fencing, l.AttemptID, s.runner.id), plain)
	if err != nil {
		return nil, err
	}
	return &runnerv1.SealedSecrets{Kem: runnerv1.HpkeKem_HPKE_KEM_MLKEM768_X25519, Enc: enc, Ciphertext: ct}, nil
}

func (h handlers) Heartbeat(_ context.Context, req *connect.Request[runnerv1.HeartbeatRequest]) (*connect.Response[runnerv1.HeartbeatResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.knobs.HeartbeatFail {
		return nil, rpcError(connect.CodeUnavailable, "UNAVAILABLE")
	}
	l, err := s.active(req.Msg.GetLease())
	if err != nil {
		return nil, err
	}
	l.ExpiresAt = time.Now().Add(l.TTL)
	return connect.NewResponse(&runnerv1.HeartbeatResponse{Control: s.control(l)}), nil
}

func (h handlers) BeginStep(_ context.Context, req *connect.Request[runnerv1.BeginStepRequest]) (*connect.Response[runnerv1.BeginStepResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.active(req.Msg.GetLease())
	if err != nil {
		return nil, err
	}
	if l.Cancel {
		return nil, rpcError(connect.CodeFailedPrecondition, "CANCEL_REQUESTED")
	}
	l.Steps[req.Msg.GetIndex()] = "running"
	return connect.NewResponse(&runnerv1.BeginStepResponse{Control: s.control(l)}), nil
}

func (h handlers) EndStep(_ context.Context, req *connect.Request[runnerv1.EndStepRequest]) (*connect.Response[runnerv1.EndStepResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.knobs.EndStepFail {
		return nil, rpcError(connect.CodeUnavailable, "UNAVAILABLE")
	}
	l, err := s.active(req.Msg.GetLease())
	if err != nil {
		return nil, err
	}
	l.Steps[req.Msg.GetIndex()] = req.Msg.GetFailureReason().String()
	return connect.NewResponse(&runnerv1.EndStepResponse{Control: s.control(l)}), nil
}

func (h handlers) Complete(_ context.Context, req *connect.Request[runnerv1.CompleteRequest]) (*connect.Response[runnerv1.CompleteResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	m := req.Msg
	if s.knobs.CompleteUnavailable > 0 {
		s.knobs.CompleteUnavailable--
		return nil, rpcError(connect.CodeUnavailable, "UNAVAILABLE")
	}
	if s.knobs.CompleteFailLease != "" && s.knobs.CompleteFailLease == m.GetLease().GetLeaseId() {
		return nil, rpcError(connect.CodeUnavailable, "UNAVAILABLE")
	}
	l := s.leases[m.GetLease().GetLeaseId()]
	if l == nil || l.Fencing != m.GetLease().GetFencingToken() {
		return nil, rpcError(connect.CodeFailedPrecondition, "LEASE_LOST")
	}
	want := runnersig.CompletionDigest(l.AttemptID, l.Fencing, int32(m.GetResult()), m.GetExitCode(), int32(m.GetFailureReason()))
	if !bytes.Equal(want, m.GetDigest()) {
		return nil, rpcError(connect.CodeInvalidArgument, "VALIDATION_FAILED: Digest")
	}
	l.Completes = append(l.Completes, proto.Clone(m).(*runnerv1.CompleteRequest))
	if l.Closed || time.Now().After(l.ExpiresAt) {
		return nil, rpcError(connect.CodeFailedPrecondition, "LEASE_LOST")
	}
	l.Closed = true
	return connect.NewResponse(&runnerv1.CompleteResponse{}), nil
}

func (h handlers) Append(_ context.Context, req *connect.Request[runnerv1.AppendRequest]) (*connect.Response[runnerv1.AppendResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.knobs.AppendUnimplemented {
		return nil, rpcError(connect.CodeUnimplemented, "UNIMPLEMENTED")
	}
	if s.knobs.AppendUnavailable > 0 {
		s.knobs.AppendUnavailable--
		return nil, rpcError(connect.CodeUnavailable, "LOG_STORAGE_UNAVAILABLE")
	}
	l, err := s.active(req.Msg.GetLease())
	if err != nil {
		return nil, err
	}
	for _, c := range req.Msg.GetChunks() {
		if c.GetSeq() < 1 || c.GetStream() == runnerv1.LogStream_LOG_STREAM_UNSPECIFIED || len(c.GetData()) == 0 ||
			len(c.GetData()) > config.LogChunkBytes {
			return nil, rpcError(connect.CodeInvalidArgument, "VALIDATION_FAILED: Chunks")
		}
	}
	res := &runnerv1.AppendResponse{Control: s.control(l)}
	for _, c := range ordered(req.Msg.GetChunks()) {
		if c.GetSeq() <= l.LastSeq {
			continue
		}
		if c.GetSeq() != l.LastSeq+1 {
			next := l.LastSeq + 1
			res.ExpectedSeq = &next
			break
		}
		l.LastSeq = c.GetSeq()
		if l.Truncated {
			continue
		}
		data := c.GetData()
		if s.knobs.LogCapBytes > 0 && l.LogBytes+int64(len(data)) > s.knobs.LogCapBytes {
			data = data[:s.knobs.LogCapBytes-l.LogBytes]
			l.Truncated = true
		}
		if len(data) > 0 {
			stored := proto.Clone(c).(*runnerv1.LogChunk)
			stored.Data = data
			l.Chunks = append(l.Chunks, stored)
			l.LogBytes += int64(len(data))
		}
		if l.Truncated {
			l.LastSeq++
			l.Chunks = append(l.Chunks, &runnerv1.LogChunk{
				Seq: l.LastSeq, Stream: runnerv1.LogStream_LOG_STREAM_SYSTEM, Data: []byte("dhow: LOG_TRUNCATED\n"),
			})
		}
	}
	res.LastSeq = l.LastSeq
	res.LogTruncated = l.Truncated
	return connect.NewResponse(res), nil
}

func ordered(chunks []*runnerv1.LogChunk) []*runnerv1.LogChunk {
	out := append([]*runnerv1.LogChunk(nil), chunks...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].GetSeq() > out[j].GetSeq(); j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
