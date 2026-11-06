package environments

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	MaxReportEvents  = 100
	MaxReportMessage = 256
	maxBatch         = 10
	maxAttempts      = 1000
	maxStatus        = 999
	maxDurationMs    = 3_600_000
	maxStable        = 100

	hostPending        = "pending"
	hostUploaded       = "uploaded"
	hostSwitched       = "switched"
	hostVerified       = "verified"
	hostFailed         = "failed"
	hostRolledBack     = "rolled_back"
	hostRollbackFailed = "rollback_failed"
	hostUnknown        = "unknown"
	hostSkipped        = "skipped"

	groupRunning        = "running"
	groupSucceeded      = "succeeded"
	groupFailed         = "failed"
	groupRolledBack     = "rolled_back"
	groupRollbackFailed = "rollback_failed"
	groupUnknown        = "unknown"

	modeDeploy   = "deploy"
	modeRollback = "rollback"
)

var (
	releaseIDShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	secretLike     = regexp.MustCompile(`[A-Za-z0-9+/=_-]{20,}|-----[A-Z ]+-----`)
	rollbackCauses = []string{"health_check_failed", "reload_failed", "hook_failed", "canceled", "upload_failed", "switch_failed", "other"}
	healthErrors   = []string{"", "timeout", "refused", "certificate", "status", "other"}
)

type hostMove struct {
	from []string
	to   string
}

var hostMoves = map[runnerv1.DeployEventKind]hostMove{
	runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_UPLOADED:        {[]string{hostPending}, hostUploaded},
	runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_SWITCHED:        {[]string{hostUploaded}, hostSwitched},
	runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_VERIFIED:        {[]string{hostSwitched}, hostVerified},
	runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_FAILED:          {[]string{hostPending, hostUploaded, hostSwitched}, hostFailed},
	runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_SKIPPED:         {[]string{hostPending, hostUploaded}, hostSkipped},
	runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLED_BACK:     {[]string{hostSwitched, hostVerified}, hostRolledBack},
	runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLBACK_FAILED: {[]string{hostSwitched, hostVerified}, hostRollbackFailed},
}

var healthStates = []string{hostSwitched, hostVerified, hostRolledBack}

func hostFinal(state string) bool {
	return slices.Contains([]string{hostVerified, hostFailed, hostRolledBack, hostRollbackFailed, hostUnknown, hostSkipped}, state)
}

type healthRecord struct {
	Attempts     int    `json:"Attempts"`
	Passed       bool   `json:"Passed"`
	LastStatus   int    `json:"LastStatus"`
	LastError    string `json:"LastError"`
	DurationMs   int64  `json:"DurationMs"`
	StableChecks int    `json:"StableChecks"`
}

func cleanMessage(v string) string {
	v = strings.ToValidUTF8(v, "")
	v = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, v)
	if utf8.RuneCountInString(v) > MaxReportMessage {
		v = string([]rune(v)[:MaxReportMessage])
	}
	return v
}

func redactMessage(v string) string {
	return secretLike.ReplaceAllString(v, "***")
}

func validEvent(ev *runnerv1.DeployEvent) *rejection {
	switch {
	case ev == nil || ev.GetKind() == runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_UNSPECIFIED:
		return errInvalidReport("Kind")
	case ev.GetKind() == runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_CHECKED:
		return validCheck(ev)
	case ev.GetCurrentReleaseId() != "" || ev.GetVerifiedReleaseId() != "" || ev.GetFreeBytes() != 0 || ev.GetLocked():
		return errInvalidReport("Kind")
	case ev.GetBatch() < 0 || ev.GetBatch() > maxBatch:
		return errInvalidReport("Batch")
	case ev.GetReleaseId() != "" && !releaseIDShape.MatchString(ev.GetReleaseId()):
		return errInvalidReport("ReleaseId")
	case ev.GetPreviousReleaseId() != "" && !releaseIDShape.MatchString(ev.GetPreviousReleaseId()):
		return errInvalidReport("PreviousReleaseId")
	case ev.GetMode() != "" && ev.GetMode() != modeDeploy && ev.GetMode() != modeRollback:
		return errInvalidReport("Mode")
	case ev.GetReason() != "" && !slices.Contains(rollbackCauses, ev.GetReason()):
		return errInvalidReport("Reason")
	case len(ev.GetMessage()) > 4*MaxReportMessage || len(ev.GetHostId()) > 64:
		return errInvalidReport("Message")
	}
	if h := ev.GetHealth(); h != nil {
		ok := h.GetAttempts() >= 0 && h.GetAttempts() <= maxAttempts && h.GetLastStatus() >= 0 && h.GetLastStatus() <= maxStatus &&
			h.GetDurationMs() >= 0 && h.GetDurationMs() <= maxDurationMs && h.GetStableChecks() >= 0 && h.GetStableChecks() <= maxStable &&
			slices.Contains(healthErrors, h.GetLastError())
		if !ok {
			return errInvalidReport("Health")
		}
	}
	return nil
}

func healthJSON(h *runnerv1.DeployHealth) []byte {
	raw, err := json.Marshal(healthRecord{
		Attempts: int(h.GetAttempts()), Passed: h.GetPassed(), LastStatus: int(h.GetLastStatus()), LastError: h.GetLastError(),
		DurationMs: h.GetDurationMs(), StableChecks: int(h.GetStableChecks()),
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}

type groupVerdict struct {
	state    string
	skip     []string
	unknown  []string
	rejected bool
}

func countStates(states []string, in ...string) int {
	n := 0
	for _, s := range states {
		if slices.Contains(in, s) {
			n++
		}
	}
	return n
}

func groupEnd(kind runnerv1.DeployEventKind, states []string) groupVerdict {
	changed := []string{hostSwitched, hostVerified, hostRolledBack, hostRollbackFailed, hostUnknown}
	open := []string{hostPending, hostUploaded}
	switch kind {
	case runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_VERIFIED:
		return groupVerdict{state: groupSucceeded, rejected: countStates(states, hostVerified) != len(states)}
	case runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_FAILED:
		return groupVerdict{state: groupFailed, skip: open, rejected: countStates(states, changed...) > 0}
	case runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLED_BACK:
		return groupVerdict{
			state: groupRolledBack, skip: open,
			rejected: countStates(states, hostSwitched, hostVerified, hostRollbackFailed, hostUnknown) > 0 || countStates(states, hostRolledBack) == 0,
		}
	case runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLBACK_FAILED:
		return groupVerdict{
			state: groupRollbackFailed, skip: open, unknown: []string{hostSwitched, hostVerified},
			rejected: countStates(states, hostRollbackFailed) == 0,
		}
	default:
		return groupVerdict{rejected: true}
	}
}
