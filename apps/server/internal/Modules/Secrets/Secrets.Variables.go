package secrets

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

func variableOf(row sqlc.Variable, wider map[string][]string) models.Variable {
	return models.Variable{
		Name: row.Name, Scope: row.Scope, Value: row.Value, Overrides: overridesOf(row.Scope, strings.ToLower(row.Name), wider),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, UpdatedByUserID: row.UpdatedByUserId,
	}
}

func (s *Service) widerVariables(ctx context.Context, q *sqlc.Queries, t models.Target) (map[string][]string, error) {
	out := map[string][]string{}
	if t.Scope == models.ScopeOrganization {
		return out, nil
	}
	rows, err := q.VisibleVariableNames(ctx, sqlc.VisibleVariableNamesParams{OrganizationID: t.OrganizationID, ProjectID: t.ProjectID})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Scope == models.ScopeProject && t.Scope == models.ScopeProject {
			continue
		}
		out[r.Scope] = append(out[r.Scope], r.LowerName)
	}
	return out, nil
}

func (s *Service) ListVariables(ctx context.Context, ref models.Ref, page, limit int) (models.Page[models.Variable], error) {
	q := sqlc.New(s.db)
	t, err := s.target(ctx, q, ref, false)
	if err != nil {
		return models.Page[models.Variable]{}, err
	}
	total, err := q.CountVariablesInScope(ctx, sqlc.CountVariablesInScopeParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID,
	})
	if err != nil {
		return models.Page[models.Variable]{}, err
	}
	rows, err := q.ListVariablesInScope(ctx, sqlc.ListVariablesInScopeParams{
		OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID,
		PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return models.Page[models.Variable]{}, err
	}
	wider, err := s.widerVariables(ctx, q, t)
	if err != nil {
		return models.Page[models.Variable]{}, err
	}
	items := make([]models.Variable, 0, len(rows))
	for _, r := range rows {
		items = append(items, variableOf(r, wider))
	}
	return models.Page[models.Variable]{Items: items, Total: total}, nil
}

func (s *Service) PutVariable(ctx context.Context, ref models.Ref, name, value string) (models.Variable, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Variable{}, err
	}
	if err := checkName(name); err != nil {
		return models.Variable{}, err
	}
	if err := checkVariableValue(value); err != nil {
		return models.Variable{}, err
	}
	var out models.Variable
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, err := s.target(ctx, q, ref, true)
		if err != nil {
			return err
		}
		if err := q.LockSecretsScope(ctx, lockKey("variable", t)); err != nil {
			return err
		}
		now := s.now()
		row, err := q.GetVariableForUpdate(ctx, sqlc.GetVariableForUpdateParams{
			OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID, Name: name,
		})
		event := eventbus.VariableUpdated
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			count, err := q.CountVariablesInScope(ctx, sqlc.CountVariablesInScopeParams{
				OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID,
			})
			if err != nil {
				return err
			}
			if count >= config.VariablesPerScope {
				return apperrors.VariableLimitReached()
			}
			row, err = q.InsertVariable(ctx, sqlc.InsertVariableParams{
				OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID, Scope: t.Scope,
				Name: name, Value: value, UserID: p.UserID, Now: now,
			})
			if err != nil {
				return err
			}
			event = eventbus.VariableCreated
		case err != nil:
			return err
		default:
			if row, err = q.UpdateVariable(ctx, sqlc.UpdateVariableParams{
				ID: row.Id, OrganizationID: row.OrganizationId, Value: value, UserID: p.UserID, Now: now,
			}); err != nil {
				return err
			}
		}
		wider, err := s.widerVariables(ctx, q, t)
		if err != nil {
			return err
		}
		out = variableOf(row, wider)
		return s.auditNamed(ctx, q, p, event, targetVariable, row.Id, t, row.Name, nil)
	})
	return out, err
}

func (s *Service) RemoveVariable(ctx context.Context, ref models.Ref, name string) (models.Variable, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Variable{}, err
	}
	if !ValidName(name) {
		return models.Variable{}, apperrors.NotFound()
	}
	var out models.Variable
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		t, err := s.target(ctx, q, ref, true)
		if err != nil {
			return err
		}
		row, err := q.GetVariableForUpdate(ctx, sqlc.GetVariableForUpdateParams{
			OrganizationID: t.OrganizationID, ProjectID: t.ProjectID, EnvironmentID: t.EnvironmentID, Name: name,
		})
		if err != nil {
			return notFoundOr(err)
		}
		wider, err := s.widerVariables(ctx, q, t)
		if err != nil {
			return err
		}
		out = variableOf(row, wider)
		if _, err := q.RemoveVariable(ctx, sqlc.RemoveVariableParams{ID: row.Id, OrganizationID: row.OrganizationId, UserID: p.UserID, Now: s.now()}); err != nil {
			return err
		}
		return s.auditNamed(ctx, q, p, eventbus.VariableRemoved, targetVariable, row.Id, t, row.Name, nil)
	})
	return out, err
}
