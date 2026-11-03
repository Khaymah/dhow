package pipelines

import (
	"context"
	"slices"
	"strings"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
)

const (
	messageListed          = "pipelines.runs.loaded"
	messageLoaded          = "pipelines.run.loaded"
	messageCreated         = "pipelines.run.created"
	messageCancelRequested = "pipelines.run.cancelRequested"
	messageRerun           = "pipelines.run.rerun"
	messageApproved        = "pipelines.run.approved"
	maxRefFilter           = 255
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

func runToGen(r models.Run) gen.Run {
	out := gen.Run{
		Id: r.ID, ProjectId: r.ProjectID, Number: r.Number, Trigger: gen.RunTrigger(r.Trigger), Ref: r.Ref, Sha: optional(r.Sha),
		BaseRef: optional(r.BaseRef), BaseSha: optional(r.BaseSha), IsFork: r.IsFork, Actor: optional(r.Actor), State: gen.RunState(r.State),
		FailureReason: optional(r.FailureReason), DefinitionPath: r.DefinitionPath, DefinitionSha: optional(r.DefinitionSha),
		DefinitionDigest: optional(r.DefinitionDigest), ConcurrencyGroup: optional(r.ConcurrencyGroup),
		TriggeredByUserId: r.TriggeredByUserID, ApprovedByUserId: r.ApprovedByUserID, ApprovedAt: r.ApprovedAt,
		CancelRequestedAt: r.CancelRequestedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.PrNumber > 0 {
		out.PrNumber = ptrOf(r.PrNumber)
	}
	return out
}

func detailToGen(d models.Detail) gen.RunDetail {
	out := gen.RunDetail{
		Run: runToGen(d.Run), Jobs: make([]gen.RunJob, 0, len(d.Jobs)), Issues: make([]gen.RunIssue, 0, len(d.Issues)),
		ImagePins: make([]gen.RunImagePin, 0, len(d.ImagePins)),
	}
	for _, j := range d.Jobs {
		out.Jobs = append(out.Jobs, gen.RunJob{
			Id: j.ID, Key: j.Key, Kind: gen.RunJobKind(j.Kind), JobId: j.JobID, Name: j.Name, Matrix: j.Matrix, Needs: j.Needs, State: gen.RunJobState(j.State),
			FailureReason: optional(j.FailureReason), CurrentJobId: j.CurrentJobID, Attempts: j.Attempts, StartedAt: j.StartedAt,
			FinishedAt: j.FinishedAt,
		})
	}
	for _, i := range d.Issues {
		out.Issues = append(out.Issues, gen.RunIssue{
			Code: i.Code, Severity: gen.RunIssueSeverity(i.Severity), Path: i.Path, Line: i.Line, Column: i.Column, Message: i.Message,
		})
	}
	for _, p := range d.ImagePins {
		out.ImagePins = append(out.ImagePins, gen.RunImagePin{Image: p.Image, Pinned: p.Pinned})
	}
	return out
}

func stateFilter(raw *[]string) ([]string, error) {
	out := []string{}
	if raw == nil {
		return out, nil
	}
	for _, part := range *raw {
		for v := range strings.SplitSeq(part, ",") {
			v = strings.TrimSpace(v)
			if !slices.Contains(models.RunStates(), v) {
				return nil, apperrors.ValidationFailed(invalid("State"))
			}
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

func (c *Controller) ListRuns(ctx context.Context, req gen.ListRunsRequestObject) (gen.ListRunsResponseObject, error) {
	page, limit := response.PageParams(req.Params.Page, req.Params.Limit)
	states, err := stateFilter(req.Params.State)
	if err != nil {
		return nil, err
	}
	ref := ""
	if req.Params.Ref != nil {
		ref = *req.Params.Ref
		if len(ref) > maxRefFilter {
			return nil, apperrors.ValidationFailed(invalid("Ref"))
		}
	}
	list, err := c.service.List(ctx, req.Id, req.ProjectId, states, ref, page, limit)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Run, 0, len(list.Items))
	for _, r := range list.Items {
		items = append(items, runToGen(r))
	}
	return gen.ListRuns200JSONResponse{
		Success: true, Message: ptrOf(messageListed), Data: &items, Meta: &gen.PageMeta{Page: page, Limit: limit, Total: int(list.Total)},
	}, nil
}

func (c *Controller) TriggerRun(ctx context.Context, req gen.TriggerRunRequestObject) (gen.TriggerRunResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	in := models.TriggerInput{Ref: req.Body.Ref, Inputs: map[string]any{}}
	if req.Body.Sha != nil {
		in.Sha = *req.Body.Sha
	}
	if req.Body.Inputs != nil {
		in.Inputs = *req.Body.Inputs
	}
	run, err := c.service.Trigger(ctx, req.Id, req.ProjectId, in)
	if err != nil {
		return nil, err
	}
	data := runToGen(run)
	return gen.TriggerRun201JSONResponse{Success: true, Message: ptrOf(messageCreated), Data: &data}, nil
}

func (c *Controller) GetRun(ctx context.Context, req gen.GetRunRequestObject) (gen.GetRunResponseObject, error) {
	d, err := c.service.Get(ctx, req.Id, req.ProjectId, req.RunId)
	if err != nil {
		return nil, err
	}
	data := detailToGen(d)
	return gen.GetRun200JSONResponse{Success: true, Message: ptrOf(messageLoaded), Data: &data}, nil
}

func (c *Controller) CancelRun(ctx context.Context, req gen.CancelRunRequestObject) (gen.CancelRunResponseObject, error) {
	d, err := c.service.Cancel(ctx, req.Id, req.ProjectId, req.RunId)
	if err != nil {
		return nil, err
	}
	data := detailToGen(d)
	return gen.CancelRun200JSONResponse{Success: true, Message: ptrOf(messageCancelRequested), Data: &data}, nil
}

func (c *Controller) RerunFailedRun(ctx context.Context, req gen.RerunFailedRunRequestObject) (gen.RerunFailedRunResponseObject, error) {
	d, err := c.service.Rerun(ctx, req.Id, req.ProjectId, req.RunId)
	if err != nil {
		return nil, err
	}
	data := detailToGen(d)
	return gen.RerunFailedRun200JSONResponse{Success: true, Message: ptrOf(messageRerun), Data: &data}, nil
}

func (c *Controller) ApproveRun(ctx context.Context, req gen.ApproveRunRequestObject) (gen.ApproveRunResponseObject, error) {
	d, err := c.service.Approve(ctx, req.Id, req.ProjectId, req.RunId)
	if err != nil {
		return nil, err
	}
	data := detailToGen(d)
	return gen.ApproveRun200JSONResponse{Success: true, Message: ptrOf(messageApproved), Data: &data}, nil
}
