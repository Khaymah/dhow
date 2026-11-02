package jobs

import (
	"context"

	"connectrpc.com/connect"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

type connectHandler struct {
	service *Service
}

var _ runnerv1connect.LeaseServiceHandler = connectHandler{}

func (h connectHandler) Lease(ctx context.Context, req *connect.Request[runnerv1.LeaseRequest]) (*connect.Response[runnerv1.LeaseResponse], error) {
	grant, err := h.service.Lease(ctx, req.Msg.GetFreeSlots(), req.Msg.GetWaitSeconds())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.LeaseResponse{Grant: grant}), nil
}

func (h connectHandler) Start(ctx context.Context, req *connect.Request[runnerv1.StartRequest]) (*connect.Response[runnerv1.StartResponse], error) {
	secrets, ctrl, err := h.service.Start(ctx, req.Msg.GetLease(), req.Msg.GetPopSignature())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.StartResponse{Secrets: secrets, Control: ctrl}), nil
}

func (h connectHandler) Heartbeat(ctx context.Context, req *connect.Request[runnerv1.HeartbeatRequest]) (*connect.Response[runnerv1.HeartbeatResponse], error) {
	ctrl, err := h.service.Heartbeat(ctx, req.Msg.GetLease())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.HeartbeatResponse{Control: ctrl}), nil
}

func (h connectHandler) BeginStep(ctx context.Context, req *connect.Request[runnerv1.BeginStepRequest]) (*connect.Response[runnerv1.BeginStepResponse], error) {
	ctrl, err := h.service.BeginStep(ctx, req.Msg.GetLease(), req.Msg.GetIndex(), req.Msg.GetMutating())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.BeginStepResponse{Control: ctrl}), nil
}

func (h connectHandler) EndStep(ctx context.Context, req *connect.Request[runnerv1.EndStepRequest]) (*connect.Response[runnerv1.EndStepResponse], error) {
	m := req.Msg
	ctrl, err := h.service.EndStep(ctx, m.GetLease(), m.GetIndex(), m.GetExitCode(), m.GetFailureReason())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.EndStepResponse{Control: ctrl}), nil
}

func (h connectHandler) Complete(ctx context.Context, req *connect.Request[runnerv1.CompleteRequest]) (*connect.Response[runnerv1.CompleteResponse], error) {
	m := req.Msg
	err := h.service.Complete(ctx, CompleteInput{
		Lease: m.GetLease(), Result: m.GetResult(), ExitCode: m.GetExitCode(), Reason: m.GetFailureReason(), Digest: m.GetDigest(),
		Outputs: m.GetOutputs(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.CompleteResponse{}), nil
}

type logHandler struct {
	service *Service
}

var _ runnerv1connect.LogServiceHandler = logHandler{}

func (h logHandler) Append(ctx context.Context, req *connect.Request[runnerv1.AppendRequest]) (*connect.Response[runnerv1.AppendResponse], error) {
	res, err := h.service.Append(ctx, req.Msg.GetLease(), req.Msg.GetChunks())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(res), nil
}
