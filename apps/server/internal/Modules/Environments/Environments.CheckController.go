package environments

import (
	"context"

	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

const (
	messageCheckLoaded  = "targets.check.loaded"
	messageCheckStarted = "targets.check.started"

	messageRollbackStarted = "targets.rollback.started"
	messageReleasesLoaded  = "targets.releases.loaded"
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

func (c *Controller) RollbackDeployTarget(ctx context.Context, req gen.RollbackDeployTargetRequestObject) (gen.RollbackDeployTargetResponseObject, error) {
	started, err := c.service.RequestTargetRollback(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, req.Body.ReleaseId, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	return gen.RollbackDeployTarget202JSONResponse{
		Success: true, Message: ptrOf(messageRollbackStarted), Data: &gen.DeployTargetCheckStarted{RunId: started.RunID, RunNumber: started.RunNumber},
	}, nil
}

func releaseHostView(h models.ReleaseHost) gen.DeployTargetReleaseHost {
	return gen.DeployTargetReleaseHost{
		HostId: h.HostID, Position: h.Position, State: gen.DeployTargetReleaseHostState(h.State), RollbackReason: optionalText(h.RollbackReason),
		Message: optionalText(h.Message),
	}
}

func targetReleaseView(r models.TargetRelease) gen.DeployTargetRelease {
	hosts := make([]gen.DeployTargetReleaseHost, 0, len(r.Hosts))
	for _, h := range r.Hosts {
		hosts = append(hosts, releaseHostView(h))
	}
	return gen.DeployTargetRelease{
		Id: r.ID, ReleaseId: r.ReleaseID, Kind: gen.DeployTargetReleaseKind(r.Kind), State: gen.DeployTargetReleaseState(r.State),
		RestoredReleaseId: optionalText(r.RestoredReleaseID), PreviousReleaseId: optionalText(r.PreviousReleaseID),
		RollbackReason: optionalText(r.RollbackReason), Message: optionalText(r.Message), StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		RollbackTarget: r.RollbackTarget, ArtifactKept: r.ArtifactKept, Hosts: hosts,
	}
}

func (c *Controller) ListDeployTargetReleases(ctx context.Context, req gen.ListDeployTargetReleasesRequestObject) (gen.ListDeployTargetReleasesResponseObject, error) {
	res, err := c.service.ListTargetReleases(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName)
	if err != nil {
		return nil, err
	}
	releases := make([]gen.DeployTargetRelease, 0, len(res.Releases))
	for _, r := range res.Releases {
		releases = append(releases, targetReleaseView(r))
	}
	return gen.ListDeployTargetReleases200JSONResponse{
		Success: true, Message: ptrOf(messageReleasesLoaded),
		Data: &gen.DeployTargetReleases{CurrentReleaseId: optionalText(res.CurrentReleaseID), Partial: res.Partial, Releases: releases},
	}, nil
}
