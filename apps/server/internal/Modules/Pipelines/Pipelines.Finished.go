package pipelines

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
)

func (s *Service) runFinished(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, state, reason string) error {
	if s.bus == nil || state != models.RunFailed {
		return nil
	}
	return s.bus.Publish(ctx, eventbus.Event{Name: eventbus.RunFinished, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{run.OrganizationId}, TargetType: "run", TargetID: run.Id.String(), Tx: q,
		Data: map[string]string{
			"State": state, "Reason": reason, "ProjectId": run.ProjectId.String(), "Number": strconv.FormatInt(run.Number, 10), "Ref": run.Ref,
		},
	}})
}
