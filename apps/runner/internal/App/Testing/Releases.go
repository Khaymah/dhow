//go:build dhowtest

package runnertest

import (
	"context"

	"connectrpc.com/connect"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type ReportCall struct {
	LeaseID string
	Fencing int64
	Step    int32
	Events  []*runnerv1.DeployEvent
}

type releaseState struct {
	calls []ReportCall
	fail  int
}

type releaseHandlers struct{ s *Server }

func (h releaseHandlers) Report(_ context.Context, req *connect.Request[runnerv1.ReleaseServiceReportRequest]) (*connect.Response[runnerv1.ReleaseServiceReportResponse], error) {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.active(req.Msg.GetLease()); err != nil {
		return nil, err
	}
	if s.releases.fail > 0 {
		s.releases.fail--
		return nil, rpcError(connect.CodeUnavailable, "UNAVAILABLE")
	}
	s.releases.calls = append(s.releases.calls, ReportCall{
		LeaseID: req.Msg.GetLease().GetLeaseId(), Fencing: req.Msg.GetLease().GetFencingToken(), Step: req.Msg.GetStepIndex(),
		Events: req.Msg.GetEvents(),
	})
	return connect.NewResponse(&runnerv1.ReleaseServiceReportResponse{Accepted: int32(len(req.Msg.GetEvents()))}), nil //nolint:gosec // A test batch is small.
}

func (s *Server) ReportCalls() []ReportCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ReportCall(nil), s.releases.calls...)
}

func (s *Server) ReportedEvents() []*runnerv1.DeployEvent {
	var out []*runnerv1.DeployEvent
	for _, c := range s.ReportCalls() {
		out = append(out, c.Events...)
	}
	return out
}

func (s *Server) FailReports(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases.fail = n
}
