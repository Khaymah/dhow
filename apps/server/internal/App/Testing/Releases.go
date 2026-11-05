package apptest

import (
	"context"

	"connectrpc.com/connect"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
	"github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1/runnerv1connect"
)

func (r *TestRunner) Releases() runnerv1connect.ReleaseServiceClient {
	return runnerv1connect.NewReleaseServiceClient(r.Client.HTTP, RunnerBaseURL)
}

func (r *TestRunner) ReportRelease(ref *runnerv1.LeaseRef, step int32, events ...*runnerv1.DeployEvent) error {
	_, err := r.Releases().Report(context.Background(), connect.NewRequest(&runnerv1.ReleaseServiceReportRequest{
		Lease: ref, StepIndex: step, Events: events,
	}))
	return err
}
