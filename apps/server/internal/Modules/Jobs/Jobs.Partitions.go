package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type PartitionResult struct {
	Skipped    bool
	Created    int
	Dropped    int
	Unreadable []string
}

func (s *Service) MaintainLogPartitions(ctx context.Context) (PartitionResult, error) {
	var out PartitionResult
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		locked, err := q.TryLockLogPartitions(ctx)
		if err != nil {
			return err
		}
		if !locked {
			out.Skipped = true
			return nil
		}
		if err := q.SetLocalLockTimeout(ctx, config.LogMaintainLockWait.String()); err != nil {
			return err
		}
		created, err := q.EnsureLogPartitions(ctx, config.LogPartitionsAhead)
		if err != nil {
			return fmt.Errorf("create log partitions: %w", err)
		}
		dropped, err := q.DropLogPartitions(ctx, int32(s.limits().RetentionDays)) //nolint:gosec // The config bounds retention to 3650 days.
		if err != nil {
			return fmt.Errorf("drop expired log partitions: %w", err)
		}
		out.Created, out.Dropped = int(created), int(dropped)
		out.Unreadable, err = q.UnreadableLogPartitions(ctx)
		return err
	})
	if err == nil && len(out.Unreadable) > 0 {
		s.log.WarnContext(ctx, "job log partitions without a dated name are never dropped; rename or detach them",
			"partitions", strings.Join(out.Unreadable, ","))
	}
	return out, err
}

func (s *Service) ScrubOrganizationLogs(ctx context.Context) (int64, error) {
	orgs, err := s.queries().OrganizationsWithLogRetention(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, o := range orgs {
		before := time.Now().Add(-time.Duration(o.Days) * 24 * time.Hour)
		for {
			n, err := s.queries().ScrubOrganizationLogs(ctx, sqlc.ScrubOrganizationLogsParams{
				OrganizationID: o.Id, Before: before, Batch: config.LogScrubBatch,
			})
			if err != nil {
				return total, fmt.Errorf("erase expired logs of an organization: %w", err)
			}
			total += n
			if n < config.LogScrubBatch {
				break
			}
		}
	}
	return total, nil
}

func (s *Service) RunLogMaintenance(ctx context.Context) {
	ticker := time.NewTicker(config.LogMaintainInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if n, err := s.ScrubOrganizationLogs(ctx); err != nil && ctx.Err() == nil {
			s.log.ErrorContext(ctx, "erasing logs past an organization's retention failed; the next pass retries", "error", err)
		} else if n > 0 {
			s.log.InfoContext(ctx, "logs past an organization's retention were erased", "chunks", n)
		}
		res, err := s.MaintainLogPartitions(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			s.log.ErrorContext(ctx, "job log partition maintenance failed; the next pass retries", "error", err)
		case res.Created > 0 || res.Dropped > 0:
			s.log.InfoContext(ctx, "job log partitions maintained", "created", res.Created, "dropped", res.Dropped)
		}
	}
}

func (s *Service) CheckLogPartition(ctx context.Context) error {
	ok, err := s.queries().LogPartitionReady(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errLogPartitionMissing
	}
	return nil
}
