package secrets

import (
	"context"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

var _ secretstore.EnvironmentWriter = (*Service)(nil)

func (s *Service) PutEnvironmentSecret(ctx context.Context, q *sqlc.Queries, w secretstore.EnvironmentSecret) (secretstore.Written, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return secretstore.Written{}, err
	}
	if err := checkName(w.Name); err != nil {
		return secretstore.Written{}, err
	}
	t := models.Target{
		Scope: models.ScopeEnvironment, OrganizationID: w.OrganizationID, ProjectID: &w.ProjectID, EnvironmentID: &w.EnvironmentID,
	}
	in := models.PutSecretInput{Description: &w.Description}
	value := append([]byte(nil), w.Value...)
	res, err := s.writeSecret(ctx, q, p, t, w.Name, in, value)
	if err != nil {
		return secretstore.Written{}, err
	}
	event := eventbus.SecretUpdated
	if res.created {
		event = eventbus.SecretCreated
	}
	if err := s.auditSecret(ctx, q, p, event, t, res.row, nil); err != nil {
		return secretstore.Written{}, err
	}
	return secretstore.Written{Version: res.row.CurrentVersion, Created: res.created}, nil
}
