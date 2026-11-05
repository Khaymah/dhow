package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"

	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	MaxReportBytes     = 64 << 10
	MaxReportEvents    = 100
	maxReportLine      = 4 << 10
	reportBatch        = 25
	reportQueue        = MaxReportEvents + 1
	reportDrainTimeout = 30 * time.Second
)

type releaseAPI interface {
	ReportRelease(ctx context.Context, ref *runnerv1.LeaseRef, step int32, events []*runnerv1.DeployEvent) (int32, error)
}

type reportHealth struct {
	Attempts     int    `json:"Attempts"`
	Passed       bool   `json:"Passed"`
	LastStatus   int    `json:"LastStatus"`
	LastError    string `json:"LastError"`
	DurationMs   int64  `json:"DurationMs"`
	StableChecks int    `json:"StableChecks"`
}

type reportLine struct {
	Event             string        `json:"Event"`
	HostID            string        `json:"HostId"`
	Batch             int           `json:"Batch"`
	ReleaseID         string        `json:"ReleaseId"`
	PreviousReleaseID string        `json:"PreviousReleaseId"`
	Mode              string        `json:"Mode"`
	Reason            string        `json:"Reason"`
	Message           string        `json:"Message"`
	Health            *reportHealth `json:"Health"`
}

var eventKinds = map[string]runnerv1.DeployEventKind{
	"release_started": runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_RELEASE_STARTED,
	"uploaded":        runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_UPLOADED,
	"switched":        runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_SWITCHED,
	"health":          runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_HEALTH,
	"verified":        runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_VERIFIED,
	"rolled_back":     runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLED_BACK,
	"rollback_failed": runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLBACK_FAILED,
	"pruned":          runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_PRUNED,
	"batch_passed":    runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_BATCH_PASSED,
	"failed":          runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_FAILED,
	"skipped":         runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_SKIPPED,
}

func parseReportLine(raw []byte, mask func(string) string) (*runnerv1.DeployEvent, bool) {
	if len(raw) > maxReportLine {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var l reportLine
	if err := dec.Decode(&l); err != nil || dec.More() {
		return nil, false
	}
	kind, ok := eventKinds[l.Event]
	if !ok {
		return nil, false
	}
	ev := &runnerv1.DeployEvent{
		Kind: kind, HostId: l.HostID, Batch: clampInt32(l.Batch), ReleaseId: l.ReleaseID, PreviousReleaseId: l.PreviousReleaseID,
		Mode: l.Mode, Reason: l.Reason, Message: mask(l.Message),
	}
	if h := l.Health; h != nil {
		ev.Health = &runnerv1.DeployHealth{
			Attempts: clampInt32(h.Attempts), Passed: h.Passed, LastStatus: clampInt32(h.LastStatus), LastError: mask(h.LastError),
			DurationMs: h.DurationMs, StableChecks: clampInt32(h.StableChecks),
		}
	}
	return ev, true
}

func clampInt32(v int) int32 {
	const limit = 1 << 30
	return int32(max(-limit, min(v, limit)))
}

type releaseReporter struct {
	a      *attempt
	ctx    context.Context
	step   int32
	api    releaseAPI
	queue  chan *runnerv1.DeployEvent
	done   chan struct{}
	mu     sync.Mutex
	buf    []byte
	bytes  int
	events int
	warned bool
	closed bool
}

func (a *attempt) newReporter(ctx context.Context, step int32) *releaseReporter {
	api, ok := a.e.o.API.(releaseAPI)
	if !ok {
		return nil
	}
	r := &releaseReporter{a: a, ctx: ctx, step: step, api: api, queue: make(chan *runnerv1.DeployEvent, reportQueue), done: make(chan struct{})}
	go r.run()
	return r
}

func (r *releaseReporter) warnOnce(msg string) {
	if r.warned {
		return
	}
	r.warned = true
	r.a.pipe.Systemf(&r.step, "dhow: warning: %s", msg)
}

func (r *releaseReporter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return len(p), nil
	}
	r.bytes += len(p)
	if r.bytes > MaxReportBytes {
		r.warnOnce("the deploy report is over its size limit, so the rest of it is ignored")
		return len(p), nil
	}
	r.buf = append(r.buf, p...)
	for {
		i := bytes.IndexByte(r.buf, '\n')
		if i < 0 {
			break
		}
		line := bytes.TrimSpace(r.buf[:i])
		r.buf = r.buf[i+1:]
		if len(line) > 0 {
			r.accept(line)
		}
	}
	if len(r.buf) > maxReportLine {
		r.buf = nil
		r.warnOnce("a deploy report line is too long and was ignored")
	}
	return len(p), nil
}

func (r *releaseReporter) accept(line []byte) {
	if r.events >= MaxReportEvents {
		r.warnOnce("the deploy report has more events than the limit, so the rest of it is ignored")
		return
	}
	ev, ok := parseReportLine(line, r.a.masker.MaskString)
	if !ok {
		r.warnOnce("a deploy report line is not a valid event and was ignored")
		return
	}
	r.events++
	r.queue <- ev
}

func (r *releaseReporter) run() {
	defer close(r.done)
	for ev := range r.queue {
		batch := []*runnerv1.DeployEvent{ev}
		for len(batch) < reportBatch {
			select {
			case more, ok := <-r.queue:
				if !ok {
					r.send(batch)
					return
				}
				batch = append(batch, more)
				continue
			default:
			}
			break
		}
		r.send(batch)
	}
}

func (r *releaseReporter) send(events []*runnerv1.DeployEvent) {
	a := r.a
	err := a.retry(r.ctx, func(rctx context.Context) error {
		_, err := r.api.ReportRelease(rctx, a.ref, r.step, events)
		return err
	})
	switch {
	case err == nil:
	case client.IsLeaseLost(err):
		a.fence.markLost(ErrLeaseLost)
	default:
		a.log.Warn("the server did not accept a deploy report", "code", client.Code(err).String(), "reason", client.Reason(err))
	}
}

func (r *releaseReporter) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if rest := bytes.TrimSpace(r.buf); len(rest) > 0 && !r.closed {
		r.accept(rest)
	}
	r.buf = nil
	r.closed = true
	close(r.queue)
	r.mu.Unlock()
	t := time.NewTimer(reportDrainTimeout)
	defer t.Stop()
	select {
	case <-r.done:
	case <-t.C:
	}
}
