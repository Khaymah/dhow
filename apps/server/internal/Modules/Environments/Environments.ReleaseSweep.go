package environments

import (
	"context"
	"time"

	"github.com/google/uuid"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

func (s *Service) markUnknown(ctx context.Context, q *sqlc.Queries, rel sqlc.DeploymentRelease, now time.Time) error {
	if _, err := q.MarkReleaseHostsUnknown(ctx, sqlc.MarkReleaseHostsUnknownParams{Now: &now, ReleaseRowID: rel.Id, OrganizationID: rel.OrganizationId}); err != nil {
		return err
	}
	if err := q.FinishRelease(ctx, sqlc.FinishReleaseParams{
		State: groupUnknown, FailedHostID: rel.FailedHostId, Now: &now, ID: rel.Id, OrganizationID: rel.OrganizationId,
	}); err != nil {
		return err
	}
	return s.bus.Publish(ctx, eventbus.Event{Name: eventbus.DeploymentReleaseUnknown, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{rel.OrganizationId}, TargetType: targetRelease, TargetID: rel.Id.String(),
		Data: map[string]string{
			"TargetId": rel.TargetId.String(), "ReleaseId": rel.ReleaseId, "DeploymentId": rel.DeploymentId.String(),
			eventbus.SeverityKey: eventbus.SeverityHigh,
		},
		Tx: q,
	}})
}

func (s *Service) SettleReleases(ctx context.Context, q *sqlc.Queries, org, run uuid.UUID) (int64, error) {
	rows, err := q.LockRunningReleasesOfRun(ctx, sqlc.LockRunningReleasesOfRunParams{RunID: run, OrganizationID: org})
	if err != nil {
		return 0, err
	}
	return s.retire(ctx, q, rows)
}

func (s *Service) SweepReleases(ctx context.Context, q *sqlc.Queries) (int64, error) {
	rows, err := q.LockStaleRunningReleases(ctx)
	if err != nil {
		return 0, err
	}
	return s.retire(ctx, q, rows)
}

func (s *Service) retire(ctx context.Context, q *sqlc.Queries, rows []sqlc.DeploymentRelease) (int64, error) {
	now := s.now()
	for _, rel := range rows {
		if err := s.markUnknown(ctx, q, rel, now); err != nil {
			return 0, err
		}
	}
	return int64(len(rows)), nil
}
