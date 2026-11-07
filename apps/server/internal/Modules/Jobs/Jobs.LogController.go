package jobs

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
)

const messageLogsLoaded = "jobs.logs.loaded"

func attemptParam(v *int) (*int, error) {
	if v != nil && (*v < 1 || *v > math.MaxInt32) {
		return nil, apperrors.ValidationFailed(invalid("Attempt"))
	}
	return v, nil
}

func parseLastEvent(v *string) (*EventRef, error) {
	if v == nil {
		return nil, nil
	}
	bad := apperrors.ValidationFailed(invalid("Last-Event-ID"))
	attempt, seq, ok := strings.Cut(*v, ":")
	if !ok {
		return nil, bad
	}
	number, err := strconv.ParseInt(attempt, 10, 32)
	if err != nil || number < 1 {
		return nil, bad
	}
	last, err := strconv.ParseInt(seq, 10, 64)
	if err != nil || last < 0 || last == math.MaxInt64 {
		return nil, bad
	}
	return &EventRef{Attempt: int(number), Seq: last}, nil
}

func (c *Controller) GetJobLogs(ctx context.Context, req gen.GetJobLogsRequestObject) (gen.GetJobLogsResponseObject, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.service.AllowLogRead(p.UserID); err != nil {
		return nil, err
	}
	if err := c.authorizeJob(ctx, policy.LogRead, req.Id, req.JobId); err != nil {
		return nil, err
	}
	attempt, err := attemptParam(req.Params.Attempt)
	if err != nil {
		return nil, err
	}
	in := models.LogQuery{Attempt: attempt, FromSeq: 1, Limit: config.LogPageDefaultLimit}
	if v := req.Params.FromSeq; v != nil {
		if *v < 1 {
			return nil, apperrors.ValidationFailed(invalid("FromSeq"))
		}
		in.FromSeq = *v
	}
	if v := req.Params.Limit; v != nil {
		if *v < 1 || *v > config.LogPageMaxLimit {
			return nil, apperrors.ValidationFailed(invalid("Limit"))
		}
		in.Limit = *v
	}
	page, err := c.service.Logs(ctx, req.Id, req.JobId, in)
	if err != nil {
		return nil, err
	}
	data := logPageToGen(page)
	return gen.GetJobLogs200JSONResponse{Success: true, Message: ptrOf(messageLogsLoaded), Data: &data}, nil
}

func (c *Controller) DownloadJobLog(ctx context.Context, req gen.DownloadJobLogRequestObject) (gen.DownloadJobLogResponseObject, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.service.AllowLogRead(p.UserID); err != nil {
		return nil, err
	}
	if err := c.authorizeJob(ctx, policy.LogRead, req.Id, req.JobId); err != nil {
		return nil, err
	}
	attempt, err := attemptParam(req.Params.Attempt)
	if err != nil {
		return nil, err
	}
	raw, err := c.service.OpenRawLog(ctx, req.Id, req.JobId, attempt)
	if err != nil {
		return nil, err
	}
	return rawLogResponse{raw: raw}, nil
}

type rawLogResponse struct {
	raw *RawLog
}

func (r rawLogResponse) VisitDownloadJobLogResponse(w http.ResponseWriter) error {
	return r.raw.Serve(w)
}

func (c *Controller) StreamJobLogs(ctx context.Context, req gen.StreamJobLogsRequestObject) (gen.StreamJobLogsResponseObject, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return nil, err
	}
	attempt, err := attemptParam(req.Params.Attempt)
	if err != nil {
		return nil, err
	}
	last, err := parseLastEvent(req.Params.LastEventID)
	if err != nil {
		return nil, err
	}
	from := int64(1)
	if req.Params.FromSeq != nil {
		if *req.Params.FromSeq < 1 {
			return nil, apperrors.ValidationFailed(invalid("FromSeq"))
		}
		from = *req.Params.FromSeq
	}
	org, job := req.Id, req.JobId
	if err := c.authorizeJob(ctx, policy.LogRead, org, job); err != nil {
		return nil, err
	}
	stream, err := c.service.OpenLogStream(ctx, StreamRequest{
		Organization: org, Job: job, Attempt: attempt, FromSeq: from, LastEvent: last, User: p.UserID,
		Reauthorize: func(ctx context.Context) error {
			if err := c.guard.Reauthorize(ctx, policy.LogRead, org); err != nil {
				return err
			}
			return c.authorizeJob(ctx, policy.LogRead, org, job)
		},
	})
	if err != nil {
		return nil, err
	}
	return streamResponse{stream: stream}, nil
}

type streamResponse struct {
	stream *LogStream
}

func (r streamResponse) VisitStreamJobLogsResponse(w http.ResponseWriter) error {
	return r.stream.Serve(w)
}

func logPageToGen(p models.LogPage) gen.JobLogPage {
	chunks := make([]gen.JobLogChunk, 0, len(p.Chunks))
	for _, c := range p.Chunks {
		chunks = append(chunks, gen.JobLogChunk{
			Seq: c.Seq, Stream: gen.JobLogStream(c.Stream), StepIndex: c.StepIndex, EmittedAt: c.EmittedAt, CreatedAt: c.CreatedAt, Text: c.Text,
		})
	}
	return gen.JobLogPage{
		Attempt: p.Attempt, AttemptId: p.AttemptID, State: gen.JobAttemptState(p.State), LastSeq: p.LastSeq,
		Truncated: p.Truncated, NextSeq: p.NextSeq, Chunks: chunks,
	}
}
