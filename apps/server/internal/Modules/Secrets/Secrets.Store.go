package secrets

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

var _ secretstore.Store = (*Service)(nil)

func scopeRank(scope string) int {
	switch scope {
	case models.ScopeEnvironment:
		return 3
	case models.ScopeProject:
		return 2
	default:
		return 1
	}
}

type selection struct {
	names  []string
	chosen map[string]sqlc.ResolveSecretsRow
}

func requestedNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func (s *Service) choose(ctx context.Context, q *sqlc.Queries, r secretstore.Request) (selection, error) {
	if r.Fork {
		return selection{}, secretstore.ErrFork
	}
	names := requestedNames(r.Names)
	if len(names) > config.SecretsPerJob {
		return selection{}, secretstore.ErrTooMany
	}
	sel := selection{names: names, chosen: map[string]sqlc.ResolveSecretsRow{}}
	if len(names) == 0 {
		return sel, nil
	}
	lower := make([]string, 0, len(names))
	for _, n := range names {
		lower = append(lower, strings.ToLower(n))
	}
	rows, err := q.ResolveSecrets(ctx, sqlc.ResolveSecretsParams{
		OrganizationID: r.OrganizationID, LowerNames: lower, ProjectID: r.ProjectID, EnvironmentID: r.EnvironmentID,
	})
	if err != nil {
		return selection{}, err
	}
	for _, row := range rows {
		key := strings.ToLower(row.Name)
		if best, ok := sel.chosen[key]; !ok || scopeRank(row.Scope) > scopeRank(best.Scope) {
			sel.chosen[key] = row
		}
	}
	return sel, nil
}

func (sel selection) check(r secretstore.Request) (secretstore.CheckResult, error) {
	out := secretstore.CheckResult{}
	total := 0
	for _, n := range sel.names {
		row, ok := sel.chosen[strings.ToLower(n)]
		switch {
		case !ok:
			out.Missing = append(out.Missing, n)
		case r.PullRequest && row.Scope != models.ScopeEnvironment && !row.AvailableToPullRequests:
			out.NotForPullRequests = append(out.NotForPullRequests, n)
		default:
			total += int(row.SizeBytes)
		}
	}
	if total > config.SecretBytesPerJob {
		return out, secretstore.ErrTooLarge
	}
	return out, nil
}

func (s *Service) Check(ctx context.Context, q *sqlc.Queries, r secretstore.Request) (secretstore.CheckResult, error) {
	if q == nil {
		q = sqlc.New(s.db)
	}
	sel, err := s.choose(ctx, q, r)
	if err != nil {
		return secretstore.CheckResult{}, err
	}
	return sel.check(r)
}

func (s *Service) Resolve(ctx context.Context, r secretstore.Request) ([]secretstore.Secret, error) {
	q := sqlc.New(s.db)
	sel, err := s.choose(ctx, q, r)
	if err != nil {
		return nil, err
	}
	res, err := sel.check(r)
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, secretstore.ErrMissingName
	}
	out := make([]secretstore.Secret, 0, len(sel.names))
	used := make([]uuid.UUID, 0, len(sel.names))
	for _, n := range sel.names {
		row := sel.chosen[strings.ToLower(n)]
		plain, err := s.secrets.Open(ctx, ownerOf(row.Scope, row.OrganizationId, row.ProjectId, row.EnvironmentId),
			sealContext(row.Id, row.CurrentVersion), row.ValueSealed)
		if err != nil {
			secretstore.Wipe(out)
			return nil, err
		}
		out = append(out, secretstore.Secret{Name: n, Scope: row.Scope, Version: row.CurrentVersion, Value: plain})
		if !slices.Contains(used, row.Id) {
			used = append(used, row.Id)
		}
	}
	if len(used) > 0 {
		if err := q.TouchSecretsUsed(ctx, sqlc.TouchSecretsUsedParams{Ids: used, Now: s.now()}); err != nil {
			secretstore.Wipe(out)
			return nil, err
		}
	}
	return out, nil
}
