package executor

import (
	"strings"
	"testing"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func identityMask(v string) string { return v }

func TestAReportLineBecomesATypedEventAndAnythingElseIsRefused(t *testing.T) {
	ev, ok := parseReportLine([]byte(`{"Event":"health","HostId":"h1","Batch":2,"Message":"m","Health":{"Attempts":3,"Passed":true,"LastStatus":200,"LastError":"","DurationMs":10,"StableChecks":3}}`), strings.ToUpper)
	if !ok || ev.GetKind() != runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_HEALTH || ev.GetHostId() != "h1" || ev.GetBatch() != 2 {
		t.Fatalf("a valid line was refused: %v %v", ev, ok)
	}
	if ev.GetMessage() != "M" || ev.GetHealth().GetAttempts() != 3 || !ev.GetHealth().GetPassed() {
		t.Fatalf("the fields did not carry over through the mask: %v", ev)
	}
	for _, bad := range []string{
		``, `{}`, `[]`, `null`, `{"Event":""}`, `{"Event":"nope"}`, `{"Event":"verified","Extra":1}`, `{"Event":"verified"} {"Event":"verified"}`,
		`{"Event":"verified","Batch":"1"}`, `{"Event":"verified"`, strings.Repeat("a", maxReportLine+1),
	} {
		if _, ok := parseReportLine([]byte(bad), identityMask); ok {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

func TestANumberBeyondTheRangeIsClampedNotWrapped(t *testing.T) {
	ev, ok := parseReportLine([]byte(`{"Event":"batch_passed","Batch":99999999999}`), identityMask)
	if !ok || ev.GetBatch() <= 0 {
		t.Fatalf("a huge batch wrapped to %v", ev.GetBatch())
	}
}

func FuzzReportLine(f *testing.F) {
	f.Add([]byte(`{"Event":"verified","HostId":"x"}`))
	f.Add([]byte(`{"Event":"health","Health":{"Attempts":-1}}`))
	f.Add([]byte("\x00{"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		ev, ok := parseReportLine(raw, identityMask)
		if !ok {
			return
		}
		if ev.GetKind() == runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_UNSPECIFIED {
			t.Fatalf("an accepted line has no kind: %q", raw)
		}
		if len(ev.GetMessage())+len(ev.GetHostId())+len(ev.GetReason()) > len(raw) {
			t.Fatalf("an event is larger than the line it came from: %q", raw)
		}
	})
}
