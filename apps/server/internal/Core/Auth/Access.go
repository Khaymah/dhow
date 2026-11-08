package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

type Access struct {
	guard    *Guard
	subject  policy.Subject
	org      uuid.UUID
	bindings map[uuid.UUID][]policy.Role
}

func (g *Guard) AccessIn(ctx context.Context, org uuid.UUID) (*Access, error) {
	principal, _, ae := g.authenticateWith(ctx, g.resolver.Peek)
	if ae != nil {
		return nil, ae
	}
	if _, member := principal.RoleIn(org); !member {
		return nil, apperrors.NotFound()
	}
	if ae := g.permit(principal.Subject, policy.OrgRead, policy.Resource{OrganizationID: org}); ae != nil {
		return nil, ae
	}
	return &Access{guard: g, subject: principal.Subject, org: org, bindings: map[uuid.UUID][]policy.Role{}}, nil
}

func (a *Access) Can(ctx context.Context, action policy.Action, project uuid.UUID) (bool, error) {
	resource := policy.Resource{OrganizationID: a.org}
	if project != uuid.Nil {
		bindings, err := a.bindingsOf(ctx, project)
		switch {
		case errors.Is(err, ErrNoProject):
			return false, nil
		case err != nil:
			return false, err
		}
		resource.ProjectID, resource.ProjectBindings = project, bindings
	}
	if !policy.Can(a.subject, action, resource) {
		return false, nil
	}
	return !policy.SecondFactorBlocks(a.subject, action, resource, a.guard.now()), nil
}

func (a *Access) bindingsOf(ctx context.Context, project uuid.UUID) ([]policy.Role, error) {
	if known, ok := a.bindings[project]; ok {
		return known, nil
	}
	if a.guard.projects == nil {
		return nil, errNoProjectsResolver
	}
	bindings, err := a.guard.projects.ProjectBindings(ctx, a.org, project, a.subject.UserID)
	if err != nil {
		return nil, err
	}
	a.bindings[project] = bindings
	return bindings, nil
}
