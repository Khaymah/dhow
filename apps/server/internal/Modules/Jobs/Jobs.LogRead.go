package jobs

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
)

func logText(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	var b strings.Builder
	b.Grow(len(data))
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			b.WriteRune(utf8.RuneError)
		} else {
			b.Write(data[:size])
		}
		data = data[size:]
	}
	return b.String()
}

func chunkOf(r sqlc.ListLogChunksRow) models.LogChunk {
	c := models.LogChunk{Seq: r.Seq, Stream: r.Stream, EmittedAt: r.EmittedAt, CreatedAt: r.CreatedAt, Text: logText(r.Data)}
	if r.StepIndex.Valid {
		step := int(r.StepIndex.Int32)
		c.StepIndex = &step
	}
	return c
}

func readFailure(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return apperrors.ServiceUnavailable()
	}
	return apperrors.Internal(err)
}

func (s *Service) logAttempt(ctx context.Context, org, job uuid.UUID, number *int) (sqlc.GetLogAttemptRow, error) {
	ctx, cancel := context.WithTimeout(ctx, s.readWait)
	defer cancel()
	q := s.readQueries()
	exists, err := q.JobExists(ctx, sqlc.JobExistsParams{ID: job, OrganizationID: org})
	if err != nil {
		return sqlc.GetLogAttemptRow{}, readFailure(err)
	}
	if !exists {
		return sqlc.GetLogAttemptRow{}, apperrors.NotFound()
	}
	arg := sqlc.GetLogAttemptParams{JobID: job, OrganizationID: org}
	if number != nil {
		arg.Number = pgtype.Int4{Int32: int32(*number), Valid: true} //nolint:gosec // The controller bounds the attempt number.
	}
	a, err := q.GetLogAttempt(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.GetLogAttemptRow{}, apperrors.AttemptNotFound()
	}
	if err != nil {
		return sqlc.GetLogAttemptRow{}, readFailure(err)
	}
	return a, nil
}

func (s *Service) attemptState(ctx context.Context, org, attempt uuid.UUID) (sqlc.GetLogAttemptStateRow, error) {
	ctx, cancel := context.WithTimeout(ctx, s.readWait)
	defer cancel()
	return s.readQueries().GetLogAttemptState(ctx, sqlc.GetLogAttemptStateParams{ID: attempt, OrganizationID: org})
}

func logsSince(a sqlc.GetLogAttemptRow, now time.Time) time.Time {
	if !a.OrgRetentionDays.Valid || a.OrgRetentionDays.Int32 <= 0 {
		return a.LeasedAt
	}
	cutoff := now.Add(-time.Duration(a.OrgRetentionDays.Int32) * 24 * time.Hour)
	if cutoff.After(a.LeasedAt) {
		return cutoff
	}
	return a.LeasedAt
}

func (s *Service) readChunks(ctx context.Context, org uuid.UUID, a sqlc.GetLogAttemptRow, from int64, rows int, maxBytes int64) ([]models.LogChunk, error) {
	ctx, cancel := context.WithTimeout(ctx, s.readWait)
	defer cancel()
	list, err := s.readQueries().ListLogChunks(ctx, sqlc.ListLogChunksParams{
		AttemptID: a.Id, OrganizationID: org, FromSeq: from, Since: logsSince(a, time.Now()), RowLimit: int32(rows), MaxBytes: maxBytes, //nolint:gosec // Row limits are small constants.
	})
	if err != nil {
		return nil, err
	}
	out := make([]models.LogChunk, 0, len(list))
	for _, r := range list {
		out = append(out, chunkOf(r))
	}
	return out, nil
}

func (s *Service) Logs(ctx context.Context, org, job uuid.UUID, in models.LogQuery) (models.LogPage, error) {
	a, err := s.logAttempt(ctx, org, job, in.Attempt)
	if err != nil {
		return models.LogPage{}, err
	}
	chunks, err := s.readChunks(ctx, org, a, in.FromSeq, in.Limit, config.LogPageMaxBytes)
	if err != nil {
		return models.LogPage{}, readFailure(err)
	}
	page := models.LogPage{
		Attempt: int(a.Number), AttemptID: a.Id, State: a.State, LastSeq: a.LastSeq, Truncated: a.Truncated, Chunks: chunks,
	}
	if n := len(chunks); n > 0 && chunks[n-1].Seq < a.LastSeq {
		next := chunks[n-1].Seq + 1
		page.NextSeq = &next
	}
	return page, nil
}
