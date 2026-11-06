package edge_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	reportMaxBatch    = 10
	reportMaxAttempts = 1000
	reportMaxStatus   = 999
	reportMaxDuration = 3_600_000
	reportMaxStable   = 100
	reportMaxHostID   = 64
	reportMaxMessage  = 4 * environments.MaxReportMessage
)

var (
	reportModes   = []string{"", "deploy", "rollback"}
	reportCauses  = []string{"health_check_failed", "reload_failed", "hook_failed", "canceled", "upload_failed", "switch_failed", "other"}
	reportErrors  = []string{"", "timeout", "refused", "certificate", "status", "other"}
	secretShaped  = regexp.MustCompile(`[A-Za-z0-9+/=_-]{20,}|-----[A-Z ]+-----`)
	reportKindMax = int32(runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_CHECKED)
)

func FuzzDeployReport(f *testing.F) {
	f.Add(int32(runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_VERIFIED), "0192f0f0-0000-7000-8000-000000000001", int32(1), "10-aaaaaaaa-0", "9-aaaaaaaa-0", "deploy", "", "ok",
		true, int32(3), int32(200), "", int64(1200), int32(2))
	f.Add(int32(runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_ROLLED_BACK), "h", int32(0), "r1", "", "rollback", "health_check_failed", "the health check failed",
		true, int32(60), int32(503), "status", int64(60000), int32(0))
	f.Add(int32(runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_FAILED), "h", int32(-1), "../etc", "-rf", "sideways", "mystery", strings.Repeat("A", 300),
		true, int32(-5), int32(1000), "dns", int64(-1), int32(101))
	f.Add(int32(0), "", int32(0), "", "", "", "", "", false, int32(0), int32(0), "", int64(0), int32(0))
	f.Add(int32(99), strings.Repeat("h", 65), int32(11), "a\x00b", "a b", "deploy", "other", strings.Repeat("Ab1+", 12), false, int32(0), int32(0), "", int64(0), int32(0))
	f.Fuzz(func(t *testing.T, kind int32, hostID string, batch int32, releaseID, previous, mode, reason, message string,
		withHealth bool, attempts, status int32, lastError string, durationMs int64, stable int32,
	) {
		ev := &runnerv1.DeployEvent{
			Kind: runnerv1.DeployEventKind(kind), HostId: hostID, Batch: batch, ReleaseId: releaseID, PreviousReleaseId: previous,
			Mode: mode, Reason: reason, Message: message,
		}
		if withHealth {
			ev.Health = &runnerv1.DeployHealth{Attempts: attempts, LastStatus: status, LastError: lastError, DurationMs: durationMs, StableChecks: stable}
		}
		if cleaned := environments.ReportMessage(message); !utf8.ValidString(cleaned) || utf8.RuneCountInString(cleaned) > environments.MaxReportMessage ||
			strings.IndexFunc(cleaned, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 || secretShaped.MatchString(cleaned) {
			t.Fatalf("the message %q was stored as %q", message, cleaned)
		}
		if !environments.ValidDeployEvent(ev) {
			return
		}
		if kind <= 0 || kind > reportKindMax {
			t.Fatalf("accepted the event kind %d", kind)
		}
		if len(hostID) > reportMaxHostID || len(message) > reportMaxMessage {
			t.Fatalf("accepted a host id of %d bytes and a message of %d bytes", len(hostID), len(message))
		}
		if kind == int32(runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_CHECKED) {
			if hostID == "" || releaseID != "" || previous != "" || batch != 0 || mode != "" || withHealth {
				t.Fatalf("accepted a check event that carries deploy fields")
			}
			return
		}
		if batch < 0 || batch > reportMaxBatch {
			t.Fatalf("accepted the batch %d", batch)
		}
		for _, id := range []string{releaseID, previous} {
			if id != "" && !jobspec.ValidReleaseID(id) {
				t.Fatalf("accepted the release id %q", id)
			}
		}
		if !slices.Contains(reportModes, mode) || (reason != "" && !slices.Contains(reportCauses, reason)) {
			t.Fatalf("accepted the mode %q and the reason %q", mode, reason)
		}
		if withHealth {
			ok := attempts >= 0 && attempts <= reportMaxAttempts && status >= 0 && status <= reportMaxStatus &&
				durationMs >= 0 && durationMs <= reportMaxDuration && stable >= 0 && stable <= reportMaxStable && slices.Contains(reportErrors, lastError)
			if !ok {
				t.Fatalf("accepted the health numbers %d %d %d %d %q", attempts, status, durationMs, stable, lastError)
			}
		}
	})
}
