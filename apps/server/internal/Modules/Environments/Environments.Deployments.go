package environments

import (
	"context"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	"github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Gen"
)

const messageDeploymentsListed = "deployments.list.loaded"

type DeploymentPage struct {
	Items []sqlc.Deployment
	Total int64
}

func (s *Service) Deployments(ctx context.Context, org, project uuid.UUID, name string, page, limit int) (DeploymentPage, error) {
	if !ValidName(name) {
		return DeploymentPage{}, apperrors.NotFound()
	}
	q := s.queries()
	env, err := q.GetEnvironment(ctx, sqlc.GetEnvironmentParams{ProjectID: project, OrganizationID: org, Name: name})
	if err != nil {
		return DeploymentPage{}, notFoundOr(err)
	}
	total, err := q.CountDeployments(ctx, sqlc.CountDeploymentsParams{EnvironmentID: env.Id, OrganizationID: org})
	if err != nil {
		return DeploymentPage{}, err
	}
	rows, err := q.ListDeployments(ctx, sqlc.ListDeploymentsParams{
		EnvironmentID: env.Id, OrganizationID: org, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return DeploymentPage{}, err
	}
	return DeploymentPage{Items: rows, Total: total}, nil
}

func deploymentOf(d sqlc.Deployment) gen.Deployment {
	out := gen.Deployment{
		Id: d.Id, RunId: d.RunId, RunNumber: d.RunNumber, RunJobId: d.RunJobId, JobId: d.JobId, JobKey: d.JobKey, Ref: d.Ref,
		RuleVersion: d.RuleVersion, State: gen.DeploymentState(d.State), CreatedAt: d.CreatedAt, FinishedAt: d.FinishedAt,
	}
	if d.Sha.Valid {
		out.Sha = ptrOf(d.Sha.String)
	}
	return out
}

func (c *Controller) ListDeployments(ctx context.Context, req gen.ListDeploymentsRequestObject) (gen.ListDeploymentsResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	list, err := c.service.Deployments(ctx, req.Id, req.ProjectId, req.EnvironmentName, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Deployment, 0, len(list.Items))
	for _, d := range list.Items {
		items = append(items, deploymentOf(d))
	}
	return gen.ListDeployments200JSONResponse{
		Success: true, Message: ptrOf(messageDeploymentsListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}
