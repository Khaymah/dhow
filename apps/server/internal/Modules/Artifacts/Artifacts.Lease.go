package artifacts

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	branchPrefix     = "refs/heads/"
	tagPrefix        = "refs/tags/"
	triggerPR        = "pull_request"
	forkCacheDefault = "default_branch"
	triggerTag       = "tag"
	triggerManual    = "manual"
	triggerAPI       = "api"
	noRepository     = "none"
)

type leaseRef struct {
	id      uuid.UUID
	fencing int64
}

func refOf(lease *runnerv1.LeaseRef) (leaseRef, error) {
	return parseRef(lease.GetLeaseId(), strconv.FormatInt(lease.GetFencingToken(), 10))
}

func parseRef(id, fencing string) (leaseRef, error) {
	lid, err := uuid.Parse(id)
	if err != nil {
		return leaseRef{}, errLeaseLost()
	}
	f, err := strconv.ParseInt(fencing, 10, 64)
	if err != nil || f <= 0 {
		return leaseRef{}, errLeaseLost()
	}
	return leaseRef{id: lid, fencing: f}, nil
}

type scope struct {
	row  sqlc.LockArtifactLeaseRow
	spec *runnerv1.JobSpec
}

func (l scope) pipeline() bool {
	return l.row.OriginKind == "pipeline" && l.row.RunId != nil && l.row.RunJobId != nil && l.row.ProjectId != nil
}

func (l scope) runJob() uuid.UUID {
	if l.row.RunJobId == nil {
		return uuid.Nil
	}
	return *l.row.RunJobId
}

func (l scope) fork() bool { return !l.row.IsFork.Valid || l.row.IsFork.Bool }

func (l scope) repository() string {
	if l.row.RepositoryId == nil {
		return "repo:" + noRepository + "/"
	}
	return "repo:" + l.row.RepositoryId.String() + "/"
}

func (l scope) ownScope() (string, bool) {
	ref := l.row.Ref.String
	switch {
	case l.row.Trigger.String == triggerPR:
		if !l.row.PrNumber.Valid || l.row.PrNumber.Int32 <= 0 {
			return "", false
		}
		return l.repository() + "pr:" + strconv.Itoa(int(l.row.PrNumber.Int32)), true
	case l.row.Trigger.String == triggerTag && strings.HasPrefix(ref, tagPrefix):
		return l.repository() + "tag:" + strings.TrimPrefix(ref, tagPrefix), true
	case strings.HasPrefix(ref, branchPrefix):
		return l.repository() + "branch:" + strings.TrimPrefix(ref, branchPrefix), true
	default:
		return "", false
	}
}

func (l scope) atHead() bool {
	switch l.row.Trigger.String {
	case triggerManual, triggerAPI:
		return l.row.Sha.Valid && l.row.RefHead != "" && l.row.Sha.String == l.row.RefHead
	default:
		return true
	}
}

func (l scope) writeScope() (string, bool) {
	if l.fork() || !l.atHead() {
		return "", false
	}
	return l.ownScope()
}

func (l scope) defaultScope() string {
	if l.row.DefaultBranch == "" {
		return ""
	}
	return l.repository() + "branch:" + l.row.DefaultBranch
}

func (l scope) hadSecrets() bool {
	for _, ref := range l.spec.GetSecrets() {
		if ref.GetName() != jobspec.CheckoutSecret {
			return true
		}
	}
	return len(l.spec.GetRegistryAuth()) > 0
}

func (l scope) restoreScopes() []string {
	if l.fork() {
		if def := l.defaultScope(); def != "" && l.row.ForkCacheRestore == forkCacheDefault {
			return []string{def}
		}
		return nil
	}
	var out []string
	if own, ok := l.ownScope(); ok && !l.fork() {
		out = append(out, own)
	}
	if def := l.defaultScope(); def != "" && (len(out) == 0 || out[0] != def) {
		out = append(out, def)
	}
	return out
}

func lockLease(ctx context.Context, q *sqlc.Queries, ref leaseRef) (scope, error) {
	p, ok := runnerauth.PrincipalFrom(ctx)
	if !ok {
		return scope{}, errUnauthenticated()
	}
	row, err := q.LockArtifactLease(ctx, sqlc.LockArtifactLeaseParams{LeaseID: ref.id, FencingToken: ref.fencing, RunnerID: p.RunnerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return scope{}, errLeaseLost()
	}
	if err != nil {
		return scope{}, err
	}
	spec, err := decodeSpec(row.Spec)
	if err != nil {
		return scope{}, err
	}
	return scope{row: row, spec: spec}, nil
}

func (s *Service) withLease(ctx context.Context, ref leaseRef, fn func(q *sqlc.Queries, l scope) error) error {
	return dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		l, err := lockLease(ctx, q, ref)
		if err != nil {
			return err
		}
		return fn(q, l)
	})
}

func declaredArtifact(spec *runnerv1.JobSpec, name string) (*runnerv1.ArtifactUpload, bool) {
	for _, a := range spec.GetArtifacts() {
		if a.GetName() == name {
			return a, true
		}
	}
	return nil, false
}
