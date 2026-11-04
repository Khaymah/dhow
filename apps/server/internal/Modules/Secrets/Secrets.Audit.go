package secrets

import (
	"context"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

const (
	targetSecret   = "secret"
	targetVariable = "variable"
	targetRegistry = "registry_credential"
)

func scopeData(t models.Target, name string, extra map[string]string) map[string]string {
	data := map[string]string{"Scope": t.Scope}
	if name != "" {
		data["Name"] = name
	}
	if t.ProjectID != nil {
		data["ProjectId"] = t.ProjectID.String()
	}
	if t.EnvironmentID != nil {
		data["EnvironmentId"] = t.EnvironmentID.String()
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func (s *Service) auditSecret(ctx context.Context, q *sqlc.Queries, p auth.Principal, event string, t models.Target, row sqlc.Secret, extra map[string]string) error {
	data := map[string]string{"Version": itoa(row.CurrentVersion)}
	for k, v := range extra {
		data[k] = v
	}
	return s.auditNamed(ctx, q, p, event, targetSecret, row.Id, t, row.Name, data)
}

func (s *Service) auditNamed(ctx context.Context, q *sqlc.Queries, p auth.Principal, event, targetType string, id uuid.UUID, t models.Target, name string, extra map[string]string) error {
	actor := p.UserID
	return s.bus.Publish(ctx, eventbus.Event{Name: event, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{t.OrganizationID}, ActorUserID: &actor, TargetType: targetType, TargetID: id.String(),
		IP: auth.RequestMetaFrom(ctx).IPString(), Data: scopeData(t, name, extra), Tx: q,
	}})
}
