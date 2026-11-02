package podman

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	CommandMountWorkspace   = "__workspace-mount"
	CommandUnmountWorkspace = "__workspace-umount"
	workspaceMountFlags     = unix.MS_NOSUID | unix.MS_NODEV
	maxUserNSID             = 1 << 32
	minWorkspaceBytes       = 1 << 20
)

var (
	ErrMountUsage = errors.New("usage: __workspace-mount <attempts dir> <workspace> <size bytes> <uid> | __workspace-umount <attempts dir> <workspace>")
	ErrMountPath  = errors.New("the workspace must be <attempts dir>/<attempt>/workspace, as clean absolute paths without symlinks")
	ErrMountHost  = errors.New("the workspace is mounted only inside the runner's own user namespace (podman unshare), never as host root")
)

func checkWorkspacePath(root, dir string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Clean(dir) != dir || root == "/" {
		return ErrMountPath
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return ErrMountPath
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || parts[1] != workspaceDirName || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
		return ErrMountPath
	}
	for _, p := range []string{root, filepath.Dir(dir), dir} {
		real, err := filepath.EvalSymlinks(p)
		if err != nil || real != p {
			return ErrMountPath
		}
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return ErrMountPath
	}
	return nil
}

func inUserNamespace(uidMap string) bool {
	ranges, err := parseIDMap(uidMap)
	if err != nil || len(ranges) == 0 {
		return false
	}
	for _, r := range ranges {
		if r.holds(0) && r.inside == 0 && r.outside == 0 {
			return false
		}
	}
	return true
}

func mountOptions(args []string) (string, error) {
	size, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil || size < minWorkspaceBytes || size > jobspec.MaxDiskBytes || strconv.FormatInt(size, 10) != args[2] {
		return "", ErrMountUsage
	}
	id, err := strconv.ParseInt(args[3], 10, 64)
	if err != nil || id < 1 || id >= maxUserNSID || strconv.FormatInt(id, 10) != args[3] {
		return "", ErrMountUsage
	}
	return fmt.Sprintf("size=%d,mode=0755,uid=%d,gid=%d", size, id, id), nil
}

func RunWorkspaceMount(command string, args []string) error {
	raw, err := os.ReadFile("/proc/self/uid_map")
	if err != nil || !inUserNamespace(string(raw)) {
		return ErrMountHost
	}
	return workspaceMount(command, args, unix.Mount, unix.Unmount)
}

func workspaceMount(command string, args []string, mount func(string, string, string, uintptr, string) error, unmount func(string, int) error) error {
	switch {
	case command == CommandMountWorkspace && len(args) == 4:
		opts, err := mountOptions(args)
		if err != nil {
			return err
		}
		if err := checkWorkspacePath(args[0], args[1]); err != nil {
			return err
		}
		return mount("tmpfs", args[1], "tmpfs", workspaceMountFlags, opts)
	case command == CommandUnmountWorkspace && len(args) == 2:
		if err := checkWorkspacePath(args[0], args[1]); err != nil {
			return err
		}
		return unmount(args[1], unix.MNT_DETACH)
	default:
		return ErrMountUsage
	}
}
