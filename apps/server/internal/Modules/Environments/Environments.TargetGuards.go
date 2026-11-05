package environments

import (
	"context"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

func (s *Service) checkProject(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID) error {
	proj, err := q.GetEnvironmentProject(ctx, sqlc.GetEnvironmentProjectParams{ProjectID: project, OrganizationID: org})
	if err != nil {
		return notFoundOr(err)
	}
	if proj.State == projectArchived {
		return apperrors.ProjectArchived()
	}
	return nil
}

func (s *Service) checkKeyNameFree(ctx context.Context, q *sqlc.Queries, env models.Environment, name string) error {
	rows, err := q.ListDeployTargets(ctx, sqlc.ListDeployTargetsParams{EnvironmentID: env.ID, OrganizationID: env.OrganizationID})
	if err != nil {
		return err
	}
	derived := KeySecretName(name)
	for _, r := range rows {
		if r.Name != name && KeySecretName(r.Name) == derived {
			return apperrors.TargetDuplicate()
		}
	}
	return nil
}
