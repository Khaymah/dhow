package environments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/pathglob"
)

var ErrRuleChanged = errors.New("the environment or its rules changed after the job passed the gate")

type Gate struct{ s *Service }

func (s *Service) Gate() *Gate { return &Gate{s: s} }

func decodeRefs(raw []byte, id uuid.UUID) ([]models.Ref, error) {
	refs := []models.Ref{}
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, fmt.Errorf("decode the allowed refs of environment %s: %w", id, err)
	}
	return refs, nil
}

func RefAllowed(refs []models.Ref, allowPullRequests bool, kind, name string, pullRequest bool) bool {
	if pullRequest {
		if !allowPullRequests {
			return false
		}
		kind = deployments.RefBranch
	}
	if name == "" || (kind != models.RefBranch && kind != models.RefTag) {
		return false
	}
	for _, ref := range refs {
		if ref.Kind == kind && pathglob.MatchPattern(ref.Pattern, name) {
			return true
		}
	}
	return false
}

func (g *Gate) Lock(ctx context.Context, q *sqlc.Queries, t deployments.Target) (deployments.Environment, bool, error) {
	row, err := q.LockGateEnvironment(ctx, sqlc.LockGateEnvironmentParams{ProjectID: t.ProjectID, OrganizationID: t.OrganizationID, Name: t.Name})
	if errors.Is(err, pgx.ErrNoRows) {
		return deployments.Environment{}, false, nil
	}
	if err != nil {
		return deployments.Environment{}, false, err
	}
	refs, err := decodeRefs(row.AllowedRefs, row.Id)
	if err != nil {
		return deployments.Environment{}, false, err
	}
	return deployments.Environment{
		ID: row.Id, Name: row.Name, RuleVersion: row.RuleVersion,
		RefAllowed:   RefAllowed(refs, row.AllowPullRequests, t.RefKind, t.RefName, t.PullRequest),
		HasReviewers: row.HasReviewers, WaitMinutes: int(row.WaitMinutes), RequirePinnedImages: row.RequirePinnedImages,
		AllowedPlugins: row.AllowedPlugins, Paused: row.Paused, LockedByRunID: row.LockedByRunId,
	}, true, nil
}

func (g *Gate) Started(ctx context.Context, q *sqlc.Queries, d deployments.Deployment) error {
	now := g.s.now()
	if err := q.HoldEnvironment(ctx, sqlc.HoldEnvironmentParams{
		RunID: &d.RunID, Now: &now, ID: d.EnvironmentID, OrganizationID: d.OrganizationID,
	}); err != nil {
		return err
	}
	sha := pgtype.Text{String: d.Sha, Valid: d.Sha != ""}
	return q.InsertDeployment(ctx, sqlc.InsertDeploymentParams{
		OrganizationID: d.OrganizationID, EnvironmentID: d.EnvironmentID, RunID: d.RunID, RunNumber: d.RunNumber,
		RunJobID: d.RunJobID, JobID: d.JobID, JobKey: d.JobKey, Ref: d.Ref, Sha: sha, RuleVersion: d.RuleVersion, Now: now,
	})
}

func (g *Gate) Settle(ctx context.Context, q *sqlc.Queries, org, run uuid.UUID, keep []string) error {
	now := g.s.now()
	if err := q.SettleDeployments(ctx, sqlc.SettleDeploymentsParams{Now: &now, RunID: run, OrganizationID: org}); err != nil {
		return err
	}
	if keep == nil {
		keep = []string{}
	}
	return q.ReleaseEnvironments(ctx, sqlc.ReleaseEnvironmentsParams{RunID: &run, OrganizationID: org, Keep: keep})
}

func (g *Gate) ReleaseStale(ctx context.Context, q *sqlc.Queries) (int64, error) {
	return q.ReleaseStaleEnvironments(ctx)
}

type StartCheck struct {
	OrganizationID uuid.UUID
	RunJobID       uuid.UUID
	JobID          uuid.UUID
	RefKind        string
	RefName        string
	PullRequest    bool
}

func (s *Service) CheckStart(ctx context.Context, c StartCheck) (uuid.UUID, error) {
	var env uuid.UUID
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		row, err := q.GetStartEnvironment(ctx, sqlc.GetStartEnvironmentParams{RunJobID: c.RunJobID, JobID: c.JobID, OrganizationID: c.OrganizationID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRuleChanged
		}
		if err != nil {
			return err
		}
		refs, err := decodeRefs(row.AllowedRefs, row.Id)
		if err != nil {
			return err
		}
		if row.Paused || row.RuleVersion != row.DeployedRuleVersion || !RefAllowed(refs, row.AllowPullRequests, c.RefKind, c.RefName, c.PullRequest) {
			return ErrRuleChanged
		}
		env = row.Id
		return nil
	})
	return env, err
}
