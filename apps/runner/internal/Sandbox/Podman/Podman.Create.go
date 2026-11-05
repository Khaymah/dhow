package podman

import (
	"fmt"
	"strconv"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

const (
	agentMount    = sandbox.AgentPath
	controlMount  = "/run/dhow/control"
	connectMount  = "/run/dhow/bin/connect"
	gitSSHMount   = "/run/dhow/bin/git-ssh"
	askpassMount  = "/run/dhow/bin/git-askpass" //nolint:gosec // A helper path, not a credential.
	secretsMount  = "/run/dhow/secrets"
	workspacePath = "/workspace"
	tmpPath       = "/tmp"
	secretsTmpfs  = 4 << 20
	shmBytes      = 64 << 20
	nofileLimit   = 4096
	entrypoint    = `["` + agentMount + `","__agent"]`
)

type layout struct {
	name       string
	kind       sandbox.Kind
	agentPath  string
	controlDir string
	inputsDir  string
	workDir    string
	idStart    int64
	hostStart  int64
}

func helperMounts(l layout) []string {
	var out []string
	if l.inputsDir != "" {
		out = append(out, "--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,ro=true,relabel=private,U=true", l.inputsDir, inputsMount))
	}
	if l.kind != sandbox.KindCheckout {
		return out
	}
	return append(out,
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,ro=true", l.agentPath, gitSSHMount),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,ro=true", l.agentPath, askpassMount),
	)
}

func createArgs(spec sandbox.PrepareSpec, l layout, seccompFile, imageID string) []string {
	r := spec.Resources
	mem := r.MemoryBytes
	if spec.DiskMode == config.DiskModeTmpfs {
		mem += r.DiskBytes
	}
	idmap := fmt.Sprintf("0:%d:%d", l.idStart, userNSSize)
	args := []string{
		"create",
		"--name", l.name,
		"--label", sandbox.LabelRunner + "=" + spec.RunnerID,
		"--label", sandbox.LabelAttempt + "=" + spec.AttemptID,
		"--pull=never",
		"--uidmap=" + idmap,
		"--gidmap=" + idmap,
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--security-opt=seccomp=" + seccompFile,
		"--read-only",
		"--read-only-tmpfs=false",
		"--ipc=private",
		"--uts=private",
		"--cgroupns=private",
		"--cgroups=split",
		"--pid=private",
		"--network=none",
		"--no-hosts",
		"--log-driver=none",
		"--init=false",
		"--http-proxy=false",
		"--unsetenv-all",
		"--ulimit=nofile=" + strconv.Itoa(nofileLimit) + ":" + strconv.Itoa(nofileLimit),
		"--ulimit=core=0:0",
		"--shm-size=" + strconv.Itoa(shmBytes),
		"--cpus=" + cpus(r.CPUMillis),
		"--memory=" + strconv.FormatInt(mem, 10),
		"--memory-swap=" + strconv.FormatInt(mem, 10),
		"--pids-limit=" + strconv.FormatInt(r.Pids, 10),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,ro=true", l.agentPath, agentMount),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,ro=true", l.agentPath, connectMount),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,ro=true,relabel=private,U=true", l.controlDir, controlMount),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s,relabel=private", l.workDir, workspacePath),
		"--mount", tmpfsMount(tmpPath, r.DiskBytes, "tmpfs-mode=1777,nosuid,nodev"),
		"--mount", tmpfsMount(secretsMount, secretsTmpfs, "tmpfs-mode=0700,noexec,nosuid,nodev,U"),
	}
	args = append(args, helperMounts(l)...)
	return append(args, "--workdir="+workspacePath, "--entrypoint="+entrypoint, "--", imageID)
}

func tmpfsMount(dst string, size int64, opts string) string {
	return fmt.Sprintf("type=tmpfs,dst=%s,tmpfs-size=%d,%s", dst, size, opts)
}

func cpus(millis int64) string {
	return strconv.FormatFloat(float64(millis)/float64(config.MillisPerCPU), 'f', 3, 64)
}
