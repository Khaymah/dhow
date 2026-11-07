package ingest

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const reasonCreatorRemoved = "creator-removed"

type Subscriber interface {
	Subscribe(name string, h eventbus.Handler)
}

func Subscribe(bus Subscriber, s *Service) {
	bus.Subscribe(eventbus.IdentityMemberRemoved, s.memberGone)
	bus.Subscribe(eventbus.IdentityMemberLeft, s.memberGone)
}

func rolesOf(raw []string) []policy.Role {
	out := make([]policy.Role, 0, len(raw))
	for _, r := range raw {
		if role, ok := policy.ParseRole(r); ok {
			out = append(out, role)
		}
	}
	return out
}

func (s *Service) creatorsAllowed(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, users []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(users))
	if len(users) == 0 {
		return out, nil
	}
	rows, err := q.ListIngestCreatorAccess(ctx, sqlc.ListIngestCreatorAccessParams{OrganizationID: org, ProjectID: project, UserIds: users})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		role, ok := policy.EffectiveOf(r.Role, r.TeamRoles)
		if !ok {
			continue
		}
		out[r.UserId] = policy.Allows(policy.ProjectRole(role, rolesOf(r.Bindings)), policy.IngestManage)
	}
	return out, nil
}

func (s *Service) creatorAllowed(ctx context.Context, q *sqlc.Queries, org, project, user uuid.UUID) (bool, error) {
	allowed, err := s.creatorsAllowed(ctx, q, org, project, []uuid.UUID{user})
	return allowed[user], err
}

func (s *Service) memberGone(ctx context.Context, event eventbus.Event) error {
	payload, ok := event.Payload.(eventbus.Audited)
	if !ok || len(payload.OrganizationIDs) != 1 {
		return fmt.Errorf("ingest: the %s event has no single organization", event.Name)
	}
	user, err := uuid.Parse(payload.TargetID)
	if err != nil {
		return fmt.Errorf("ingest: the user id %q of the %s event: %w", payload.TargetID, event.Name, err)
	}
	actor := user
	if payload.ActorUserID != nil {
		actor = *payload.ActorUserID
	}
	return s.RevokeCreatorTokens(ctx, payload.OrganizationIDs[0], user, actor, payload.IP)
}

func (s *Service) RevokeCreatorTokens(ctx context.Context, org, user, actor uuid.UUID, ip string) error {
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		revoked, err := q.RevokeIngestTokensOfCreator(ctx, sqlc.RevokeIngestTokensOfCreatorParams{
			OrganizationID: org, UserID: user, ActorID: actor, Now: s.now(),
		})
		if err != nil {
			return err
		}
		for _, t := range revoked {
			err := s.audit(ctx, auditEntry{
				name: eventbus.IngestTokenRevoked, org: org, actor: &actor, ip: ip, target: targetIngestToken, targetID: t.Id, tx: q,
				data: map[string]string{"Name": t.Name, "ProjectId": t.ProjectId.String(), "Reason": reasonCreatorRemoved},
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("revoke the ingest tokens of a removed member: %w", err)
	}
	return nil
}
