package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
	"github.com/Khaymah/dhow/internal/pathglob"
)

const wildcards = "*?"

func literalPrefix(p string) string {
	if i := strings.IndexAny(p, wildcards); i >= 0 {
		return p[:i]
	}
	return p
}

func CouldOverlap(a, b string) bool {
	if !pathglob.ValidPattern(a) || !pathglob.ValidPattern(b) {
		return true
	}
	aLiteral, bLiteral := !strings.ContainsAny(a, wildcards), !strings.ContainsAny(b, wildcards)
	switch {
	case aLiteral && bLiteral:
		return a == b
	case aLiteral:
		return pathglob.MatchPattern(b, a)
	case bLiteral:
		return pathglob.MatchPattern(a, b)
	}
	pa, pb := literalPrefix(a), literalPrefix(b)
	return strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa)
}

func patternsReach(token []models.RefPattern, env []models.RefPattern) bool {
	for _, t := range token {
		for _, e := range env {
			if t.Kind == e.Kind && CouldOverlap(t.Pattern, e.Pattern) {
				return true
			}
		}
	}
	return false
}

type reachEnv struct {
	name      string
	protected bool
	refs      []models.RefPattern
}

func (s *Service) environmentsOf(ctx context.Context, q *sqlc.Queries, org, project uuid.UUID) ([]reachEnv, error) {
	rows, err := q.ListEnvironmentReach(ctx, sqlc.ListEnvironmentReachParams{OrganizationID: org, ProjectID: project})
	if err != nil {
		return nil, err
	}
	out := make([]reachEnv, 0, len(rows))
	for _, r := range rows {
		var refs []models.RefPattern
		if err := json.Unmarshal(r.AllowedRefs, &refs); err != nil {
			return nil, fmt.Errorf("decode the allowed refs of environment %q: %w", r.Name, err)
		}
		out = append(out, reachEnv{name: r.Name, protected: r.HasTargets || r.HasReviewers, refs: refs})
	}
	return out, nil
}

func reachOf(patterns []models.RefPattern, envs []reachEnv) (names []string, protected bool) {
	names = []string{}
	for _, e := range envs {
		if patternsReach(patterns, e.refs) {
			names = append(names, e.name)
			protected = protected || e.protected
		}
	}
	return names, protected
}
