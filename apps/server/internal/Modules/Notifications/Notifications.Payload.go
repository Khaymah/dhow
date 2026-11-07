package notifications

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	EventDeploymentFinished = "deployment.finished"
	EventApprovalRequested  = "deployment.approval.requested"
	EventRunFinished        = "run.finished"
	EventRunnerOffline      = "runner.offline"
	EventNewDeviceLogin     = "account.new_device_login"
	EventSecondFactor       = "account.second_factor_changed"
	EventMemberRemoved      = "member.removed"
	EventRoleChanged        = "member.role_changed"
	EventTest               = "notification.test"

	StatusSucceeded       = "succeeded"
	StatusFailed          = "failed"
	StatusCanceled        = "canceled"
	StatusRolledBack      = "rolled_back"
	StatusRollbackFailed  = "rollback_failed"
	StatusAwaitingApprove = "awaiting_approval"
	StatusTest            = "test"

	fieldMax = 120
)

func Events() []string {
	return []string{
		EventDeploymentFinished, EventApprovalRequested, EventRunFinished, EventRunnerOffline,
		EventNewDeviceLogin, EventSecondFactor, EventMemberRemoved, EventRoleChanged,
	}
}

func Statuses() []string {
	return []string{StatusSucceeded, StatusFailed, StatusCanceled, StatusRolledBack, StatusRollbackFailed, StatusAwaitingApprove}
}

func DefaultEvents() []string {
	return []string{EventDeploymentFinished, EventApprovalRequested, EventRunFinished}
}

func DefaultStatuses() []string {
	return []string{StatusFailed, StatusRolledBack, StatusRollbackFailed, StatusAwaitingApprove}
}

func ValidEvent(v string) bool  { return slices.Contains(Events(), v) }
func ValidStatus(v string) bool { return slices.Contains(Statuses(), v) }

type Payload struct {
	Event       string `json:"Event"`
	EventID     string `json:"EventId"`
	Time        string `json:"Time"`
	Status      string `json:"Status,omitempty"`
	Reason      string `json:"Reason,omitempty"`
	Project     string `json:"Project,omitempty"`
	Environment string `json:"Environment,omitempty"`
	Ref         string `json:"Ref,omitempty"`
	Run         int64  `json:"Run,omitempty"`
	Release     string `json:"Release,omitempty"`
	Runner      string `json:"Runner,omitempty"`
	Change      string `json:"Change,omitempty"`
	Browser     string `json:"Browser,omitempty"`
	Network     string `json:"Network,omitempty"`
	URL         string `json:"Url,omitempty"`
}

func (p Payload) Encode() ([]byte, error) {
	p.Project, p.Environment, p.Ref = clip(p.Project), clip(p.Environment), clip(p.Ref)
	p.Runner, p.Browser, p.Release = clip(p.Runner), clip(p.Browser), clip(p.Release)
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode a notification payload: %w", err)
	}
	return raw, nil
}

func DecodePayload(raw []byte) (Payload, error) {
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Payload{}, fmt.Errorf("decode a notification payload: %w", err)
	}
	return p, nil
}

func clip(v string) string {
	v = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, v)
	if utf8.RuneCountInString(v) <= fieldMax {
		return v
	}
	return string([]rune(v)[:fieldMax])
}

type Notice struct {
	EventID     uuid.UUID
	Event       string
	Status      string
	Environment string
	At          time.Time
	Payload     Payload
}

type RuleSpec struct {
	Events       []string
	Environments []string
	Statuses     []string
}

func (r RuleSpec) Matches(n Notice) bool {
	if !slices.Contains(r.Events, n.Event) {
		return false
	}
	if len(r.Environments) > 0 && (n.Environment == "" || !slices.Contains(r.Environments, n.Environment)) {
		return false
	}
	if n.Status != "" && !slices.Contains(r.Statuses, n.Status) {
		return false
	}
	return true
}

func shortDigest(v string) string {
	v = strings.TrimPrefix(v, "sha256:")
	if len(v) > 12 {
		return v[:12]
	}
	return v
}
