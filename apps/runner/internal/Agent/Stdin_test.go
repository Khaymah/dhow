package agent

import (
	"strings"
	"testing"
	"time"

	agentv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/agent/v1"
)

func TestAnExecReadsTheInputTheRunnerStreams(t *testing.T) {
	conn, _ := startAgent(t)
	out := make(chan string, 1)
	go func() {
		var b strings.Builder
		for {
			f, err := conn.Read()
			if err != nil {
				out <- b.String()
				return
			}
			if o := f.GetOutput(); o != nil {
				b.Write(o.GetData())
			}
			if f.GetStepExit() != nil {
				out <- b.String()
				return
			}
		}
	}()
	if err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_RunExec{RunExec: &agentv1.RunExec{
		Index: -2, Argv: []string{Shell, "-c", "wc -c"}, Stdin: true,
	}}}); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"hello ", "agent", ""} {
		in := &agentv1.Input{StepIndex: -2, Data: []byte(part), Eof: part == ""}
		if err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_Input{Input: in}}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case got := <-out:
		if strings.TrimSpace(got) != "11" {
			t.Fatalf("the exec read %q, want 11 bytes", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the exec did not end")
	}
}

func TestInputForAnotherStepIsIgnored(t *testing.T) {
	conn, _ := startAgent(t)
	if err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_RunExec{RunExec: &agentv1.RunExec{
		Index: 3, Argv: []string{Shell, "-c", "wc -c"}, Stdin: true,
	}}}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []*agentv1.Input{{StepIndex: 4, Data: []byte("not mine")}, {StepIndex: 3, Eof: true}} {
		if err := conn.Write(&agentv1.Frame{Body: &agentv1.Frame_Input{Input: in}}); err != nil {
			t.Fatal(err)
		}
	}
	if e := stepExitOf(t, conn, 10*time.Second); e.GetExitCode() != 0 {
		t.Fatalf("exit %d", e.GetExitCode())
	}
}
