package jobs

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
)

type RawLog struct {
	service *Service
	ctx     context.Context
	cancel  context.CancelFunc
	release func()
	org     uuid.UUID
	job     uuid.UUID
	attempt sqlc.GetLogAttemptRow
	first   []models.LogChunk
}

func (s *Service) OpenRawLog(ctx context.Context, user, org, job uuid.UUID, attempt *int) (*RawLog, error) {
	a, err := s.logAttempt(ctx, org, job, attempt)
	if err != nil {
		return nil, err
	}
	release, err := s.streams.acquire(user, org)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, config.LogDownloadMaxDuration)
	l := &RawLog{service: s, ctx: ctx, cancel: cancel, release: release, org: org, job: job, attempt: a}
	if a.LastSeq >= 1 {
		l.first, err = s.readChunks(ctx, org, a, 1, config.LogStreamBatchRows, config.LogStreamBatchBytes)
		if err != nil {
			l.Close()
			return nil, readFailure(err)
		}
	}
	return l, nil
}

func (l *RawLog) Close() {
	l.cancel()
	l.release()
}

func (l *RawLog) Filename() string {
	return "job-" + l.job.String() + "-attempt-" + strconv.Itoa(int(l.attempt.Number)) + ".log"
}

func (l *RawLog) Serve(w http.ResponseWriter) error {
	defer l.Close()
	rc := http.NewResponseController(w)
	if err := supported(rc.SetReadDeadline(time.Time{})); err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="`+l.Filename()+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := l.copy(w, rc); err != nil {
		l.abort(rc, err)
	}
	return nil
}

func (l *RawLog) copy(w http.ResponseWriter, rc *http.ResponseController) error {
	end := l.attempt.LastSeq
	chunks := l.first
	for len(chunks) > 0 {
		if err := supported(rc.SetWriteDeadline(time.Now().Add(l.service.wait))); err != nil {
			return err
		}
		from := int64(0)
		for _, c := range chunks {
			if c.Seq > end {
				return supported(rc.Flush())
			}
			if _, err := w.Write([]byte(c.Text)); err != nil {
				return err
			}
			from = c.Seq + 1
		}
		if err := supported(rc.Flush()); err != nil {
			return err
		}
		if from > end {
			return nil
		}
		next, err := l.service.readChunks(l.ctx, l.org, l.attempt, from, config.LogStreamBatchRows, config.LogStreamBatchBytes)
		if err != nil {
			return err
		}
		chunks = next
	}
	return nil
}

func (l *RawLog) abort(rc *http.ResponseController, cause error) {
	l.service.log.Warn("raw log download stopped before the end", slog.String("Job", l.job.String()), slog.Int("Attempt", int(l.attempt.Number)), slog.String("Error", cause.Error()))
	conn, _, err := rc.Hijack()
	if err != nil {
		l.service.log.Warn("raw log connection cannot be cut", slog.String("Error", err.Error()))
		return
	}
	if err := conn.Close(); err != nil {
		l.service.log.Warn("raw log connection close failed", slog.String("Error", err.Error()))
	}
}
