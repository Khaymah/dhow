package podman

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

type mountCall struct {
	target, fstype, data string
	flags                uintptr
	unmountFlags         int
}

func recorder(calls *[]mountCall) (func(string, string, string, uintptr, string) error, func(string, int) error) {
	return func(_, target, fstype string, flags uintptr, data string) error {
			*calls = append(*calls, mountCall{target: target, fstype: fstype, flags: flags, data: data})
			return nil
		}, func(target string, flags int) error {
			*calls = append(*calls, mountCall{target: target, unmountFlags: flags})
			return nil
		}
}

func TestTheMountHelperMountsOnlyTheWorkspaceWithFixedFlags(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attempts")
	ws := filepath.Join(root, "a1", "workspace")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	var calls []mountCall
	mount, unmount := recorder(&calls)
	if err := workspaceMount(CommandMountWorkspace, []string{root, ws, "67108864", "65537"}, mount, unmount); err != nil {
		t.Fatal(err)
	}
	if err := workspaceMount(CommandUnmountWorkspace, []string{root, ws}, mount, unmount); err != nil {
		t.Fatal(err)
	}
	want := []mountCall{
		{target: ws, fstype: "tmpfs", flags: unix.MS_NOSUID | unix.MS_NODEV, data: "size=67108864,mode=0755,uid=65537,gid=65537"},
		{target: ws, unmountFlags: unix.MNT_DETACH},
	}
	if len(calls) != 2 || calls[0] != want[0] || calls[1] != want[1] {
		t.Fatalf("got %+v", calls)
	}
}

func TestTheMountHelperRefusesEverythingElse(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "attempts")
	ws := filepath.Join(root, "a1", "workspace")
	other := filepath.Join(base, "elsewhere", "workspace")
	for _, d := range []string{ws, other, filepath.Join(root, "a2")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(other, filepath.Join(root, "a2", "workspace")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(other), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a1", "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{"outside the attempts dir", CommandMountWorkspace, []string{root, other, "67108864", "65537"}},
		{"a parent directory step", CommandMountWorkspace, []string{root, root + "/a1/../a1/workspace", "67108864", "65537"}},
		{"the attempts dir itself", CommandMountWorkspace, []string{root, root, "67108864", "65537"}},
		{"not a workspace dir", CommandMountWorkspace, []string{root, filepath.Join(root, "a1"), "67108864", "65537"}},
		{"too deep", CommandMountWorkspace, []string{root, filepath.Join(ws, "workspace"), "67108864", "65537"}},
		{"a symlinked workspace", CommandMountWorkspace, []string{root, filepath.Join(root, "a2", "workspace"), "67108864", "65537"}},
		{"a symlinked attempt", CommandMountWorkspace, []string{root, filepath.Join(root, "linked", "workspace"), "67108864", "65537"}},
		{"a relative root", CommandMountWorkspace, []string{"attempts", "attempts/a1/workspace", "67108864", "65537"}},
		{"the filesystem root", CommandMountWorkspace, []string{"/", "/a1/workspace", "67108864", "65537"}},
		{"a size with a unit", CommandMountWorkspace, []string{root, ws, "64m", "65537"}},
		{"a size below the minimum", CommandMountWorkspace, []string{root, ws, "4096", "65537"}},
		{"a size above the maximum", CommandMountWorkspace, []string{root, ws, "1099511627776", "65537"}},
		{"extra mount options", CommandMountWorkspace, []string{root, ws, "67108864,exec", "65537"}},
		{"uid 0", CommandMountWorkspace, []string{root, ws, "67108864", "0"}},
		{"a uid with options", CommandMountWorkspace, []string{root, ws, "67108864", "1,suid"}},
		{"a flag argument", CommandMountWorkspace, []string{root, ws, "67108864", "65537", "--bind"}},
		{"too few arguments", CommandMountWorkspace, []string{root, ws}},
		{"unmount outside", CommandUnmountWorkspace, []string{root, other}},
		{"unmount with a flag", CommandUnmountWorkspace, []string{root, ws, "--lazy"}},
		{"another command", "__mount", []string{root, ws}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []mountCall
			mount, unmount := recorder(&calls)
			if err := workspaceMount(tt.command, tt.args, mount, unmount); err == nil {
				t.Fatal("accepted")
			}
			if len(calls) != 0 {
				t.Fatalf("mounted anyway: %+v", calls)
			}
		})
	}
}

func TestTheMountHelperRunsOnlyInsideAUserNamespace(t *testing.T) {
	if inUserNamespace("         0          0 4294967295\n") {
		t.Fatal("host root is not a user namespace")
	}
	if !inUserNamespace("         0       1000          1\n         1     200000    1048576\n") {
		t.Fatal("podman unshare is a user namespace")
	}
	if inUserNamespace("") {
		t.Fatal("an unreadable map is not trusted")
	}
}

func TestTheDriverNeverRunsTheMountBinaries(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "podman.log")
	fake := filepath.Join(dir, "podman")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\n" +
		"if [ \"$1 $2\" = \"unshare cat\" ]; then printf '0 1000 1\\n1 200000 1048576\\n'; fi\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	attempts := filepath.Join(dir, "state", "attempts")
	if err := os.MkdirAll(filepath.Join(attempts, "a1"), 0o700); err != nil {
		t.Fatal(err)
	}
	d := &Driver{binary: fake, attemptsDir: attempts, agentPath: "/usr/local/bin/dhow-runner", live: map[string][]idRange{}}
	ctx := context.Background()
	ws, err := d.Workspace(ctx, sandbox.WorkspaceSpec{AttemptID: "a1", Dir: filepath.Join(attempts, "a1"), DiskMode: config.DiskModeTmpfs, DiskBytes: 64 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Destroy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.Cleanup(ctx, "runner"); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	sawMount, sawUnmount := false, false
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		f := strings.Fields(line)
		if len(f) > 1 && f[0] == "unshare" {
			switch filepath.Base(f[1]) {
			case "mount", "umount":
				t.Fatalf("the driver runs %s, which the hardened unit makes unreachable: %s", f[1], line)
			}
			sawMount = sawMount || (len(f) > 2 && f[1] == d.agentPath && f[2] == CommandMountWorkspace)
			sawUnmount = sawUnmount || (len(f) > 2 && f[1] == d.agentPath && f[2] == CommandUnmountWorkspace)
		}
	}
	if !sawMount || !sawUnmount {
		t.Fatalf("the runner binary did not mount and unmount the workspace:\n%s", raw)
	}
}
