package sources

import (
	"context"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
)

const (
	messageLoaded           = "sources.repository.loaded"
	messageLinked           = "sources.repository.linked"
	messageUpdated          = "sources.repository.updated"
	messageHostKeyConfirmed = "sources.hostKey.confirmed"
	messageDeployRotated    = "sources.deployKey.rotated"
	messageDeployConfirmed  = "sources.deployKey.confirmed"
	messageSecretRotated    = "sources.webhookSecret.rotated"
	messageUnlinked         = "sources.repository.unlinked"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptrOf[T any](v T) *T { return &v }

func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (c *Controller) toGen(r models.Repository) gen.Repository {
	return gen.Repository{
		Id: r.ID, ProjectId: r.ProjectID, Provider: gen.GitProvider(r.Provider), CloneUrl: r.CloneURL, Transport: gen.GitTransport(r.Transport),
		FullName: r.FullName, DefaultBranch: r.DefaultBranch, WebhookUrl: c.service.WebhookURL(r.HookID),
		DeployKeyPublic: optional(r.DeployKeyPublic), PreviousDeployKeyPublic: optional(r.PreviousDeployKeyPublic),
		PreviousDeployKeyExpiresAt: r.PreviousDeployKeyExpiresAt, HostKeyFingerprint: optional(r.HostKeyFingerprint),
		PendingHostKeyFingerprint: optional(r.PendingHostKeyFingerprint), State: gen.RepositoryState(r.State),
		LastDeliveryAt: r.LastDeliveryAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (c *Controller) withSecret(l models.Linked) *gen.RepositoryWithSecret {
	return &gen.RepositoryWithSecret{Repository: c.toGen(l.Repository), WebhookSecret: optional(l.WebhookSecret)}
}

func (c *Controller) GetRepository(ctx context.Context, req gen.GetRepositoryRequestObject) (gen.GetRepositoryResponseObject, error) {
	r, err := c.service.Get(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data := c.toGen(r)
	return gen.GetRepository200JSONResponse{Success: true, Message: ptrOf(messageLoaded), Data: &data}, nil
}

func (c *Controller) LinkRepository(ctx context.Context, req gen.LinkRepositoryRequestObject) (gen.LinkRepositoryResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	l, err := c.service.Link(ctx, req.Id, req.ProjectId, models.LinkInput{CloneURL: req.Body.CloneUrl, DefaultBranch: req.Body.DefaultBranch})
	if err != nil {
		return nil, err
	}
	if l.Created {
		return gen.LinkRepository201JSONResponse{Success: true, Message: ptrOf(messageLinked), Data: c.withSecret(l)}, nil
	}
	return gen.LinkRepository200JSONResponse{Success: true, Message: ptrOf(messageUpdated), Data: c.withSecret(l)}, nil
}

func (c *Controller) ConfirmHostKey(ctx context.Context, req gen.ConfirmHostKeyRequestObject) (gen.ConfirmHostKeyResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	r, err := c.service.ConfirmHostKey(ctx, req.Id, req.ProjectId, req.Body.Fingerprint)
	if err != nil {
		return nil, err
	}
	data := c.toGen(r)
	return gen.ConfirmHostKey200JSONResponse{Success: true, Message: ptrOf(messageHostKeyConfirmed), Data: &data}, nil
}

func (c *Controller) RotateDeployKey(ctx context.Context, req gen.RotateDeployKeyRequestObject) (gen.RotateDeployKeyResponseObject, error) {
	r, err := c.service.RotateDeployKey(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data := c.toGen(r)
	return gen.RotateDeployKey200JSONResponse{Success: true, Message: ptrOf(messageDeployRotated), Data: &data}, nil
}

func (c *Controller) ConfirmDeployKey(ctx context.Context, req gen.ConfirmDeployKeyRequestObject) (gen.ConfirmDeployKeyResponseObject, error) {
	r, err := c.service.ConfirmDeployKey(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data := c.toGen(r)
	return gen.ConfirmDeployKey200JSONResponse{Success: true, Message: ptrOf(messageDeployConfirmed), Data: &data}, nil
}

func (c *Controller) RotateWebhookSecret(
	ctx context.Context, req gen.RotateWebhookSecretRequestObject,
) (gen.RotateWebhookSecretResponseObject, error) {
	l, err := c.service.RotateWebhookSecret(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	return gen.RotateWebhookSecret200JSONResponse{Success: true, Message: ptrOf(messageSecretRotated), Data: c.withSecret(l)}, nil
}

func (c *Controller) UnlinkRepository(ctx context.Context, req gen.UnlinkRepositoryRequestObject) (gen.UnlinkRepositoryResponseObject, error) {
	r, err := c.service.Unlink(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data := c.toGen(r)
	return gen.UnlinkRepository200JSONResponse{Success: true, Message: ptrOf(messageUnlinked), Data: &data}, nil
}
