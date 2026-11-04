package projects

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Projects/Models"
)

func lockProject(ctx context.Context, q *sqlc.Queries, org, id uuid.UUID) (sqlc.Project, error) {
	row, err := q.LockProject(ctx, sqlc.LockProjectParams{ID: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Project{}, apperrors.NotFound()
	}
	return row, err
}

func projectRole(p auth.Principal, org, id uuid.UUID) policy.Role {
	if p.ProjectID == id && p.ProjectRole != "" {
		return p.ProjectRole
	}
	role, _ := p.RoleIn(org)
	return role
}

type change struct {
	params sqlc.UpdateProjectParams
	fields []string
}

func planChange(row sqlc.Project, in models.UpdateInput) change {
	c := change{params: sqlc.UpdateProjectParams{
		ID: row.Id, OrganizationID: row.OrganizationId, Name: row.Name, PipelinePath: row.PipelinePath, ForkPolicy: row.ForkPolicy,
		ForkCacheRestore: row.ForkCacheRestore,
	}}
	if in.Name != nil && *in.Name != row.Name {
		c.params.Name = *in.Name
		c.fields = append(c.fields, fieldName)
	}
	if in.PipelinePath != nil && *in.PipelinePath != row.PipelinePath {
		c.params.PipelinePath = *in.PipelinePath
		c.fields = append(c.fields, fieldPipelinePath)
	}
	if in.ForkPolicy != nil && *in.ForkPolicy != row.ForkPolicy {
		c.params.ForkPolicy = *in.ForkPolicy
		c.fields = append(c.fields, fieldForkPolicy)
	}
	if in.ForkCache != nil && *in.ForkCache != row.ForkCacheRestore {
		c.params.ForkCacheRestore = *in.ForkCache
		c.fields = append(c.fields, fieldForkCache)
	}
	return c
}

func (s *Service) Update(ctx context.Context, org, id uuid.UUID, in models.UpdateInput) (models.Project, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Project{}, err
	}
	if ae := validateUpdate(in); ae != nil {
		return models.Project{}, ae
	}
	var row sqlc.Project
	var c change
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		current, err := lockProject(ctx, q, org, id)
		if err != nil {
			return err
		}
		if current.State != models.StateActive {
			return apperrors.ProjectArchived()
		}
		row, c = current, planChange(current, in)
		if len(c.fields) == 0 {
			return nil
		}
		if c.params.ForkPolicy == models.ForkPolicyAutomatic && current.ForkPolicy != models.ForkPolicyAutomatic &&
			!policy.AtLeast(projectRole(p, org, id), policy.RoleAdmin) {
			return apperrors.ForkPolicyNotAllowed()
		}
		if c.params.ForkCacheRestore == models.ForkCacheDefault && current.ForkCacheRestore != models.ForkCacheDefault &&
			!policy.AtLeast(projectRole(p, org, id), policy.RoleAdmin) {
			return apperrors.ForkCacheNotAllowed()
		}
		c.params.Now = s.now()
		row, err = q.UpdateProject(ctx, c.params)
		return err
	})
	if err != nil {
		return models.Project{}, err
	}
	project := projectOf(row)
	if len(c.fields) == 0 {
		return project, nil
	}
	return project, s.audit(ctx, p, eventbus.ProjectUpdated, project, map[string]string{
		"Fields": strings.Join(c.fields, ","), "ForkPolicy": project.ForkPolicy, "ForkCacheRestore": project.ForkCache,
	})
}

func (s *Service) Archive(ctx context.Context, org, id uuid.UUID) (models.Project, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Project{}, err
	}
	var row sqlc.Project
	changed := false
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		current, err := lockProject(ctx, q, org, id)
		if err != nil {
			return err
		}
		row = current
		if current.State != models.StateActive {
			return nil
		}
		row, err = q.ArchiveProject(ctx, sqlc.ArchiveProjectParams{ID: id, OrganizationID: org, Now: s.now()})
		changed = err == nil
		return err
	})
	if err != nil {
		return models.Project{}, err
	}
	project := projectOf(row)
	if !changed {
		return project, nil
	}
	return project, s.audit(ctx, p, eventbus.ProjectArchived, project, map[string]string{"Slug": project.Slug})
}
