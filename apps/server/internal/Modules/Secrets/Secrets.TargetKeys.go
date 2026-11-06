package secrets

import (
	"context"
	"errors"
	"slices"
	"strings"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

var errTargetKeyGuardMissing = errors.New("secrets: the deploy target guard is not wired, so a key secret cannot be told from another secret")

func (s *Service) UseTargetKeyGuard(targets deployments.Targets, factors auth.FactorConfirmer) {
	s.targets, s.factors = targets, factors
}

func (s *Service) guardTargetKey(ctx context.Context, q *sqlc.Queries, p auth.Principal, t models.Target, name string, factor *auth.Factor) error {
	if t.Scope != models.ScopeEnvironment || t.EnvironmentID == nil {
		return nil
	}
	if s.targets == nil || s.factors == nil {
		return errTargetKeyGuardMissing
	}
	names, err := s.targets.KeyNames(ctx, q, t.OrganizationID, *t.EnvironmentID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(names, func(key string) bool { return strings.EqualFold(key, name) }) {
		return nil
	}
	if factor == nil || factor.Method == "" {
		return apperrors.TargetKeyNeedsFactor()
	}
	return s.factors.ConfirmFactor(ctx, p, *factor)
}
