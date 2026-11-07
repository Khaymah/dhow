package jobs

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type RawLog struct {
	service *Service
	ctx     context.Context
	org     uuid.UUID
	job     uuid.UUID
	attempt sqlc.GetLogAttemptRow
}

func (s *Service) OpenRawLog(ctx context.Context, org, job uuid.UUID, attempt *int) (*RawLog, error) {
	a, err := s.logAttempt(ctx, org, job, attempt)
	if err != nil {
		return nil, err
	}
	return &RawLog{service: s, ctx: ctx, org: org, job: job, attempt: a}, nil
}

func (l *RawLog) Filename() string {
	return "job-" + l.job.String() + "-attempt-" + strconv.Itoa(int(l.attempt.Number)) + ".log"
}

func (l *RawLog) Serve(w http.ResponseWriter) error {
	rc := http.NewResponseController(w)
	if err := supported(rc.SetReadDeadline(time.Time{})); err != nil {
		return nil
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="`+l.Filename()+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	end := l.attempt.LastSeq
	from := int64(1)
	for from <= end {
		chunks, err := l.service.readChunks(l.ctx, l.org, l.attempt, from, config.LogStreamBatchRows, config.LogStreamBatchBytes)
		if err != nil || len(chunks) == 0 {
			return nil
		}
		if err := supported(rc.SetWriteDeadline(time.Now().Add(l.service.wait))); err != nil {
			return nil
		}
		for _, c := range chunks {
			if c.Seq > end {
				return nil
			}
			if _, err := w.Write([]byte(c.Text)); err != nil {
				return nil
			}
			from = c.Seq + 1
		}
		if err := supported(rc.Flush()); err != nil {
			return nil
		}
	}
	return nil
}
