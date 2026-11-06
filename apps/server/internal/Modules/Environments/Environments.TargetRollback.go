package environments

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

const (
	releasesListed = 20
	kindDeploy     = "deploy"
)

var healthInputNames = []string{
	"health_check_url", "health_check_status", "health_check_timeout", "health_check_retries", "health_check_interval", "health_check_stable",
	"health_check_start_delay",
}

func (s *Service) rollbackEligible(rel sqlc.DeploymentRelease) bool {
	return rel.Kind == kindDeploy && rel.State == groupSucceeded
}

func (s *Service) RequestTargetRollback(ctx context.Context, org, project uuid.UUID, envName, name, releaseID string, factor auth.Factor) (models.CheckStarted, error) {
	if !ValidName(name) || !releaseIDShape.MatchString(releaseID) {
		return models.CheckStarted{}, apperrors.NotFound()
	}
	if s.synthetic == nil {
		return models.CheckStarted{}, errNoSynthetic
	}
	p, err := principalOf(ctx)
	if err != nil {
		return models.CheckStarted{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
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
	if env.PausedAt != nil {
		return models.CheckStarted{}, apperrors.EnvironmentPaused()
	}
	rel, err := q.GetRelease(ctx, sqlc.GetReleaseParams{TargetID: row.Id, ReleaseID: releaseID, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !s.rollbackEligible(rel)) {
		return models.CheckStarted{}, apperrors.RollbackNotEligible()
	}
	if err != nil {
		return models.CheckStarted{}, err
	}
	health, err := s.healthInputsOf(ctx, q, row.Id, org)
	if err != nil {
		return models.CheckStarted{}, err
	}
	run, err := s.synthetic.StartSynthetic(ctx, deployments.SyntheticRequest{
		OrganizationID: org, ProjectID: project, EnvironmentID: env.Id, EnvironmentName: env.Name, TargetName: row.Name,
		Kind: deployments.SyntheticRollback, ReleaseID: rel.ReleaseId, HealthInputs: health,
	})
	if err != nil {
		return models.CheckStarted{}, err
	}
	actor := p.UserID
	if err := s.bus.Publish(ctx, eventbus.Event{Name: eventbus.DeploymentRollbackRequested, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{org}, ActorUserID: &actor, TargetType: targetCheck, TargetID: row.Id.String(),
		Data: map[string]string{
			"RunId": run.RunID.String(), "ProjectId": project.String(), "Environment": env.Name, "Name": row.Name, "ReleaseId": rel.ReleaseId,
			eventbus.SeverityKey: eventbus.SeverityHigh,
		},
	}}); err != nil && s.log != nil {
		s.log.ErrorContext(ctx, "the audit entry of a rollback of a target could not be written", "run_id", run.RunID.String(), "error", err.Error())
	}
	return models.CheckStarted{RunID: run.RunID, RunNumber: run.Number}, nil
}

func (s *Service) healthInputsOf(ctx context.Context, q *sqlc.Queries, target, org uuid.UUID) (map[string]string, error) {
	raw, err := q.LatestDeployHealthInputs(ctx, sqlc.LatestDeployHealthInputsParams{TargetID: target, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all map[string]string
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, nil
	}
	out := map[string]string{}
	for _, k := range healthInputNames {
		if v := all[k]; v != "" {
			out[k] = v
		}
	}
	return out, nil
}

func releaseView(rel sqlc.DeploymentRelease, hosts []sqlc.DeploymentReleaseHost, kept bool, eligible bool) models.TargetRelease {
	out := models.TargetRelease{
		ID: rel.Id, ReleaseID: rel.ReleaseId, Kind: rel.Kind, State: rel.State, RestoredReleaseID: rel.RestoredReleaseId.String,
		PreviousReleaseID: rel.PreviousReleaseId.String, RollbackReason: rel.RollbackReason.String, Message: rel.Message.String,
		StartedAt: rel.StartedAt, FinishedAt: rel.FinishedAt, RollbackTarget: eligible, ArtifactKept: kept, Hosts: []models.ReleaseHost{},
	}
	for _, h := range hosts {
		out.Hosts = append(out.Hosts, models.ReleaseHost{
			HostID: h.HostId, Position: int(h.Position), State: h.State, RollbackReason: h.RollbackReason.String, Message: h.Message.String,
		})
	}
	return out
}

func currentReleaseOf(rows []sqlc.DeploymentRelease) string {
	for _, r := range rows {
		if r.State != groupSucceeded {
			continue
		}
		if r.Kind == modeRollback && r.RestoredReleaseId.Valid {
			return r.RestoredReleaseId.String
		}
		return r.ReleaseId
	}
	return ""
}

func isPartial(rows []sqlc.DeploymentRelease, checks []models.HostCheck) bool {
	var latest *sqlc.DeploymentRelease
	for i := range rows {
		if rows[i].State != groupRunning {
			latest = &rows[i]
			break
		}
	}
	distinct := map[string]bool{}
	fresh := 0
	for _, c := range checks {
		if !c.Checked || c.CheckedAt == nil || c.CurrentReleaseID == "" {
			continue
		}
		if latest != nil && latest.FinishedAt != nil && !c.CheckedAt.After(*latest.FinishedAt) {
			continue
		}
		distinct[c.CurrentReleaseID] = true
		fresh++
	}
	if len(distinct) > 1 {
		return true
	}
	if latest != nil && slices.Contains([]string{groupRollbackFailed, groupUnknown}, latest.State) {
		return len(distinct) != 1 || fresh != len(checks)
	}
	return false
}

func (s *Service) ListTargetReleases(ctx context.Context, org, project uuid.UUID, envName, name string) (models.TargetReleases, error) {
	check, err := s.LatestTargetCheck(ctx, org, project, envName, name)
	if err != nil {
		return models.TargetReleases{}, err
	}
	target, err := s.GetTarget(ctx, org, project, envName, name)
	if err != nil {
		return models.TargetReleases{}, err
	}
	q := s.queries()
	rows, err := q.ListTargetReleases(ctx, sqlc.ListTargetReleasesParams{TargetID: target.ID, OrganizationID: org, MaxRows: releasesListed})
	if err != nil {
		return models.TargetReleases{}, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Id)
	}
	hostRows, err := q.ListTargetReleaseHosts(ctx, sqlc.ListTargetReleaseHostsParams{TargetID: target.ID, OrganizationID: org, ReleaseRowIds: ids})
	if err != nil {
		return models.TargetReleases{}, err
	}
	byRelease := map[uuid.UUID][]sqlc.DeploymentReleaseHost{}
	for _, h := range hostRows {
		byRelease[h.ReleaseRowId] = append(byRelease[h.ReleaseRowId], h)
	}
	out := models.TargetReleases{CurrentReleaseID: currentReleaseOf(rows), Partial: isPartial(rows, check.Hosts), Releases: make([]models.TargetRelease, 0, len(rows))}
	for _, r := range rows {
		kept := false
		if len(r.ArtifactDigest) == 32 {
			_, err := q.FindRollbackArtifact(ctx, sqlc.FindRollbackArtifactParams{OrganizationID: org, ProjectID: project, Digest: r.ArtifactDigest})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return models.TargetReleases{}, err
			}
			kept = err == nil
		}
		out.Releases = append(out.Releases, releaseView(r, byRelease[r.Id], kept, s.rollbackEligible(r)))
	}
	return out, nil
}
