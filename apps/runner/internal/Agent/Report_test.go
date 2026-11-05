package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/agent/v1"
)

func collectReports(t *testing.T, conn interface {
	Read() (*agentv1.Frame, error)
}, within time.Duration,
) (string, *agentv1.StepExit) {
	t.Helper()
	type result struct {
		text string
		exit *agentv1.StepExit
	}
	got := make(chan result, 1)
	go func() {
		var sb strings.Builder
		for {
			f, err := conn.Read()
			if err != nil {
				got <- result{text: sb.String()}
				return
			}
			if r := f.GetReport(); r != nil {
				sb.Write(r.GetData())
			}
			if e := f.GetStepExit(); e != nil {
				got <- result{text: sb.String(), exit: e}
				return
			}
		}
	}()
	select {
	case r := <-got:
		return r.text, r.exit
	case <-time.After(within):
		t.Fatalf("the step did not end within %s", within)
		return "", nil
	}
}

func TestTheAgentSendsTheLinesAPluginWritesToItsReportFileBeforeTheStepEnds(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report")
	conn, _ := startAgentWith(t, Options{SecretsDir: t.TempDir(), ReportPath: report})
	err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_RunExec{RunExec: &agentv1.RunExec{
		Index: 2, Argv: []string{"/usr/bin/tee", report}, Stdin: true, Report: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []string{"{\"Event\":\"verified\"}\n", "{\"Event\":\"par"} {
		if err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_Input{Input: &agentv1.Input{StepIndex: 2, Data: []byte(chunk)}}}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * reportPoll)
	}
	if err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_Input{Input: &agentv1.Input{StepIndex: 2, Eof: true}}}); err != nil {
		t.Fatal(err)
	}
	text, exit := collectReports(t, conn, 10*time.Second)
	if exit == nil || exit.GetExitCode() != 0 {
		t.Fatalf("the step did not end cleanly: %v", exit)
	}
	if want := "{\"Event\":\"verified\"}\n{\"Event\":\"par\n"; text != want {
		t.Fatalf("want %q, got %q", want, text)
	}
	if _, err := os.Stat(report); !os.IsNotExist(err) {
		t.Fatalf("the report file stays after the step: %v", err)
	}
}

func TestAReportFileIsReadOnlyUpToItsLimitAndNeverThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	report := filepath.Join(dir, "report")
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("{\"Event\":\"leak\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, report); err != nil {
		t.Fatal(err)
	}
	tail := &reportTail{path: report}
	if got := tail.next(true); len(got) != 0 {
		t.Fatalf("a symlink was followed: %q", got)
	}
	if err := os.Remove(report); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("{\"Event\":\"health\"}\n", (MaxReportFileBytes/19)+100)
	if err := os.WriteFile(report, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	var total int
	for range 3 {
		total += len(tail.next(true))
	}
	if total > MaxReportFileBytes {
		t.Fatalf("read %d bytes of a report limited to %d", total, MaxReportFileBytes)
	}
}
