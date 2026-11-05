package environments

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

const (
	CodeReleaseLeaseLost  = "LEASE_LOST"
	CodeReleaseInvalid    = "VALIDATION_FAILED"
	CodeNotTargetStep     = "NOT_A_TARGET_STEP"
	CodeLeaseMismatch     = "RELEASE_LEASE_MISMATCH"
	CodeUnknownHost       = "RELEASE_UNKNOWN_HOST"
	CodeIllegalTransition = "RELEASE_ILLEGAL_TRANSITION"
	CodeReleaseFinished   = "RELEASE_FINISHED"
	CodeReleaseNotStarted = "RELEASE_NOT_STARTED"
	CodeReleaseConflict   = "RELEASE_CONFLICT"
	CodeReleaseUnauth     = "UNAUTHENTICATED"
)

type rejection struct {
	rpc  connect.Code
	code string
}

func (r *rejection) Error() string { return r.code }

func reject(rpc connect.Code, code string) *rejection { return &rejection{rpc: rpc, code: code} }

func errInvalidReport(field string) *rejection {
	return reject(connect.CodeInvalidArgument, CodeReleaseInvalid+": "+field)
}

func rpcError(err error) error {
	var r *rejection
	if errors.As(err, &r) {
		return connect.NewError(r.rpc, errors.New(r.code))
	}
	return err
}

type releaseHandler struct{ service *Service }

var _ runnerv1connect.ReleaseServiceHandler = releaseHandler{}

func (h releaseHandler) Report(ctx context.Context, req *connect.Request[runnerv1.ReleaseServiceReportRequest]) (*connect.Response[runnerv1.ReleaseServiceReportResponse], error) {
	m := req.Msg
	n, err := h.service.ReportRelease(ctx, m.GetLease(), m.GetStepIndex(), m.GetEvents())
	if err != nil {
		return nil, rpcError(err)
	}
	return connect.NewResponse(&runnerv1.ReleaseServiceReportResponse{Accepted: n}), nil
}

type ConnectDeps struct {
	Service *Service
	Gate    *runnerauth.Gate
	Log     *slog.Logger
}

func Procedures() []policy.Rule {
	return []policy.Rule{policy.Runner(runnerv1connect.ReleaseServiceReportProcedure, policy.RunnerSession)}
}

func ConnectRoutes(mux *http.ServeMux, d ConnectDeps) {
	table := policy.NewTable(Procedures()...)
	options := []connect.HandlerOption{
		d.Gate.RequestGate(table),
		connect.WithReadMaxBytes(config.RunnerReadMaxBytes),
		connect.WithInterceptors(runnerauth.Interceptor(d.Log)),
	}
	mux.Handle(runnerv1connect.NewReleaseServiceHandler(releaseHandler{service: d.Service}, options...))
}
