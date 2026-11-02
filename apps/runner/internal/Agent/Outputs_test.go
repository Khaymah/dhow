package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/agent/v1"
)

func TestRunExecRunsArgvWithoutAShellAndReturnsOutputs(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	conn, _ := startAgentWith(t, Options{SecretsDir: t.TempDir(), OutputPath: out})
	frame := &agentv1.Frame{Body: &agentv1.Frame_RunExec{RunExec: &agentv1.RunExec{
		Index: 3, Argv: []string{"/bin/sh", "-c", `printf 'url=https://example.com/$x\n' > "$DHOW_OUTPUT"; printf '%s' "$1"`, "probe", "$(touch /tmp/never)"},
		Env: map[string]string{"DHOW_OUTPUT": out},
	}}}
	if err := conn.Write(frame); err != nil {
		t.Fatal(err)
	}
	e := stepExitOf(t, conn, 5*time.Second)
	if e.GetIndex() != 3 || e.GetExitCode() != 0 || e.GetFailure() != agentv1.StepFailure_STEP_FAILURE_UNSPECIFIED {
		t.Fatalf("unexpected exit %+v", e)
	}
	if got := string(e.GetOutputs()); got != "url=https://example.com/$x\n" {
		t.Fatalf("outputs %q", got)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("the output file outlived the step")
	}
}

func TestOutputsThatAreTooLargeOrNotAFileFailTheStep(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	tests := []struct {
		name   string
		script string
	}{
		{"too large", "head -c " + strings.Repeat("9", 1) + "00000 /dev/zero > \"$DHOW_OUTPUT\"\n"},
		{"a symlink", "ln -s /etc/passwd \"$DHOW_OUTPUT\"\n"},
		{"a directory", "mkdir \"$DHOW_OUTPUT\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := startAgentWith(t, Options{SecretsDir: t.TempDir(), OutputPath: out})
			err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_RunStep{RunStep: &agentv1.RunStep{
				Index: 0, Script: tt.script, Env: map[string]string{"DHOW_OUTPUT": out},
			}}})
			if err != nil {
				t.Fatal(err)
			}
			e := stepExitOf(t, conn, 5*time.Second)
			if e.GetFailure() != agentv1.StepFailure_STEP_FAILURE_OUTPUTS || len(e.GetOutputs()) != 0 {
				t.Fatalf("unexpected exit %+v", e)
			}
			_ = os.RemoveAll(out)
		})
	}
}

func TestRunExecWithAnEmptyArgvDoesNotStart(t *testing.T) {
	conn, _ := startAgent(t)
	if err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_RunExec{RunExec: &agentv1.RunExec{Index: 1}}}); err != nil {
		t.Fatal(err)
	}
	if e := stepExitOf(t, conn, 5*time.Second); e.GetFailure() != agentv1.StepFailure_STEP_FAILURE_START_FAILED {
		t.Fatalf("unexpected exit %+v", e)
	}
}
