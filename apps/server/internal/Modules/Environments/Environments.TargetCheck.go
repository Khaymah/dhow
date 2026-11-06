package environments

import (
	"context"
	"errors"

	"github.com/google/uuid"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

var errNoSynthetic = errors.New("the server has no way to start a run for a check of a target")

func (s *Service) UseSynthetic(p deployments.Synthetic) { s.synthetic = p }

func (s *Service) RequestTargetCheck(ctx context.Context, org, project uuid.UUID, envName, name string) (models.CheckStarted, error) {
	if !ValidName(name) {
		return models.CheckStarted{}, apperrors.NotFound()
	}
	if s.synthetic == nil {
		return models.CheckStarted{}, errNoSynthetic
	}
	p, err := principalOf(ctx)
	if err != nil {
		return models.CheckStarted{}, err
	}
	q := s.queries()
	env, err := s.environmentRow(ctx, q, org, project, envName)
	if err != nil {
		return models.CheckStarted{}, err
	}
	row, err := q.GetDeployTarget(ctx, sqlc.GetDeployTargetParams{EnvironmentID: env.Id, OrganizationID: org, Name: name})
	if err != nil {
		return models.CheckStarted{}, notFoundOr(err)
	}
	run, err := s.synthetic.StartSynthetic(ctx, deployments.SyntheticRequest{
		OrganizationID: org, ProjectID: project, EnvironmentID: env.Id, EnvironmentName: env.Name, TargetName: row.Name, Kind: deployments.SyntheticCheck,
	})
	if err != nil {
		return models.CheckStarted{}, err
	}
	actor := p.UserID
	if err := s.bus.Publish(ctx, eventbus.Event{Name: eventbus.DeploymentTargetCheckRequested, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{org}, ActorUserID: &actor, TargetType: targetCheck, TargetID: row.Id.String(),
		Data: map[string]string{"RunId": run.RunID.String(), "ProjectId": project.String(), "Environment": env.Name, "Name": row.Name},
	}}); err != nil && s.log != nil {
		s.log.ErrorContext(ctx, "the audit entry of a check of a target could not be written", "run_id", run.RunID.String(), "error", err.Error())
	}
	return models.CheckStarted{RunID: run.RunID, RunNumber: run.Number}, nil
}

func (s *Service) LatestTargetCheck(ctx context.Context, org, project uuid.UUID, envName, name string) (models.TargetCheckResult, error) {
	target, err := s.GetTarget(ctx, org, project, envName, name)
	if err != nil {
		return models.TargetCheckResult{}, err
	}
	rows, err := s.queries().ListLatestHostChecks(ctx, sqlc.ListLatestHostChecksParams{TargetID: target.ID, OrganizationID: org})
	if err != nil {
		return models.TargetCheckResult{}, err
	}
	byHost := make(map[uuid.UUID]sqlc.DeploymentHostCheck, len(rows))
	for _, r := range rows {
		byHost[r.HostId] = r
	}
	out := models.TargetCheckResult{Hosts: make([]models.HostCheck, 0, len(target.Hosts))}
	for _, h := range target.Hosts {
		item := models.HostCheck{HostID: h.ID, Position: h.Position, Host: h.Host, Port: h.Port}
		if r, ok := byHost[h.ID]; ok {
			item.Checked, item.Reason, item.Message = true, r.Reason, r.Message.String
			item.CurrentReleaseID, item.VerifiedReleaseID = r.CurrentReleaseId.String, r.VerifiedReleaseId.String
			if r.FreeBytes.Valid {
				v := r.FreeBytes.Int64
				item.FreeBytes = &v
			}
			at := r.CheckedAt
			item.Locked, item.Repaired, item.CheckedAt = r.Locked, r.Repaired, &at
		}
		out.Hosts = append(out.Hosts, item)
	}
	return out, nil
}
