package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

var (
	ErrNoProject          = errors.New("no such project in the organization")
	errNoProjectsResolver = errors.New("a project route has no project resolver")
)

type ProjectResolver interface {
	ProjectBindings(ctx context.Context, org, project, user uuid.UUID) ([]policy.Role, error)
}

func scopedToOrganization(rule policy.Rule) bool {
	return rule.Scope == policy.ScopeOrganization || rule.Scope == policy.ScopeProject
}

func (g *Guard) projectOf(ctx context.Context, r *http.Request, rule policy.Rule, resource policy.Resource, user uuid.UUID) (policy.Resource, *apperrors.AppError) {
	if rule.Scope != policy.ScopeProject {
		return resource, nil
	}
	if g.projects == nil {
		return resource, apperrors.Internal(errNoProjectsResolver)
	}
	id, err := uuid.Parse(chi.URLParam(r, rule.ProjectParam))
	if err != nil || id == uuid.Nil {
		return resource, apperrors.NotFound()
	}
	bindings, err := g.projects.ProjectBindings(ctx, resource.OrganizationID, id, user)
	switch {
	case errors.Is(err, ErrNoProject):
		return resource, apperrors.NotFound()
	case err != nil:
		return resource, apperrors.Internal(err)
	}
	resource.ProjectID = id
	resource.ProjectBindings = bindings
	return resource, nil
}

func withProjectRole(p Principal, resource policy.Resource) Principal {
	if resource.ProjectID == uuid.Nil {
		return p
	}
	role, _ := p.RoleIn(resource.OrganizationID)
	p.ProjectID = resource.ProjectID
	p.ProjectRole = policy.ProjectRole(role, resource.ProjectBindings)
	return p
}

func (g *Guard) AuthorizeProject(ctx context.Context, action policy.Action, org, project uuid.UUID) error {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return apperrors.Unauthenticated()
	}
	role, member := p.RoleIn(org)
	if !member {
		return apperrors.NotFound()
	}
	if g.projects == nil {
		return apperrors.Internal(errNoProjectsResolver)
	}
	bindings, err := g.projects.ProjectBindings(ctx, org, project, p.UserID)
	switch {
	case errors.Is(err, ErrNoProject):
		return apperrors.Forbidden()
	case err != nil:
		return apperrors.Internal(err)
	}
	if !policy.Allows(policy.ProjectRole(role, bindings), action) {
		return apperrors.Forbidden()
	}
	return nil
}
