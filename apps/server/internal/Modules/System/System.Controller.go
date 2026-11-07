package system

import (
	"context"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Models"
)

const (
	messageLive  = "system.health.live"
	messageReady = "system.health.ready"
	messageMeta  = "system.meta.loaded"
)

type Controller struct {
	health *HealthService
	meta   *MetaService
	keys   KeyringReader
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(health *HealthService, meta *MetaService) *Controller {
	return &Controller{health: health, meta: meta}
}

func (c *Controller) GetLiveness(context.Context, gen.GetLivenessRequestObject) (gen.GetLivenessResponseObject, error) {
	data := healthToGen(c.health.Liveness())
	return gen.GetLiveness200JSONResponse{Success: true, Message: ptr(messageLive), Data: &data}, nil
}

func (c *Controller) GetReadiness(ctx context.Context, _ gen.GetReadinessRequestObject) (gen.GetReadinessResponseObject, error) {
	status := c.health.Readiness(ctx)
	data := healthToGen(status)
	if !status.Ready() {
		return gen.GetReadiness503JSONResponse{
			Success: false,
			Data:    &data,
			Error:   errorToGen(response.Localize(ctx, apperrors.ServiceUnavailable())),
		}, nil
	}
	return gen.GetReadiness200JSONResponse{Success: true, Message: ptr(messageReady), Data: &data}, nil
}

func (c *Controller) GetMeta(ctx context.Context, _ gen.GetMetaRequestObject) (gen.GetMetaResponseObject, error) {
	meta, err := c.meta.Get(ctx)
	if err != nil {
		return nil, err
	}
	data := gen.Meta{Version: meta.Version, Commit: meta.Commit, SetupRequired: meta.SetupRequired}
	return gen.GetMeta200JSONResponse{Success: true, Message: ptr(messageMeta), Data: &data}, nil
}

func healthToGen(h models.HealthStatus) gen.HealthStatus {
	checks := make([]gen.HealthCheck, 0, len(h.Checks))
	for _, c := range h.Checks {
		checks = append(checks, gen.HealthCheck{Name: gen.HealthCheckName(c.Name), Ok: c.Ok})
	}
	return gen.HealthStatus{Status: gen.HealthStatusStatus(h.Status), Checks: checks}
}

func errorToGen(ae *apperrors.AppError) *gen.ErrorDetail {
	detail := &gen.ErrorDetail{Code: ae.Code, Message: ae.Message}
	if ae.Fix != "" {
		detail.Fix = &ae.Fix
	}
	if ae.DocsPath != "" {
		detail.DocsPath = &ae.DocsPath
	}
	if len(ae.Details) > 0 {
		fields := make([]gen.ErrorFieldDetail, 0, len(ae.Details))
		for _, d := range ae.Details {
			fields = append(fields, gen.ErrorFieldDetail{Field: d.Field, Message: d.Message})
		}
		detail.Details = &fields
	}
	return detail
}

func ptr[T any](v T) *T { return &v }
