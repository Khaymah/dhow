package secrets

import (
	"context"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

const (
	messageListed          = "secrets.list.loaded"
	messageSaved           = "secrets.secret.saved"
	messageRemoved         = "secrets.secret.removed"
	messageRestored        = "secrets.secret.restored"
	messageVariablesListed = "secrets.variables.loaded"
	messageVariableSaved   = "secrets.variable.saved"
	messageVariableRemoved = "secrets.variable.removed"
	messageRegistryListed  = "secrets.registry.loaded"
	messageRegistryCreated = "secrets.registry.created"
	messageRegistryUpdated = "secrets.registry.updated"
	messageRegistryRemoved = "secrets.registry.removed"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptrOf[T any](v T) *T { return &v }

func orgRef(org uuid.UUID) models.Ref { return models.Ref{OrganizationID: org} }

func projectRef(org, project uuid.UUID) models.Ref {
	return models.Ref{OrganizationID: org, ProjectID: &project}
}

func envRef(org, project uuid.UUID, env string) models.Ref {
	return models.Ref{OrganizationID: org, ProjectID: &project, EnvironmentName: env}
}

func scopesOf(list []string) []gen.SecretScope {
	out := make([]gen.SecretScope, 0, len(list))
	for _, s := range list {
		out = append(out, gen.SecretScope(s))
	}
	return out
}

func toSecret(s models.Secret) gen.Secret {
	return gen.Secret{
		Name: s.Name, Scope: gen.SecretScope(s.Scope), Description: s.Description, Version: int(s.Version), SizeBytes: int(s.SizeBytes),
		AvailableToPullRequests: s.AvailableToPullRequests, Visibility: gen.SecretVisibility(s.Visibility), ProjectIds: s.ProjectIDs,
		Overrides: scopesOf(s.Overrides), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, UpdatedByUserId: s.UpdatedByUserID,
		LastUsedAt: s.LastUsedAt,
	}
}

func factorOf(f *gen.CurrentFactor) *auth.Factor {
	if f == nil {
		return nil
	}
	out := auth.Factor{Method: string(f.Method)}
	if f.Password != nil {
		out.Password = *f.Password
	}
	if f.Code != nil {
		out.Code = *f.Code
	}
	if f.ChallengeId != nil {
		out.ChallengeID = *f.ChallengeId
	}
	if f.Credential != nil {
		out.Credential = *f.Credential
	}
	return &out
}

func secretInput(body *gen.PutSecretRequest) (models.PutSecretInput, error) {
	if body == nil || body.Value == nil {
		return models.PutSecretInput{}, invalid(fieldValue, messageValue)
	}
	in := models.PutSecretInput{
		Value: *body.Value, Description: body.Description, AvailableToPullRequests: body.AvailableToPullRequests, ProjectIDs: body.ProjectIds,
		Factor: factorOf(body.CurrentFactor),
	}
	if body.Visibility != nil {
		in.Visibility = ptrOf(string(*body.Visibility))
	}
	return in, nil
}

func removalFactor(body *gen.RemoveEnvironmentSecretJSONRequestBody) *auth.Factor {
	if body == nil {
		return nil
	}
	return factorOf(&body.CurrentFactor)
}

func versionOf(body *gen.RestoreSecretVersionRequest) (int, *auth.Factor, error) {
	if body == nil {
		return 0, nil, apperrors.BadRequest()
	}
	return body.Version, factorOf(body.CurrentFactor), nil
}

func (c *Controller) listSecrets(ctx context.Context, ref models.Ref, pageIn, limitIn *int) (gen.SecretListResponse, error) {
	page, limit := response.PageParams(pageIn, limitIn)
	list, err := c.service.ListSecrets(ctx, ref, page, limit)
	if err != nil {
		return gen.SecretListResponse{}, err
	}
	items := make([]gen.Secret, 0, len(list.Items))
	for _, s := range list.Items {
		items = append(items, toSecret(s))
	}
	return gen.SecretListResponse{
		Success: true, Message: ptrOf(messageListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) putSecret(ctx context.Context, ref models.Ref, name string, body *gen.PutSecretRequest) (gen.SecretResponse, error) {
	in, err := secretInput(body)
	if err != nil {
		return gen.SecretResponse{}, err
	}
	s, err := c.service.PutSecret(ctx, ref, name, in)
	return secretResponse(s, messageSaved), err
}

func (c *Controller) restoreSecret(ctx context.Context, ref models.Ref, name string, body *gen.RestoreSecretVersionRequest) (gen.SecretResponse, error) {
	version, factor, err := versionOf(body)
	if err != nil {
		return gen.SecretResponse{}, err
	}
	s, err := c.service.RestoreSecretVersion(ctx, ref, name, version, factor)
	return secretResponse(s, messageRestored), err
}

func secretResponse(s models.Secret, message string) gen.SecretResponse {
	data := toSecret(s)
	return gen.SecretResponse{Success: true, Message: ptrOf(message), Data: &data}
}

func (c *Controller) removeSecret(ctx context.Context, ref models.Ref, name string, factor *auth.Factor) (gen.SecretResponse, error) {
	s, err := c.service.RemoveSecret(ctx, ref, name, factor)
	return secretResponse(s, messageRemoved), err
}

func (c *Controller) ListOrganizationSecrets(ctx context.Context, req gen.ListOrganizationSecretsRequestObject) (gen.ListOrganizationSecretsResponseObject, error) {
	out, err := c.listSecrets(ctx, orgRef(req.Id), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListOrganizationSecrets200JSONResponse(out), nil
}

func (c *Controller) PutOrganizationSecret(ctx context.Context, req gen.PutOrganizationSecretRequestObject) (gen.PutOrganizationSecretResponseObject, error) {
	out, err := c.putSecret(ctx, orgRef(req.Id), req.SecretName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutOrganizationSecret200JSONResponse(out), nil
}

func (c *Controller) RemoveOrganizationSecret(ctx context.Context, req gen.RemoveOrganizationSecretRequestObject) (gen.RemoveOrganizationSecretResponseObject, error) {
	out, err := c.removeSecret(ctx, orgRef(req.Id), req.SecretName, nil)
	if err != nil {
		return nil, err
	}
	return gen.RemoveOrganizationSecret200JSONResponse(out), nil
}

func (c *Controller) RestoreOrganizationSecretVersion(ctx context.Context, req gen.RestoreOrganizationSecretVersionRequestObject) (gen.RestoreOrganizationSecretVersionResponseObject, error) {
	out, err := c.restoreSecret(ctx, orgRef(req.Id), req.SecretName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.RestoreOrganizationSecretVersion200JSONResponse(out), nil
}

func (c *Controller) ListProjectSecrets(ctx context.Context, req gen.ListProjectSecretsRequestObject) (gen.ListProjectSecretsResponseObject, error) {
	out, err := c.listSecrets(ctx, projectRef(req.Id, req.ProjectId), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListProjectSecrets200JSONResponse(out), nil
}

func (c *Controller) PutProjectSecret(ctx context.Context, req gen.PutProjectSecretRequestObject) (gen.PutProjectSecretResponseObject, error) {
	out, err := c.putSecret(ctx, projectRef(req.Id, req.ProjectId), req.SecretName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutProjectSecret200JSONResponse(out), nil
}

func (c *Controller) RemoveProjectSecret(ctx context.Context, req gen.RemoveProjectSecretRequestObject) (gen.RemoveProjectSecretResponseObject, error) {
	out, err := c.removeSecret(ctx, projectRef(req.Id, req.ProjectId), req.SecretName, nil)
	if err != nil {
		return nil, err
	}
	return gen.RemoveProjectSecret200JSONResponse(out), nil
}

func (c *Controller) RestoreProjectSecretVersion(ctx context.Context, req gen.RestoreProjectSecretVersionRequestObject) (gen.RestoreProjectSecretVersionResponseObject, error) {
	out, err := c.restoreSecret(ctx, projectRef(req.Id, req.ProjectId), req.SecretName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.RestoreProjectSecretVersion200JSONResponse(out), nil
}

func (c *Controller) ListEnvironmentSecrets(ctx context.Context, req gen.ListEnvironmentSecretsRequestObject) (gen.ListEnvironmentSecretsResponseObject, error) {
	out, err := c.listSecrets(ctx, envRef(req.Id, req.ProjectId, req.EnvironmentName), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListEnvironmentSecrets200JSONResponse(out), nil
}

func (c *Controller) PutEnvironmentSecret(ctx context.Context, req gen.PutEnvironmentSecretRequestObject) (gen.PutEnvironmentSecretResponseObject, error) {
	out, err := c.putSecret(ctx, envRef(req.Id, req.ProjectId, req.EnvironmentName), req.SecretName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutEnvironmentSecret200JSONResponse(out), nil
}

func (c *Controller) RemoveEnvironmentSecret(ctx context.Context, req gen.RemoveEnvironmentSecretRequestObject) (gen.RemoveEnvironmentSecretResponseObject, error) {
	out, err := c.removeSecret(ctx, envRef(req.Id, req.ProjectId, req.EnvironmentName), req.SecretName, removalFactor(req.Body))
	if err != nil {
		return nil, err
	}
	return gen.RemoveEnvironmentSecret200JSONResponse(out), nil
}

func (c *Controller) RestoreEnvironmentSecretVersion(ctx context.Context, req gen.RestoreEnvironmentSecretVersionRequestObject) (gen.RestoreEnvironmentSecretVersionResponseObject, error) {
	out, err := c.restoreSecret(ctx, envRef(req.Id, req.ProjectId, req.EnvironmentName), req.SecretName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.RestoreEnvironmentSecretVersion200JSONResponse(out), nil
}
