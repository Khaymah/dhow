package pipelines

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	triggerCheck       = "check"
	syntheticPath      = "synthetic:check"
	syntheticJobKey    = "check"
	defaultBranchValue = "main"
)

var _ deployments.Synthetic = (*Service)(nil)

func quoted(v string) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return string(raw)
}

func syntheticSource(image, environment, target string) []byte {
	var b strings.Builder
	b.WriteString("version: 1\non:\n  api: {}\njobs:\n  " + syntheticJobKey + ":\n")
	fmt.Fprintf(&b, "    environment: %s\n    steps:\n      - uses: %s\n        with:\n          target: %s\n          %s: %s\n",
		quoted(environment), quoted(image), quoted(target), jobspec.TargetModeInput, jobspec.TargetModeCheck)
	return []byte(b.String())
}

func (s *Service) syntheticRef(ctx context.Context, q *sqlc.Queries, req deployments.SyntheticRequest, project sqlc.GetRunProjectRow) (string, error) {
	ref, err := q.LatestEnvironmentRef(ctx, sqlc.LatestEnvironmentRefParams{EnvironmentID: req.EnvironmentID, OrganizationID: req.OrganizationID})
	if err == nil {
		return ref, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	branch := project.DefaultBranch.String
	if branch == "" {
		branch = defaultBranchValue
	}
	return gitref.PrefixBranch + branch, nil
}

func (s *Service) StartSynthetic(ctx context.Context, req deployments.SyntheticRequest) (deployments.SyntheticRun, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return deployments.SyntheticRun{}, err
	}
	if req.Kind != deployments.SyntheticCheck {
		return deployments.SyntheticRun{}, errors.New("this kind of synthetic run is not supported")
	}
	if s.deployImage == "" {
		return deployments.SyntheticRun{}, apperrors.TargetCheckUnavailable()
	}
	if err := s.requireActiveProject(ctx, req); err != nil {
		return deployments.SyntheticRun{}, err
	}
	source := syntheticSource(s.deployImage, req.EnvironmentName, req.TargetName)
	digest := sha256.Sum256(source)
	var run sqlc.PipelineRun
	existing := false
	err = s.inTx(ctx, func(q *sqlc.Queries) error {
		if err := q.LockCheckRequests(ctx, req.ProjectID.String()+"/"+hex.EncodeToString(digest[:])); err != nil {
			return err
		}
		found, err := q.FindActiveCheckRun(ctx, sqlc.FindActiveCheckRunParams{
			ProjectID: req.ProjectID, OrganizationID: req.OrganizationID, DefinitionSource: text(string(source)),
		})
		if err == nil {
			run, existing = found, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := s.chargeCheck(ctx, req.ProjectID, p.UserID); err != nil {
			return err
		}
		project, err := q.GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: req.ProjectID, OrganizationID: req.OrganizationID})
		if err != nil {
			return err
		}
		ref, err := s.syntheticRef(ctx, q, req, project)
		if err != nil {
			return err
		}
		number, err := q.NextRunNumber(ctx, project.Id)
		if err != nil {
			return err
		}
		run, err = q.InsertSyntheticRun(ctx, sqlc.InsertSyntheticRunParams{
			OrganizationID: req.OrganizationID, ProjectID: project.Id, Number: number, Trigger: triggerCheck, Ref: ref, Actor: text(p.Name),
			DefinitionPath: syntheticPath, DefinitionSource: text(string(source)), DefinitionDigest: digest[:], TriggeredByUserID: &p.UserID,
		})
		return err
	})
	if err != nil {
		return deployments.SyntheticRun{}, err
	}
	if !existing {
		s.auditByUser(ctx, eventbus.RunCreated, run, p)
	}
	return deployments.SyntheticRun{RunID: run.Id, Number: run.Number, Existing: existing}, nil
}

func (s *Service) requireActiveProject(ctx context.Context, req deployments.SyntheticRequest) error {
	project, err := s.queries().GetRunProject(ctx, sqlc.GetRunProjectParams{ProjectID: req.ProjectID, OrganizationID: req.OrganizationID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.NotFound()
	case err != nil:
		return err
	case project.State != projectActive:
		return apperrors.ProjectArchived()
	}
	return nil
}
