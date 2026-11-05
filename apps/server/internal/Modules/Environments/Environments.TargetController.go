package environments

import (
	"context"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

const (
	messageTargetsListed    = "targets.list.loaded"
	messageTargetLoaded     = "targets.target.loaded"
	messageTargetCreated    = "targets.target.created"
	messageTargetUpdated    = "targets.target.updated"
	messageTargetRemoved    = "targets.target.removed"
	messageKeyGenerated     = "targets.key.generated"
	messageHostAdded        = "targets.host.added"
	messageHostUpdated      = "targets.host.updated"
	messageHostRemoved      = "targets.host.removed"
	messageHostsReordered   = "targets.hosts.reordered"
	messageHostKeyFetched   = "targets.hostKey.fetched"
	messageHostKeyPending   = "targets.hostKey.pending"
	messageHostKeyConfirmed = "targets.hostKey.confirmed"
)

func valueOr[T any](v *T, fallback T) T {
	if v == nil {
		return fallback
	}
	return *v
}

func settingsInOf(in *gen.DeployTargetSettings) *models.TargetSettings {
	if in == nil {
		return nil
	}
	out := &models.TargetSettings{AllowRoot: valueOr(in.AllowRoot, false)}
	if in.Rolling != nil {
		out.Rolling = &models.Rolling{
			BatchSize: valueOr(in.Rolling.BatchSize, models.DefaultBatchSize), BatchPauseSeconds: valueOr(in.Rolling.BatchPauseSeconds, 0),
		}
	}
	if in.Ssh != nil {
		out.SSH = &models.SSHSettings{
			ReloadCommand: valueOr(in.Ssh.ReloadCommand, ""), PreSwitchCommand: valueOr(in.Ssh.PreSwitchCommand, ""),
			PostSwitchCommand: valueOr(in.Ssh.PostSwitchCommand, ""), Shared: valueOr(in.Ssh.Shared, []string{}),
		}
	}
	if in.Compose != nil {
		out.Compose = &models.ComposeSettings{
			ProjectName: valueOr(in.Compose.ProjectName, ""), ComposeFile: valueOr(in.Compose.ComposeFile, ""),
			AllowUnsafeCompose: valueOr(in.Compose.AllowUnsafeCompose, false),
		}
	}
	return out
}

func settingsView(s models.TargetSettings) gen.DeployTargetSettings {
	out := gen.DeployTargetSettings{AllowRoot: ptrOf(s.AllowRoot)}
	if s.Rolling != nil {
		out.Rolling = &gen.DeployTargetRolling{BatchSize: ptrOf(s.Rolling.BatchSize), BatchPauseSeconds: ptrOf(s.Rolling.BatchPauseSeconds)}
	}
	if s.SSH != nil {
		out.Ssh = &gen.DeployTargetSshSettings{
			ReloadCommand: ptrOf(s.SSH.ReloadCommand), PreSwitchCommand: ptrOf(s.SSH.PreSwitchCommand),
			PostSwitchCommand: ptrOf(s.SSH.PostSwitchCommand), Shared: ptrOf(nonNil(s.SSH.Shared)),
		}
	}
	if s.Compose != nil {
		out.Compose = &gen.DeployTargetComposeSettings{
			ProjectName: ptrOf(s.Compose.ProjectName), ComposeFile: ptrOf(s.Compose.ComposeFile),
			AllowUnsafeCompose: ptrOf(s.Compose.AllowUnsafeCompose),
		}
	}
	return out
}

func optionalText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func hostView(h models.TargetHost) gen.DeployTargetHost {
	return gen.DeployTargetHost{
		Id: h.ID, Position: h.Position, Host: h.Host, Port: h.Port, KeyPinned: h.KeyPinned,
		HostKeyFingerprint: optionalText(h.HostKeyFingerprint), HostKeyConfirmedAt: h.HostKeyConfirmedAt,
		HostKeyConfirmedByUserId: h.HostKeyConfirmedByUserID, PendingHostKeyFingerprint: optionalText(h.PendingHostKeyFingerprint),
	}
}

func targetView(t models.Target) gen.DeployTarget {
	hosts := make([]gen.DeployTargetHost, 0, len(t.Hosts))
	for _, h := range t.Hosts {
		hosts = append(hosts, hostView(h))
	}
	notReady := make([]gen.DeployTargetNotReady, 0, len(t.NotReady))
	for _, n := range t.NotReady {
		notReady = append(notReady, gen.DeployTargetNotReady(n))
	}
	return gen.DeployTarget{
		Id: t.ID, EnvironmentId: t.EnvironmentID, Name: t.Name, Kind: gen.DeployTargetKind(t.Kind), Username: t.Username,
		KeySecretName: optionalText(t.KeySecretName), PublicKey: optionalText(t.PublicKey), DeployPath: t.DeployPath,
		KeepReleases: t.KeepReleases, Settings: settingsView(t.Settings), AllowedImages: nonNil(t.AllowedImages), Hosts: hosts,
		Ready: t.Ready, NotReady: notReady, Version: t.Version, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func (c *Controller) ListDeployTargets(ctx context.Context, req gen.ListDeployTargetsRequestObject) (gen.ListDeployTargetsResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	list, err := c.service.ListTargets(ctx, req.Id, req.ProjectId, req.EnvironmentName, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.DeployTarget, 0, len(list.Items))
	for _, t := range list.Items {
		items = append(items, targetView(t))
	}
	return gen.ListDeployTargets200JSONResponse{
		Success: true, Message: ptrOf(messageTargetsListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) CreateDeployTarget(ctx context.Context, req gen.CreateDeployTargetRequestObject) (gen.CreateDeployTargetResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	hosts := make([]models.HostInput, 0, len(b.Hosts))
	for _, h := range b.Hosts {
		hosts = append(hosts, models.HostInput{Host: h.Host, Port: h.Port})
	}
	t, err := c.service.CreateTarget(ctx, req.Id, req.ProjectId, req.EnvironmentName, models.CreateTargetInput{
		Name: b.Name, Kind: string(b.Kind), Username: b.Username, KeySecretName: b.KeySecretName, DeployPath: b.DeployPath,
		KeepReleases: b.KeepReleases, Settings: settingsInOf(b.Settings), AllowedImages: b.AllowedImages, Hosts: hosts,
	}, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.CreateDeployTarget201JSONResponse{Success: true, Message: ptrOf(messageTargetCreated), Data: &data}, nil
}

func (c *Controller) GetDeployTarget(ctx context.Context, req gen.GetDeployTargetRequestObject) (gen.GetDeployTargetResponseObject, error) {
	t, err := c.service.GetTarget(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName)
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.GetDeployTarget200JSONResponse{Success: true, Message: ptrOf(messageTargetLoaded), Data: &data}, nil
}

func (c *Controller) UpdateDeployTarget(ctx context.Context, req gen.UpdateDeployTargetRequestObject) (gen.UpdateDeployTargetResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	t, err := c.service.UpdateTarget(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, models.UpdateTargetInput{
		Version: b.Version, Username: b.Username, KeySecretName: b.KeySecretName, DeployPath: b.DeployPath, KeepReleases: b.KeepReleases,
		Settings: settingsInOf(b.Settings), AllowedImages: b.AllowedImages,
	}, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.UpdateDeployTarget200JSONResponse{Success: true, Message: ptrOf(messageTargetUpdated), Data: &data}, nil
}

func (c *Controller) RemoveDeployTarget(ctx context.Context, req gen.RemoveDeployTargetRequestObject) (gen.RemoveDeployTargetResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	t, err := c.service.RemoveTarget(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.RemoveDeployTarget200JSONResponse{Success: true, Message: ptrOf(messageTargetRemoved), Data: &data}, nil
}

func (c *Controller) GenerateDeployTargetKey(ctx context.Context, req gen.GenerateDeployTargetKeyRequestObject) (gen.GenerateDeployTargetKeyResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	t, err := c.service.GenerateKey(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, factorOf(req.Body.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.GenerateDeployTargetKey200JSONResponse{Success: true, Message: ptrOf(messageKeyGenerated), Data: &data}, nil
}

func (c *Controller) AddDeployTargetHost(ctx context.Context, req gen.AddDeployTargetHostRequestObject) (gen.AddDeployTargetHostResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	t, err := c.service.AddHost(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, b.Version,
		models.HostInput{Host: b.Host, Port: b.Port}, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.AddDeployTargetHost200JSONResponse{Success: true, Message: ptrOf(messageHostAdded), Data: &data}, nil
}

func (c *Controller) ReorderDeployTargetHosts(ctx context.Context, req gen.ReorderDeployTargetHostsRequestObject) (gen.ReorderDeployTargetHostsResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	t, err := c.service.ReorderHosts(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, b.Version, b.HostIds, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.ReorderDeployTargetHosts200JSONResponse{Success: true, Message: ptrOf(messageHostsReordered), Data: &data}, nil
}

func (c *Controller) UpdateDeployTargetHost(ctx context.Context, req gen.UpdateDeployTargetHostRequestObject) (gen.UpdateDeployTargetHostResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	t, err := c.service.UpdateHost(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, req.HostId,
		models.UpdateHostInput{Version: b.Version, Host: b.Host, Port: b.Port}, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.UpdateDeployTargetHost200JSONResponse{Success: true, Message: ptrOf(messageHostUpdated), Data: &data}, nil
}

func (c *Controller) RemoveDeployTargetHost(ctx context.Context, req gen.RemoveDeployTargetHostRequestObject) (gen.RemoveDeployTargetHostResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	t, err := c.service.RemoveHost(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, req.HostId, b.Version, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.RemoveDeployTargetHost200JSONResponse{Success: true, Message: ptrOf(messageHostRemoved), Data: &data}, nil
}

func (c *Controller) FetchDeployTargetHostKey(ctx context.Context, req gen.FetchDeployTargetHostKeyRequestObject) (gen.FetchDeployTargetHostKeyResponseObject, error) {
	t, err := c.service.FetchHostKey(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, req.HostId)
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.FetchDeployTargetHostKey200JSONResponse{Success: true, Message: ptrOf(messageHostKeyFetched), Data: &data}, nil
}

func (c *Controller) SetDeployTargetPendingHostKey(ctx context.Context, req gen.SetDeployTargetPendingHostKeyRequestObject) (gen.SetDeployTargetPendingHostKeyResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	t, err := c.service.SetPendingHostKey(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, req.HostId, req.Body.HostKey)
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.SetDeployTargetPendingHostKey200JSONResponse{Success: true, Message: ptrOf(messageHostKeyPending), Data: &data}, nil
}

func (c *Controller) ConfirmDeployTargetHostKey(ctx context.Context, req gen.ConfirmDeployTargetHostKeyRequestObject) (gen.ConfirmDeployTargetHostKeyResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	t, err := c.service.ConfirmHostKey(ctx, req.Id, req.ProjectId, req.EnvironmentName, req.TargetName, req.HostId, b.Fingerprint, factorOf(b.CurrentFactor))
	if err != nil {
		return nil, err
	}
	data := targetView(t)
	return gen.ConfirmDeployTargetHostKey200JSONResponse{Success: true, Message: ptrOf(messageHostKeyConfirmed), Data: &data}, nil
}
