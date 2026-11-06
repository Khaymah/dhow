package deployrelease

import (
	"encoding/json"
	"io"
	"sync"
)

const (
	MaxReportEvents = 100
	maxReportLine   = 4 << 10

	EventStarted        = "release_started"
	EventUploaded       = "uploaded"
	EventSwitched       = "switched"
	EventHealth         = "health"
	EventVerified       = "verified"
	EventRolledBack     = "rolled_back"
	EventRollbackFailed = "rollback_failed"
	EventPruned         = "pruned"
	EventFailed         = "failed"
	EventSkipped        = "skipped"
	EventBatchPassed    = "batch_passed"
	EventChecked        = "checked"

	ModeDeploy   = "deploy"
	ModeRollback = "rollback"

	CauseHealth   = "health_check_failed"
	CauseReload   = "reload_failed"
	CauseHook     = "hook_failed"
	CauseCanceled = "canceled"
	CauseUpload   = "upload_failed"
	CauseSwitch   = "switch_failed"
	CauseOther    = "other"

	ErrKindTimeout     = "timeout"
	ErrKindRefused     = "refused"
	ErrKindCertificate = "certificate"
	ErrKindStatus      = "status"
	ErrKindOther       = "other"
)

type HealthReport struct {
	Attempts     int    `json:"Attempts"`
	Passed       bool   `json:"Passed"`
	LastStatus   int    `json:"LastStatus"`
	LastError    string `json:"LastError"`
	DurationMs   int64  `json:"DurationMs"`
	StableChecks int    `json:"StableChecks"`
}

type Event struct {
	Event             string        `json:"Event"`
	HostID            string        `json:"HostId"`
	Batch             int           `json:"Batch"`
	ReleaseID         string        `json:"ReleaseId"`
	PreviousReleaseID string        `json:"PreviousReleaseId"`
	Mode              string        `json:"Mode"`
	Reason            string        `json:"Reason"`
	Message           string        `json:"Message"`
	Health            *HealthReport `json:"Health"`

	CurrentReleaseID  string `json:"CurrentReleaseId,omitempty"`
	VerifiedReleaseID string `json:"VerifiedReleaseId,omitempty"`
	FreeBytes         int64  `json:"FreeBytes,omitempty"`
	Locked            bool   `json:"Locked,omitempty"`
}

type Reporter interface {
	Emit(Event)
}

type StreamReporter struct {
	mu     sync.Mutex
	w      io.Writer
	events int
}

func NewStreamReporter(w io.Writer) *StreamReporter { return &StreamReporter{w: w} }

func (r *StreamReporter) Emit(e Event) {
	raw, err := json.Marshal(e)
	if err != nil || len(raw) >= maxReportLine {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.events >= MaxReportEvents-1 {
		return
	}
	r.events++
	_, _ = r.w.Write(append(raw, '\n'))
}

type retagReporter struct {
	inner    Reporter
	from, to string
}

func NewRetagReporter(inner Reporter, from, to string) Reporter {
	return &retagReporter{inner: inner, from: from, to: to}
}

func (r *retagReporter) Emit(e Event) {
	if e.ReleaseID == r.from {
		e.ReleaseID = r.to
	}
	r.inner.Emit(e)
}

type DiscardReporter struct{}

func (DiscardReporter) Emit(Event) {}
