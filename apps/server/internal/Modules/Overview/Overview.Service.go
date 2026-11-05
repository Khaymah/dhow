package overview

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Overview/Models"
)

type Counter interface {
	CountOverviewJobs(ctx context.Context, arg sqlc.CountOverviewJobsParams) (sqlc.CountOverviewJobsRow, error)
	CountOverviewRunners(ctx context.Context, arg sqlc.CountOverviewRunnersParams) (sqlc.CountOverviewRunnersRow, error)
	CountOverviewInvitations(ctx context.Context, arg sqlc.CountOverviewInvitationsParams) (int64, error)
	CountOverviewApprovals(ctx context.Context, arg sqlc.CountOverviewApprovalsParams) (int64, error)
}

type Options struct {
	Counter Counter
	Log     *slog.Logger
	Now     func() time.Time
}

type Service struct {
	counter Counter
	log     *slog.Logger
	now     func() time.Time
}

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{counter: o.Counter, log: o.Log, now: now}
}

func (s *Service) Get(ctx context.Context, org uuid.UUID) (models.Overview, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return models.Overview{}, apperrors.Unauthenticated()
	}
	resource := policy.Resource{OrganizationID: org}
	now := s.now().UTC()
	allowed := func(action policy.Action) bool {
		return policy.Can(p.Subject, action, resource) && !policy.SecondFactorBlocks(p.Subject, action, resource, now)
	}
	var out models.Overview
	if allowed(policy.JobRead) {
		jobs, err := s.jobs(ctx, org, now)
		if err != nil {
			return models.Overview{}, err
		}
		out.Jobs = &jobs
	}
	if allowed(policy.RunnerRead) {
		runners, err := s.runners(ctx, org, now)
		if err != nil {
			return models.Overview{}, err
		}
		out.Runners = &runners
	}
	if allowed(policy.MemberManage) {
		pending, err := s.counter.CountOverviewInvitations(ctx, sqlc.CountOverviewInvitationsParams{OrganizationID: org, Now: now})
		if err != nil {
			return models.Overview{}, fmt.Errorf("count pending invitations: %w", err)
		}
		out.Invitations = &models.Invitations{Pending: int(pending)}
	}
	if allowed(policy.RunRead) {
		pending, err := s.counter.CountOverviewApprovals(ctx, sqlc.CountOverviewApprovalsParams{OrganizationID: org, Now: now, UserID: &p.UserID})
		if err != nil {
			return models.Overview{}, fmt.Errorf("count pending approvals: %w", err)
		}
		out.Approvals = &models.Approvals{Pending: int(pending)}
	}
	return out, nil
}

func (s *Service) jobs(ctx context.Context, org uuid.UUID, now time.Time) (models.Jobs, error) {
	row, err := s.counter.CountOverviewJobs(ctx, sqlc.CountOverviewJobsParams{
		OrganizationID: org, FailedSince: now.Add(-config.JobRecentFailureWindow),
	})
	if err != nil {
		return models.Jobs{}, fmt.Errorf("count jobs: %w", err)
	}
	return models.Jobs{
		Running: int(row.Running), Queued: int(row.Queued), QueuedNoRunner: int(row.QueuedNoRunner), FailedLast24h: int(row.FailedLast24h),
	}, nil
}

func (s *Service) runners(ctx context.Context, org uuid.UUID, now time.Time) (models.Runners, error) {
	row, err := s.counter.CountOverviewRunners(ctx, sqlc.CountOverviewRunnersParams{
		OrganizationID: org, OnlineSince: now.Add(-config.RunnerOnlineWindow),
	})
	if err != nil {
		return models.Runners{}, fmt.Errorf("count runners: %w", err)
	}
	return models.Runners{Online: int(row.Online), Offline: int(row.Offline), Paused: int(row.Paused)}, nil
}
