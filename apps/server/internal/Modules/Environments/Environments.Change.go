package environments

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
)

type diff struct {
	fields []string
	rules  bool
}

func (d *diff) add(field string, changed, rule bool) {
	if changed {
		d.fields = append(d.fields, field)
		d.rules = d.rules || rule
	}
}

func compare(old, updated models.Environment) diff {
	d := diff{}
	o, n := old.Rules, updated.Rules
	d.add(fieldDescription, old.Description != updated.Description, false)
	d.add(ruleAllowedRefs, !reflect.DeepEqual(o.AllowedRefs, n.AllowedRefs), true)
	d.add(ruleAllowPullRequests, o.AllowPullRequests != n.AllowPullRequests, true)
	d.add(ruleReviewers, !reflect.DeepEqual(o.Reviewers, n.Reviewers), true)
	d.add(ruleRequiredApprovals, o.RequiredApprovals != n.RequiredApprovals, true)
	d.add(ruleAllowSelfApproval, o.AllowSelfApproval != n.AllowSelfApproval, true)
	d.add(ruleWaitMinutes, o.WaitMinutes != n.WaitMinutes, true)
	d.add(ruleApprovalTimeoutHours, o.ApprovalTimeoutHours != n.ApprovalTimeoutHours, true)
	d.add(ruleRequirePinnedImages, o.RequirePinnedImages != n.RequirePinnedImages, true)
	d.add(ruleAllowedPlugins, !reflect.DeepEqual(o.AllowedPlugins, n.AllowedPlugins), true)
	d.add(ruleRequiredIsolation, o.RequiredIsolation != n.RequiredIsolation, true)
	d.add(ruleAllowAdminBypass, o.AllowAdminBypass != n.AllowAdminBypass, true)
	d.add(ruleSupersedePending, o.SupersedePending != n.SupersedePending, true)
	d.add(ruleAllowExtraEgress, o.AllowExtraDeployEgress != n.AllowExtraDeployEgress, true)
	return d
}

func (d diff) has(field string) bool { return slices.Contains(d.fields, field) }

func (s *Service) lock(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID, name string) (models.Environment, sqlc.Environment, error) {
	if !ValidName(name) {
		return models.Environment{}, sqlc.Environment{}, apperrors.NotFound()
	}
	row, err := q.LockEnvironment(ctx, sqlc.LockEnvironmentParams{ProjectID: project, OrganizationID: org, Name: name})
	if err != nil {
		return models.Environment{}, sqlc.Environment{}, notFoundOr(err)
	}
	e, err := s.one(ctx, q, row)
	return e, row, err
}

func (s *Service) confirm(ctx context.Context, p auth.Principal, factor auth.Factor) error {
	return s.factors.ConfirmFactor(ctx, p, factor)
}

func (s *Service) Update(ctx context.Context, org, project uuid.UUID, name string, in models.UpdateInput, factor auth.Factor) (models.Environment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Environment{}, err
	}
	if in.Description == nil && in.Rules == nil {
		return models.Environment{}, apperrors.BadRequest()
	}
	if ae := validateInput(nil, in.Description, in.Rules); ae != nil {
		return models.Environment{}, ae
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Environment{}, err
	}
	var out models.Environment
	var d diff
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		current, row, err := s.lock(ctx, q, org, project, name)
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return apperrors.EnvironmentVersionConflict()
		}
		next := current
		next.Rules = Apply(current.Rules, in.Rules)
		if in.Description != nil {
			next.Description = *in.Description
		}
		if details := ValidateRules(next.Rules); len(details) > 0 {
			return apperrors.ValidationFailed(details...)
		}
		out, d = current, compare(current, next)
		if len(d.fields) == 0 {
			return nil
		}
		if d.has(ruleReviewers) {
			if err := checkReviewers(ctx, q, org, project, next.Rules.Reviewers); err != nil {
				return err
			}
		}
		if out, err = s.save(ctx, q, row, next, d.rules, d.has(ruleReviewers)); err != nil {
			return err
		}
		data := auditData{"Fields": strings.Join(d.fields, ","), "Version": versionOf(out)}
		data.rules(current.Rules, out.Rules, false)
		return s.audit(ctx, q, p, eventbus.EnvironmentUpdated, out, data)
	})
	return out, err
}

