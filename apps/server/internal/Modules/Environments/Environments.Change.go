package environments

import (
	"context"
	"encoding/json"
	"reflect"
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

type reviewerView struct {
	UserIds []uuid.UUID `json:"UserIds"`
	TeamIds []uuid.UUID `json:"TeamIds"`
}

type diff struct {
	fields []string
	from   map[string]any
	to     map[string]any
	rules  bool
}

func (d *diff) add(field string, before, after any, rule bool) {
	if reflect.DeepEqual(before, after) {
		return
	}
	d.fields = append(d.fields, field)
	d.from[field], d.to[field] = before, after
	d.rules = d.rules || rule
}

func compare(old, updated models.Environment) diff {
	d := diff{from: map[string]any{}, to: map[string]any{}}
	o, n := old.Rules, updated.Rules
	d.add(fieldDescription, old.Description, updated.Description, false)
	d.add(ruleAllowedRefs, o.AllowedRefs, n.AllowedRefs, true)
	d.add(ruleAllowPullRequests, o.AllowPullRequests, n.AllowPullRequests, true)
	d.add(ruleReviewers, reviewerView{o.Reviewers.UserIDs, o.Reviewers.TeamIDs}, reviewerView{n.Reviewers.UserIDs, n.Reviewers.TeamIDs}, true)
	d.add(ruleRequiredApprovals, o.RequiredApprovals, n.RequiredApprovals, true)
	d.add(ruleAllowSelfApproval, o.AllowSelfApproval, n.AllowSelfApproval, true)
	d.add(ruleWaitMinutes, o.WaitMinutes, n.WaitMinutes, true)
	d.add(ruleApprovalTimeoutHours, o.ApprovalTimeoutHours, n.ApprovalTimeoutHours, true)
	d.add(ruleRequirePinnedImages, o.RequirePinnedImages, n.RequirePinnedImages, true)
	d.add(ruleAllowedPlugins, o.AllowedPlugins, n.AllowedPlugins, true)
	d.add(ruleRequiredIsolation, o.RequiredIsolation, n.RequiredIsolation, true)
	d.add(ruleAllowAdminBypass, o.AllowAdminBypass, n.AllowAdminBypass, true)
	d.add(ruleSupersedePending, o.SupersedePending, n.SupersedePending, true)
	return d
}

func compact(v map[string]any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

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
		reviewersChanged := d.to[ruleReviewers] != nil
		if reviewersChanged {
			if err := checkReviewers(ctx, q, org, project, next.Rules.Reviewers); err != nil {
				return err
			}
		}
		out, err = s.save(ctx, q, row, next, d.rules, reviewersChanged)
		return err
	})
	if err != nil || len(d.fields) == 0 {
		return out, err
	}
	return out, s.audit(ctx, p, eventbus.EnvironmentUpdated, out, map[string]string{
		"Fields": strings.Join(d.fields, ","), "From": compact(d.from), "To": compact(d.to), "Version": versionOf(out),
	})
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
		AllowAdminBypass: r.AllowAdminBypass, SupersedePending: r.SupersedePending, RulesChanged: rules, Now: now,
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
		return q.RemoveEnvironmentReviewers(ctx, sqlc.RemoveEnvironmentReviewersParams{OrganizationID: org, EnvironmentID: row.Id, Now: now})
	})
	if err != nil {
		return models.Environment{}, err
	}
	return out, s.audit(ctx, p, eventbus.EnvironmentRemoved, out, map[string]string{"Version": versionOf(out)})
}

func (s *Service) Pause(ctx context.Context, org, project uuid.UUID, name, reason string) (models.Environment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Environment{}, err
	}
	if ae := validateReason(reason); ae != nil {
		return models.Environment{}, ae
	}
	changed := false
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
		changed = true
		out, err = s.one(ctx, q, paused)
		return err
	})
	if err != nil || !changed {
		return out, err
	}
	return out, s.audit(ctx, p, eventbus.EnvironmentPaused, out, map[string]string{"Reason": reason, "Version": versionOf(out)})
}

func (s *Service) Resume(ctx context.Context, org, project uuid.UUID, name string, factor auth.Factor) (models.Environment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Environment{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Environment{}, err
	}
	changed := false
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
		changed = true
		out, err = s.one(ctx, q, resumed)
		return err
	})
	if err != nil || !changed {
		return out, err
	}
	return out, s.audit(ctx, p, eventbus.EnvironmentResumed, out, map[string]string{"Version": versionOf(out)})
}
