package secrets

import (
	"context"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

func toVariable(v models.Variable) gen.Variable {
	return gen.Variable{
		Name: v.Name, Scope: gen.SecretScope(v.Scope), Value: v.Value, Overrides: scopesOf(v.Overrides),
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, UpdatedByUserId: v.UpdatedByUserID,
	}
}

func variableResponse(v models.Variable, message string) gen.VariableResponse {
	data := toVariable(v)
	return gen.VariableResponse{Success: true, Message: ptrOf(message), Data: &data}
}

func (c *Controller) listVariables(ctx context.Context, ref models.Ref, pageIn, limitIn *int) (gen.VariableListResponse, error) {
	page, limit := response.PageParams(pageIn, limitIn)
	list, err := c.service.ListVariables(ctx, ref, page, limit)
	if err != nil {
		return gen.VariableListResponse{}, err
	}
	items := make([]gen.Variable, 0, len(list.Items))
	for _, v := range list.Items {
		items = append(items, toVariable(v))
	}
	return gen.VariableListResponse{
		Success: true, Message: ptrOf(messageVariablesListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) putVariable(ctx context.Context, ref models.Ref, name string, body *gen.PutVariableRequest) (gen.VariableResponse, error) {
	if body == nil {
		return gen.VariableResponse{}, apperrors.BadRequest()
	}
	v, err := c.service.PutVariable(ctx, ref, name, body.Value)
	return variableResponse(v, messageVariableSaved), err
}

func (c *Controller) removeVariable(ctx context.Context, ref models.Ref, name string) (gen.VariableResponse, error) {
	v, err := c.service.RemoveVariable(ctx, ref, name)
	return variableResponse(v, messageVariableRemoved), err
}

func (c *Controller) ListOrganizationVariables(ctx context.Context, req gen.ListOrganizationVariablesRequestObject) (gen.ListOrganizationVariablesResponseObject, error) {
	out, err := c.listVariables(ctx, orgRef(req.Id), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListOrganizationVariables200JSONResponse(out), nil
}

func (c *Controller) PutOrganizationVariable(ctx context.Context, req gen.PutOrganizationVariableRequestObject) (gen.PutOrganizationVariableResponseObject, error) {
	out, err := c.putVariable(ctx, orgRef(req.Id), req.VariableName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutOrganizationVariable200JSONResponse(out), nil
}

func (c *Controller) RemoveOrganizationVariable(ctx context.Context, req gen.RemoveOrganizationVariableRequestObject) (gen.RemoveOrganizationVariableResponseObject, error) {
	out, err := c.removeVariable(ctx, orgRef(req.Id), req.VariableName)
	if err != nil {
		return nil, err
	}
	return gen.RemoveOrganizationVariable200JSONResponse(out), nil
}

func (c *Controller) ListProjectVariables(ctx context.Context, req gen.ListProjectVariablesRequestObject) (gen.ListProjectVariablesResponseObject, error) {
	out, err := c.listVariables(ctx, projectRef(req.Id, req.ProjectId), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListProjectVariables200JSONResponse(out), nil
}

func (c *Controller) PutProjectVariable(ctx context.Context, req gen.PutProjectVariableRequestObject) (gen.PutProjectVariableResponseObject, error) {
	out, err := c.putVariable(ctx, projectRef(req.Id, req.ProjectId), req.VariableName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutProjectVariable200JSONResponse(out), nil
}

func (c *Controller) RemoveProjectVariable(ctx context.Context, req gen.RemoveProjectVariableRequestObject) (gen.RemoveProjectVariableResponseObject, error) {
	out, err := c.removeVariable(ctx, projectRef(req.Id, req.ProjectId), req.VariableName)
	if err != nil {
		return nil, err
	}
	return gen.RemoveProjectVariable200JSONResponse(out), nil
}

func (c *Controller) ListEnvironmentVariables(ctx context.Context, req gen.ListEnvironmentVariablesRequestObject) (gen.ListEnvironmentVariablesResponseObject, error) {
	out, err := c.listVariables(ctx, envRef(req.Id, req.ProjectId, req.EnvironmentName), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListEnvironmentVariables200JSONResponse(out), nil
}

func (c *Controller) PutEnvironmentVariable(ctx context.Context, req gen.PutEnvironmentVariableRequestObject) (gen.PutEnvironmentVariableResponseObject, error) {
	out, err := c.putVariable(ctx, envRef(req.Id, req.ProjectId, req.EnvironmentName), req.VariableName, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutEnvironmentVariable200JSONResponse(out), nil
}

func (c *Controller) RemoveEnvironmentVariable(ctx context.Context, req gen.RemoveEnvironmentVariableRequestObject) (gen.RemoveEnvironmentVariableResponseObject, error) {
	out, err := c.removeVariable(ctx, envRef(req.Id, req.ProjectId, req.EnvironmentName), req.VariableName)
	if err != nil {
		return nil, err
	}
	return gen.RemoveEnvironmentVariable200JSONResponse(out), nil
}

func toRegistry(r models.RegistryCredential) gen.RegistryCredential {
	return gen.RegistryCredential{
		Id: r.ID, Scope: gen.SecretScope(r.Scope), Host: r.Host, Username: r.Username, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		UpdatedByUserId: r.UpdatedByUserID,
	}
}

func registryResponse(r models.RegistryCredential, message string) gen.RegistryCredentialResponse {
	data := toRegistry(r)
	return gen.RegistryCredentialResponse{Success: true, Message: ptrOf(message), Data: &data}
}

func (c *Controller) listRegistry(ctx context.Context, ref models.Ref, pageIn, limitIn *int) (gen.RegistryCredentialListResponse, error) {
	page, limit := response.PageParams(pageIn, limitIn)
	list, err := c.service.ListRegistryCredentials(ctx, ref, page, limit)
	if err != nil {
		return gen.RegistryCredentialListResponse{}, err
	}
	items := make([]gen.RegistryCredential, 0, len(list.Items))
	for _, r := range list.Items {
		items = append(items, toRegistry(r))
	}
	return gen.RegistryCredentialListResponse{
		Success: true, Message: ptrOf(messageRegistryListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) createRegistry(ctx context.Context, ref models.Ref, body *gen.CreateRegistryCredentialRequest) (gen.RegistryCredentialResponse, error) {
	if body == nil {
		return gen.RegistryCredentialResponse{}, apperrors.BadRequest()
	}
	r, err := c.service.CreateRegistryCredential(ctx, ref, models.RegistryInput{Host: body.Host, Username: body.Username, Password: valueOr(body.Password, "")})
	return registryResponse(r, messageRegistryCreated), err
}

func (c *Controller) updateRegistry(ctx context.Context, ref models.Ref, id uuid.UUID, body *gen.UpdateRegistryCredentialRequest) (gen.RegistryCredentialResponse, error) {
	if body == nil {
		return gen.RegistryCredentialResponse{}, apperrors.BadRequest()
	}
	r, err := c.service.UpdateRegistryCredential(ctx, ref, id, models.RegistryInput{Username: body.Username, Password: valueOr(body.Password, "")})
	return registryResponse(r, messageRegistryUpdated), err
}

func (c *Controller) removeRegistry(ctx context.Context, ref models.Ref, id uuid.UUID) (gen.RegistryCredentialResponse, error) {
	r, err := c.service.RemoveRegistryCredential(ctx, ref, id)
	return registryResponse(r, messageRegistryRemoved), err
}

func (c *Controller) ListOrganizationRegistryCredentials(ctx context.Context, req gen.ListOrganizationRegistryCredentialsRequestObject) (gen.ListOrganizationRegistryCredentialsResponseObject, error) {
	out, err := c.listRegistry(ctx, orgRef(req.Id), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListOrganizationRegistryCredentials200JSONResponse(out), nil
}

func (c *Controller) CreateOrganizationRegistryCredential(ctx context.Context, req gen.CreateOrganizationRegistryCredentialRequestObject) (gen.CreateOrganizationRegistryCredentialResponseObject, error) {
	out, err := c.createRegistry(ctx, orgRef(req.Id), req.Body)
	if err != nil {
		return nil, err
	}
	return gen.CreateOrganizationRegistryCredential201JSONResponse(out), nil
}

func (c *Controller) UpdateOrganizationRegistryCredential(ctx context.Context, req gen.UpdateOrganizationRegistryCredentialRequestObject) (gen.UpdateOrganizationRegistryCredentialResponseObject, error) {
	out, err := c.updateRegistry(ctx, orgRef(req.Id), req.CredentialId, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.UpdateOrganizationRegistryCredential200JSONResponse(out), nil
}

func (c *Controller) RemoveOrganizationRegistryCredential(ctx context.Context, req gen.RemoveOrganizationRegistryCredentialRequestObject) (gen.RemoveOrganizationRegistryCredentialResponseObject, error) {
	out, err := c.removeRegistry(ctx, orgRef(req.Id), req.CredentialId)
	if err != nil {
		return nil, err
	}
	return gen.RemoveOrganizationRegistryCredential200JSONResponse(out), nil
}

func (c *Controller) ListProjectRegistryCredentials(ctx context.Context, req gen.ListProjectRegistryCredentialsRequestObject) (gen.ListProjectRegistryCredentialsResponseObject, error) {
	out, err := c.listRegistry(ctx, projectRef(req.Id, req.ProjectId), req.Params.Page, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return gen.ListProjectRegistryCredentials200JSONResponse(out), nil
}

func (c *Controller) CreateProjectRegistryCredential(ctx context.Context, req gen.CreateProjectRegistryCredentialRequestObject) (gen.CreateProjectRegistryCredentialResponseObject, error) {
	out, err := c.createRegistry(ctx, projectRef(req.Id, req.ProjectId), req.Body)
	if err != nil {
		return nil, err
	}
	return gen.CreateProjectRegistryCredential201JSONResponse(out), nil
}

func (c *Controller) UpdateProjectRegistryCredential(ctx context.Context, req gen.UpdateProjectRegistryCredentialRequestObject) (gen.UpdateProjectRegistryCredentialResponseObject, error) {
	out, err := c.updateRegistry(ctx, projectRef(req.Id, req.ProjectId), req.CredentialId, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.UpdateProjectRegistryCredential200JSONResponse(out), nil
}

func (c *Controller) RemoveProjectRegistryCredential(ctx context.Context, req gen.RemoveProjectRegistryCredentialRequestObject) (gen.RemoveProjectRegistryCredentialResponseObject, error) {
	out, err := c.removeRegistry(ctx, projectRef(req.Id, req.ProjectId), req.CredentialId)
	if err != nil {
		return nil, err
	}
	return gen.RemoveProjectRegistryCredential200JSONResponse(out), nil
}
