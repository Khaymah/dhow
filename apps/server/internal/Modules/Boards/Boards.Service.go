package boards

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Boards/Models"
)

const (
	MaxTargets      = 500
	maxEnvironments = 1000
	slotCurrent     = "current"
	slotLast        = "last"
	slotRunning     = "running"
)

type Store interface {
	ListBoardEnvironments(ctx context.Context, arg sqlc.ListBoardEnvironmentsParams) ([]sqlc.ListBoardEnvironmentsRow, error)
	ListBoardTargets(ctx context.Context, arg sqlc.ListBoardTargetsParams) ([]sqlc.ListBoardTargetsRow, error)
	ListBoardReleases(ctx context.Context, arg sqlc.ListBoardReleasesParams) ([]sqlc.ListBoardReleasesRow, error)
	ListBoardRunningHosts(ctx context.Context, arg sqlc.ListBoardRunningHostsParams) ([]sqlc.ListBoardRunningHostsRow, error)
	ListBoardHostChecks(ctx context.Context, arg sqlc.ListBoardHostChecksParams) ([]sqlc.ListBoardHostChecksRow, error)
	CountBoardApprovals(ctx context.Context, arg sqlc.CountBoardApprovalsParams) ([]sqlc.CountBoardApprovalsRow, error)
}

type Options struct {
	Store   Store
	Limiter ratelimit.Limiter
	Log     *slog.Logger
	Now     func() time.Time
}

type Service struct {
	store   Store
	limiter ratelimit.Limiter
	log     *slog.Logger
	now     func() time.Time
}

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: o.Store, limiter: o.Limiter, log: o.Log, now: now}
}

type snapshot struct {
	envs      []sqlc.ListBoardEnvironmentsRow
	targets   []sqlc.ListBoardTargetsRow
	releases  map[uuid.UUID]map[string]sqlc.ListBoardReleasesRow
	progress  map[uuid.UUID]sqlc.ListBoardRunningHostsRow
	checks    map[uuid.UUID][]sqlc.ListBoardHostChecksRow
	approvals map[uuid.UUID]int
	truncated bool
}

func (sn snapshot) targetIDs() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(sn.targets))
	for _, t := range sn.targets {
		ids = append(ids, t.Id)
	}
	return ids
}

func (s *Service) Get(ctx context.Context, org, project uuid.UUID) (models.Board, error) {
	if err := s.allow(ctx); err != nil {
		return models.Board{}, err
	}
	snap, err := s.load(ctx, org, project)
	if err != nil {
		return models.Board{}, err
	}
	return assemble(snap), nil
}

func (s *Service) load(ctx context.Context, org, project uuid.UUID) (snapshot, error) {
	var snap snapshot
	var err error
	if snap.envs, err = s.store.ListBoardEnvironments(ctx, sqlc.ListBoardEnvironmentsParams{
		OrganizationID: org, ProjectID: project, MaxRows: maxEnvironments,
	}); err != nil {
		return snapshot{}, fmt.Errorf("list the environments of the board: %w", err)
	}
	if snap.targets, err = s.store.ListBoardTargets(ctx, sqlc.ListBoardTargetsParams{
		OrganizationID: org, ProjectID: project, MaxRows: MaxTargets + 1,
	}); err != nil {
		return snapshot{}, fmt.Errorf("list the targets of the board: %w", err)
	}
	if len(snap.targets) > MaxTargets {
		snap.targets, snap.truncated = snap.targets[:MaxTargets], true
	}
	if err := s.loadReleases(ctx, org, project, &snap); err != nil {
		return snapshot{}, err
	}
	if err := s.loadChecks(ctx, org, project, &snap); err != nil {
		return snapshot{}, err
	}
	return snap, nil
}

func (s *Service) loadReleases(ctx context.Context, org, project uuid.UUID, snap *snapshot) error {
	rows, err := s.store.ListBoardReleases(ctx, sqlc.ListBoardReleasesParams{OrganizationID: org, ProjectID: project, TargetIds: snap.targetIDs()})
	if err != nil {
		return fmt.Errorf("list the releases of the board: %w", err)
	}
	snap.releases = map[uuid.UUID]map[string]sqlc.ListBoardReleasesRow{}
	var running []uuid.UUID
	for _, r := range rows {
		if snap.releases[r.TargetId] == nil {
			snap.releases[r.TargetId] = map[string]sqlc.ListBoardReleasesRow{}
		}
		snap.releases[r.TargetId][r.Slot] = r
		if r.Slot == slotRunning {
			running = append(running, r.Id)
		}
	}
	snap.progress = map[uuid.UUID]sqlc.ListBoardRunningHostsRow{}
	if len(running) == 0 {
		return nil
	}
	hosts, err := s.store.ListBoardRunningHosts(ctx, sqlc.ListBoardRunningHostsParams{ReleaseRowIds: running, OrganizationID: org})
	if err != nil {
		return fmt.Errorf("list the hosts of the running releases: %w", err)
	}
	for _, h := range hosts {
		snap.progress[h.ReleaseRowId] = h
	}
	return nil
}

func (s *Service) loadChecks(ctx context.Context, org, project uuid.UUID, snap *snapshot) error {
	checks, err := s.store.ListBoardHostChecks(ctx, sqlc.ListBoardHostChecksParams{OrganizationID: org, TargetIds: snap.targetIDs()})
	if err != nil {
		return fmt.Errorf("list the checks of the board: %w", err)
	}
	snap.checks = map[uuid.UUID][]sqlc.ListBoardHostChecksRow{}
	for _, c := range checks {
		snap.checks[c.TargetId] = append(snap.checks[c.TargetId], c)
	}
	pending, err := s.store.CountBoardApprovals(ctx, sqlc.CountBoardApprovalsParams{OrganizationID: org, ProjectID: project, Now: s.now().UTC()})
	if err != nil {
		return fmt.Errorf("count the approvals of the board: %w", err)
	}
	snap.approvals = map[uuid.UUID]int{}
	for _, p := range pending {
		snap.approvals[p.EnvironmentId] = int(p.Pending)
	}
	return nil
}

func (s *Service) allow(ctx context.Context) error {
	if s.limiter == nil {
		return nil
	}
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return apperrors.Unauthenticated()
	}
	d, err := s.limiter.Take(ctx, ratelimit.Limit{Name: "board-read", Max: config.BoardReadsPerMinute, Window: time.Minute}, p.UserID.String())
	if err != nil {
		return fmt.Errorf("take the board read limit: %w", err)
	}
	return d.Err()
}