func (s *Service) save(ctx context.Context, q *sqlc.Queries, row sqlc.Environment, next models.Environment, rules, reviewers bool) (models.Environment, error) {
	refs, err := json.Marshal(next.Rules.AllowedRefs)
	if err != nil {
		return models.Environment{}, err
	}
	r, now := next.Rules, s.now()
	updated, err := q.UpdateEnvironmentRules(ctx, sqlc.UpdateEnvironmentRulesParams{
		ID: row.Id, OrganizationID: row.OrganizationId, Description: next.Description, AllowedRefs: refs,
		AllowPullRequests: r.AllowPullRequests, RequiredApprovals: int32(r.RequiredApprovals), //nolint:gosec // RequiredApprovals is validated to be 1.
		AllowSelfApproval: r.AllowSelfApproval, WaitMinutes: int32(r.WaitMinutes), //nolint:gosec // WaitMinutes is validated to fit.
		ApprovalTimeoutHours: int32(r.ApprovalTimeoutHours), //nolint:gosec // ApprovalTimeoutHours is validated to fit.
		RequirePinnedImages:  r.RequirePinnedImages, AllowedPlugins: r.AllowedPlugins, RequiredIsolation: r.RequiredIsolation,
		AllowAdminBypass: r.AllowAdminBypass, SupersedePending: r.SupersedePending, AllowExtraDeployEgress: r.AllowExtraDeployEgress,
		RulesChanged: rules, Now: now,
	})
	if err != nil {
		return models.Environment{}, err
	}
	if reviewers {
		if err := q.RemoveEnvironmentReviewers(ctx, sqlc.RemoveEnvironmentReviewersParams{
			OrganizationID: row.OrganizationId, EnvironmentID: row.Id, Now: now,
		}); err != nil {
			return models.Environment{}, err
		}
		if err := insertReviewers(ctx, q, row.OrganizationId, row.Id, r.Reviewers, now); err != nil {
			return models.Environment{}, err
		}
	}
	return s.one(ctx, q, updated)
}

func (s *Service) Remove(ctx context.Context, org, project uuid.UUID, name string, factor auth.Factor) (models.Environment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Environment{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Environment{}, err
	}
	var out models.Environment
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		current, row, err := s.lock(ctx, q, org, project, name)
		if err != nil {
			return err
		}
		now := s.now()
		if _, err := q.RemoveEnvironment(ctx, sqlc.RemoveEnvironmentParams{ID: row.Id, OrganizationID: org, Now: now}); err != nil {
			return err
		}
		out = current
		if err := q.RemoveEnvironmentReviewers(ctx, sqlc.RemoveEnvironmentReviewersParams{OrganizationID: org, EnvironmentID: row.Id, Now: now}); err != nil {
			return err
		}
		if err := s.removeTargetsOf(ctx, q, org, row.Id, now); err != nil {
			return err
		}
		return s.audit(ctx, q, p, eventbus.EnvironmentRemoved, out, auditData{"Version": versionOf(out)})
	})
	if err != nil {
		return models.Environment{}, err
	}
	return out, nil
}

func (s *Service) Pause(ctx context.Context, org, project uuid.UUID, name, reason string) (models.Environment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Environment{}, err
	}
	if ae := validateReason(reason); ae != nil {
		return models.Environment{}, ae
	}
	var out models.Environment
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		current, row, err := s.lock(ctx, q, org, project, name)
		if err != nil || current.PausedAt != nil {
			out = current
			return err
		}
		actor := p.UserID
		paused, err := q.PauseEnvironment(ctx, sqlc.PauseEnvironmentParams{
			ID: row.Id, OrganizationID: org, PausedByUserID: &actor, PausedReason: pgtype.Text{String: reason, Valid: true}, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if out, err = s.one(ctx, q, paused); err != nil {
			return err
		}
		data := auditData{"Version": versionOf(out)}
		data.reason(reason)
		return s.audit(ctx, q, p, eventbus.EnvironmentPaused, out, data)
	})
	return out, err
}

func (s *Service) Resume(ctx context.Context, org, project uuid.UUID, name string, factor auth.Factor) (models.Environment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Environment{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Environment{}, err
	}
	var out models.Environment
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		current, row, err := s.lock(ctx, q, org, project, name)
		if err != nil || current.PausedAt == nil {
			out = current
			return err
		}
		resumed, err := q.ResumeEnvironment(ctx, sqlc.ResumeEnvironmentParams{ID: row.Id, OrganizationID: org, Now: s.now()})
		if err != nil {
			return err
		}
		if out, err = s.one(ctx, q, resumed); err != nil {
			return err
		}
		return s.audit(ctx, q, p, eventbus.EnvironmentResumed, out, auditData{"Version": versionOf(out)})
	})
	return out, err
}
