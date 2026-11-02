package sources

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
	"github.com/Khaymah/dhow/internal/gitref"
)

const (
	StatusPending  = commitstatus.StatePending
	StatusRunning  = commitstatus.StateRunning
	StatusSuccess  = commitstatus.StateSuccess
	StatusFailed   = commitstatus.StateFailed
	StatusCanceled = commitstatus.StateCanceled

	maxStatusError = 256
)

type DesiredStatus struct {
	OrganizationID uuid.UUID
	RepositoryID   uuid.UUID
	RunID          uuid.UUID
	Sha            string
	State          string
	TargetURL      string
}

type StatusPass struct {
	Sent      int
	Unneeded  int
	Failed    int
	Abandoned int
}

func validStatus(v string) bool {
	switch v {
	case StatusPending, StatusRunning, StatusSuccess, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}

func (s *Service) RecordStatus(ctx context.Context, tx *sqlc.Queries, d DesiredStatus) error {
	if !validStatus(d.State) || !gitref.ValidSha(d.Sha) || len(d.TargetURL) > 2048 {
		return apperrors.ValidationFailed()
	}
	_, err := tx.UpsertCommitStatus(ctx, sqlc.UpsertCommitStatusParams{
		OrganizationID: d.OrganizationID, RepositoryID: d.RepositoryID, Sha: d.Sha, RunID: d.RunID, TargetUrl: text(d.TargetURL),
		DesiredState: d.State, Now: s.now(),
	})
	return err
}

func backoff(attempts int32) time.Duration {
	d := config.CommitStatusBaseBackoff
	for i := int32(0); i < attempts && d < config.CommitStatusMaxBackoff; i++ {
		d *= 2
	}
	return min(d, config.CommitStatusMaxBackoff)
}

func shortError(err error) string {
	msg := err.Error()
	if len(msg) > maxStatusError {
		msg = msg[:maxStatusError]
	}
	if msg == "" {
		msg = "failed"
	}
	return msg
}

func (s *Service) SendStatuses(ctx context.Context) (StatusPass, error) {
	q := s.queries()
	now := s.now()
	rows, err := q.ClaimCommitStatuses(ctx, sqlc.ClaimCommitStatusesParams{
		LeaseUntil: now.Add(config.CommitStatusClaim), Now: now, Batch: config.CommitStatusBatch,
	})
	if err != nil {
		return StatusPass{}, err
	}
	var pass StatusPass
	for _, row := range rows {
		if err := s.sendStatus(ctx, q, row, &pass); err != nil {
			return pass, err
		}
	}
	return pass, nil
}

func (s *Service) sendStatus(ctx context.Context, q *sqlc.Queries, row sqlc.ClaimCommitStatusesRow, pass *StatusPass) error {
	err := ErrUnsupported
	if p, ok := s.providers[row.Provider]; ok {
		repo := models.Repository{ID: row.RepositoryId, OrganizationID: row.OrganizationId, Provider: row.Provider, CloneURL: row.CloneUrl}
		err = p.ReportStatus(ctx, repo, StatusReport{Sha: row.Sha, State: row.DesiredState, TargetURL: row.TargetUrl.String})
	}
	now := s.now()
	switch {
	case err == nil || errors.Is(err, ErrUnsupported):
		if err == nil {
			pass.Sent++
		} else {
			pass.Unneeded++
		}
		return q.MarkCommitStatusSent(ctx, sqlc.MarkCommitStatusSentParams{ID: row.Id, SentState: text(row.DesiredState), Now: now})
	default:
		pass.Failed++
		params := sqlc.MarkCommitStatusFailedParams{
			ID: row.Id, LastError: text(shortError(err)), NextAttemptAt: now.Add(backoff(row.Attempts)), Now: now,
		}
		if now.Sub(row.DesiredAt) >= config.CommitStatusGiveUpAfter {
			params.AbandonedAt = &now
			pass.Abandoned++
		}
		return q.MarkCommitStatusFailed(ctx, params)
	}
}

func (s *Service) RunStatusWorker(ctx context.Context) {
	ticker := time.NewTicker(config.CommitStatusInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		pass, err := s.SendStatuses(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			s.log.ErrorContext(ctx, "sending commit statuses failed; the next pass retries", "error", err)
		case pass.Abandoned > 0:
			s.log.WarnContext(ctx, "commit statuses were given up after a day of failures", "abandoned", pass.Abandoned)
		}
	}
}

type statusRecorder struct {
	service *Service
}

var _ commitstatus.Recorder = statusRecorder{}

func (s *Service) StatusRecorder() commitstatus.Recorder { return statusRecorder{service: s} }

func (r statusRecorder) Record(ctx context.Context, tx *sqlc.Queries, d commitstatus.Desired) error {
	return r.service.RecordStatus(ctx, tx, DesiredStatus{
		OrganizationID: d.OrganizationID, RepositoryID: d.RepositoryID, RunID: d.RunID, Sha: d.Sha, State: d.State, TargetURL: d.TargetURL,
	})
}
