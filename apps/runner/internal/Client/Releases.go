package client

import (
	"context"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (c *Client) ReportRelease(ctx context.Context, ref *runnerv1.LeaseRef, step int32, events []*runnerv1.DeployEvent) (int32, error) {
	res, err := call(ctx, c, config.RPCTimeout, c.releases.Report, &runnerv1.ReleaseServiceReportRequest{Lease: ref, StepIndex: step, Events: events})
	if err != nil {
		return 0, err
	}
	return res.GetAccepted(), nil
}
