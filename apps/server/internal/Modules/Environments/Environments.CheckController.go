package environments

import (
	"context"

	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

const (
	messageCheckLoaded  = "targets.check.loaded"
	messageCheckStarted = "targets.check.started"
)

func hostCheckView(h models.HostCheck) gen.DeployTargetHostCheck {
	out := gen.DeployTargetHostCheck{
		HostId: h.HostID, Position: h.Position, Host: h.Host, Port: h.Port, Checked: h.Checked, Locked: h.Locked, Repaired: h.Repaired,
		FreeBytes: h.FreeBytes, CheckedAt: h.CheckedAt,
	}
	if !h.Checked {
		return out
	}
	reason := gen.DeployTargetHostCheckReason(h.Reason)
	out.Reason = &reason
	out.Message = optionalText(h.Message)
	out.CurrentReleaseId = optionalText(h.CurrentReleaseID)
	out.VerifiedReleaseId = optionalText(h.VerifiedReleaseID)
	return out
}

func (c *Controller) GetDeployTargetCheck(ctx context.Context, req gen.GetDeployTargetCheckRequestObject) (gen.GetDeployTargetCheckResponseObject, error) {
	res, err := c.service.LatestTargetCheck(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName)
	if err != nil {
		return nil, err
	}
	hosts := make([]gen.DeployTargetHostCheck, 0, len(res.Hosts))
	for _, h := range res.Hosts {
		hosts = append(hosts, hostCheckView(h))
	}
	return gen.GetDeployTargetCheck200JSONResponse{Success: true, Message: ptrOf(messageCheckLoaded), Data: &gen.DeployTargetCheck{Hosts: hosts}}, nil
}

func (c *Controller) StartDeployTargetCheck(ctx context.Context, req gen.StartDeployTargetCheckRequestObject) (gen.StartDeployTargetCheckResponseObject, error) {
	started, err := c.service.RequestTargetCheck(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName)
	if err != nil {
		return nil, err
	}
	return gen.StartDeployTargetCheck202JSONResponse{
		Success: true, Message: ptrOf(messageCheckStarted), Data: &gen.DeployTargetCheckStarted{RunId: started.RunID, RunNumber: started.RunNumber},
	}, nil
}
