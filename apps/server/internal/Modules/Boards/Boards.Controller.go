package boards

import (
	"context"

	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Boards/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Boards/Models"
)

const messageLoaded = "boards.deploy.loaded"

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptr[T any](v T) *T { return &v }

func (c *Controller) GetProjectDeployBoard(ctx context.Context, req gen.GetProjectDeployBoardRequestObject) (gen.GetProjectDeployBoardResponseObject, error) {
	board, err := c.service.Get(ctx, req.Id, req.ProjectId)
	if err != nil {
		return nil, err
	}
	data := toGen(board)
	etag, err := response.ETagOf(data)
	if err != nil {
		return nil, err
	}
	if response.ETagMatches(req.Params.IfNoneMatch, etag) {
		return gen.GetProjectDeployBoard304Response{}, nil
	}
	return gen.GetProjectDeployBoard200JSONResponse{
		Body:    gen.DeployBoardResponse{Success: true, Message: ptr(messageLoaded), Data: &data},
		Headers: gen.GetProjectDeployBoard200ResponseHeaders{ETag: &etag},
	}, nil
}

func toGen(b models.Board) gen.DeployBoard {
	envs := make([]gen.DeployBoardEnvironment, 0, len(b.Environments))
	for _, e := range b.Environments {
		targets := make([]gen.DeployBoardTarget, 0, len(e.Targets))
		for _, t := range e.Targets {
			targets = append(targets, targetToGen(t))
		}
		envs = append(envs, gen.DeployBoardEnvironment{Id: e.ID, Name: e.Name, Paused: e.Paused, PendingApprovals: e.PendingApprovals, Targets: targets})
	}
	return gen.DeployBoard{Environments: envs, Truncated: b.Truncated}
}

func targetToGen(t models.Target) gen.DeployBoardTarget {
	out := gen.DeployBoardTarget{
		Id: t.ID, Name: t.Name, Kind: gen.DeployBoardTargetKind(t.Kind), Hosts: t.Hosts, Partial: t.Partial,
		AutoRolledBack: t.AutoRolledBack, RollbackFailed: t.RollbackFailed,
		CurrentReleaseId: optional(t.CurrentReleaseID), CurrentDigest: optional(t.CurrentDigest),
	}
	if t.Last != nil {
		out.Last = &gen.DeployBoardRelease{
			Id: t.Last.ID, ReleaseId: t.Last.ReleaseID, Kind: gen.DeployBoardReleaseKind(t.Last.Kind), State: gen.DeployBoardReleaseState(t.Last.State),
			RollbackReason: optional(t.Last.RollbackReason), StartedAt: t.Last.StartedAt, FinishedAt: t.Last.FinishedAt,
		}
	}
	if t.Running != nil {
		out.Running = &gen.DeployBoardProgress{
			ReleaseId: t.Running.ReleaseID, Hosts: t.Running.Hosts, HostsDone: t.Running.HostsDone,
			BatchSize: t.Running.BatchSize, Batch: t.Running.Batch, Batches: t.Running.Batches,
		}
	}
	return out
}

func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
