package runners

import (
	"context"

	"connectrpc.com/connect"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners/Models"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

type connectHandler struct {
	service *Service
}

var (
	_ runnerv1connect.RegistrationServiceHandler = connectHandler{}
	_ runnerv1connect.AuthServiceHandler         = connectHandler{}
)

func isolationName(v runnerv1.Isolation) string {
	switch v {
	case runnerv1.Isolation_ISOLATION_CONTAINER:
		return models.IsolationContainer
	case runnerv1.Isolation_ISOLATION_GVISOR:
		return models.IsolationGvisor
	default:
		return ""
	}
}

func (h connectHandler) Register(ctx context.Context, req *connect.Request[runnerv1.RegisterRequest]) (*connect.Response[runnerv1.RegisterResponse], error) {
	m := req.Msg
	levels := make([]string, 0, len(m.GetIsolationLevels()))
	for _, l := range m.GetIsolationLevels() {
		levels = append(levels, isolationName(l))
	}
	res, err := h.service.Register(ctx, models.RegisterInput{
		Token: m.GetToken(), Name: m.GetName(), Os: m.GetOs(), Arch: m.GetArch(), Version: m.GetVersion(),
		IsolationLevels: levels, Capacity: int(m.GetCapacity()), SigningPublicKey: m.GetSigningPublicKey(),
		EncryptionPublicKey: m.GetEncryptionPublicKey(), Proof: m.GetProof(), Features: m.GetFeatures(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.RegisterResponse{
		RunnerId: res.RunnerID.String(), OrganizationId: res.OrganizationID.String(), Labels: res.Labels,
		ServerSigningPublicKey: res.ServerSigningKey, ServerKeyVersion: res.ServerKeyVersion,
	}), nil
}

func (h connectHandler) Challenge(ctx context.Context, req *connect.Request[runnerv1.ChallengeRequest]) (*connect.Response[runnerv1.ChallengeResponse], error) {
	challenge, mac, err := h.service.Challenge(ctx, req.Msg.GetRunner().GetRunnerId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ChallengeResponse{
		Challenge: &runnerv1.IssuedChallenge{ChallengeBytes: challenge, Mac: mac},
	}), nil
}

func (h connectHandler) Authenticate(ctx context.Context, req *connect.Request[runnerv1.AuthenticateRequest]) (*connect.Response[runnerv1.AuthenticateResponse], error) {
	m := req.Msg
	token, err := h.service.Authenticate(ctx, models.AuthenticateInput{
		RunnerID: m.GetRunner().GetRunnerId(), ChallengeBytes: m.GetChallenge().GetChallengeBytes(),
		ChallengeMAC: m.GetChallenge().GetMac(), Signature: m.GetSignature(), Features: m.GetFeatures(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.AuthenticateResponse{
		AccessToken: token, ExpiresInSeconds: int32(config.RunnerAccessTokenTTL.Seconds()),
	}), nil
}

func (h connectHandler) RotateKeys(ctx context.Context, req *connect.Request[runnerv1.RotateKeysRequest]) (*connect.Response[runnerv1.RotateKeysResponse], error) {
	m := req.Msg
	version, err := h.service.RotateKeys(ctx, models.RotateInput{
		NewSigningPublicKey: m.GetNewSigningPublicKey(), NewEncryptionPublicKey: m.GetNewEncryptionPublicKey(),
		OldKeySignature: m.GetOldKeySignature(), NewKeySignature: m.GetNewKeySignature(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.RotateKeysResponse{KeyVersion: version}), nil
}

func (h connectHandler) Revoke(ctx context.Context, req *connect.Request[runnerv1.RevokeRequest]) (*connect.Response[runnerv1.RevokeResponse], error) {
	if err := h.service.RevokeSelf(ctx, req.Msg.GetSignature()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.RevokeResponse{}), nil
}
