package podman

import (
	"errors"
	"testing"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

func TestTheUserNamespaceMapIsChecked(t *testing.T) {
	const runner = 1001
	other := [][]idRange{{{inside: 0, outside: 200000, count: 65536}}}
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"one private range", "0 100000 65536\n", true},
		{"the runner's own uid", "0 1001 1\n1 100000 65535\n", false},
		{"host root", "0 0 65536\n", false},
		{"too many ids", "0 100000 131072\n", false},
		{"overlaps another sandbox", "0 230000 65536\n", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranges, err := parseIDMap(tt.raw)
			if err == nil {
				err = verifyIDMap(ranges, runner, other)
			}
			if tt.ok && err != nil {
				t.Fatalf("want ok, got %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrPolicyMismatch) {
				t.Fatalf("want a policy mismatch, got %v", err)
			}
		})
	}
}

func TestTheMapMustBeExactlyTheAttemptRange(t *testing.T) {
	if err := exactMap([]idRange{{inside: 0, outside: 265536, count: userNSSize}}, 265536); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]idRange{
		{{inside: 0, outside: 200000, count: userNSSize}},
		{{inside: 0, outside: 265536, count: 1000}},
		{{inside: 0, outside: 265536, count: 1000}, {inside: 1000, outside: 266536, count: 64536}},
		nil,
	} {
		if err := exactMap(bad, 265536); !errors.Is(err, ErrPolicyMismatch) {
			t.Fatalf("want a policy mismatch for %v, got %v", bad, err)
		}
	}
}

func TestSlotsComeFromTheRunnerSubidRange(t *testing.T) {
	r, err := parseSubidRange("         0       1000          1\n         1     200000    1000000\n")
	if err != nil {
		t.Fatal(err)
	}
	s := newSlots(r)
	if s.count != 15 {
		t.Fatalf("1000000 sub-ids give 15 slots, got %d", s.count)
	}
	a, err := s.take("a")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := s.take("a")
	b, _ := s.take("b")
	if a != again || a == b {
		t.Fatalf("an attempt keeps its slot and two attempts never share one: %d %d %d", a, again, b)
	}
	if s.innerStart(b) != 1+userNSSize || s.hostStart(b) != 200000+userNSSize {
		t.Fatalf("slot %d maps to %d inside and %d on the host", b, s.innerStart(b), s.hostStart(b))
	}
	for i := 2; i < 15; i++ {
		if _, err := s.take(string(rune('c' + i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.take("full"); !errors.Is(err, sandbox.ErrNoIDRange) {
		t.Fatalf("want ErrNoIDRange, got %v", err)
	}
	s.release("a")
	if got, err := s.take("late"); err != nil || got != a {
		t.Fatalf("a released slot is reused: %d %v", got, err)
	}
	if _, err := parseSubidRange("0 1000 1\n"); !errors.Is(err, ErrIDRange) {
		t.Fatalf("a runner without sub-ids has no slots: %v", err)
	}
}

func TestTheWorkspaceMountIsCheckedBeforeEachSandbox(t *testing.T) {
	const dir = "/state/attempts/a/workspace"
	line := func(opts, fs, super string) string {
		return "290 280 0:64 / " + dir + " " + opts + " shared:1 - " + fs + " tmpfs " + super + "\n"
	}
	good := "1 0 0:1 / / rw - ext4 /dev/vda rw\n" + line("rw,nosuid,nodev,relatime", "tmpfs", "rw,size=65536k,mode=755,uid=65537,gid=65537")
	if err := checkWorkspaceMount(good, dir, 64<<20); err != nil {
		t.Fatal(err)
	}
	for name, info := range map[string]string{
		"nothing mounted":          "1 0 0:1 / / rw - ext4 /dev/vda rw\n",
		"setuid allowed":           line("rw,nodev,relatime", "tmpfs", "rw,size=65536k"),
		"devices allowed":          line("rw,nosuid,relatime", "tmpfs", "rw,size=65536k"),
		"not a tmpfs":              line("rw,nosuid,nodev", "ext4", "rw"),
		"bigger than the job disk": line("rw,nosuid,nodev", "tmpfs", "rw,size=131072k"),
		"no size":                  line("rw,nosuid,nodev", "tmpfs", "rw,mode=755"),
		"a later mount over it":    good + line("rw,relatime", "tmpfs", "rw,size=65536k"),
		"a bind of another dir":    good + "300 290 0:70 / " + dir + " rw,nosuid,nodev - ext4 /dev/vda rw\n",
	} {
		if err := checkWorkspaceMount(info, dir, 64<<20); !errors.Is(err, sandbox.ErrWorkspace) {
			t.Fatalf("%s: want ErrWorkspace, got %v", name, err)
		}
	}
}
