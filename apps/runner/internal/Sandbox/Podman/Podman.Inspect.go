package podman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

var ErrPolicyMismatch = errors.New("the created sandbox does not match the required isolation policy")

type inspection struct {
	State struct {
		Running    bool   `json:"Running"`
		OOMKilled  bool   `json:"OOMKilled"`
		Status     string `json:"Status"`
		CgroupPath string `json:"CgroupPath"`
		Pid        int    `json:"Pid"`
	} `json:"State"`
	Config struct {
		Env         []string          `json:"Env"`
		Annotations map[string]string `json:"Annotations"`
	} `json:"Config"`
	EffectiveCaps []string    `json:"EffectiveCaps"`
	BoundingCaps  []string    `json:"BoundingCaps"`
	Mounts        []mountInfo `json:"Mounts"`
	HostConfig    struct {
		CapDrop        []string          `json:"CapDrop"`
		SecurityOpt    []string          `json:"SecurityOpt"`
		ReadonlyRootfs bool              `json:"ReadonlyRootfs"`
		NetworkMode    string            `json:"NetworkMode"`
		IpcMode        string            `json:"IpcMode"`
		PidMode        string            `json:"PidMode"`
		UTSMode        string            `json:"UTSMode"`
		CgroupMode     string            `json:"CgroupMode"`
		Memory         int64             `json:"Memory"`
		MemorySwap     int64             `json:"MemorySwap"`
		PidsLimit      int64             `json:"PidsLimit"`
		NanoCpus       int64             `json:"NanoCpus"`
		Privileged     bool              `json:"Privileged"`
		Tmpfs          map[string]string `json:"Tmpfs"`
		Annotations    map[string]string `json:"Annotations"`
	} `json:"HostConfig"`
}

type mountInfo struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

func inspect(ctx context.Context, r runner, name string) (*inspection, error) {
	out, err := r.run(ctx, "inspect", "--type=container", "--format", "json", name)
	if err != nil {
		return nil, err
	}
	var all []inspection
	if err := json.Unmarshal(out, &all); err != nil {
		return nil, fmt.Errorf("decode podman inspect: %w", err)
	}
	if len(all) != 1 {
		return nil, fmt.Errorf("podman inspect returned %d containers for %s", len(all), name)
	}
	return &all[0], nil
}

func verify(in *inspection, spec sandbox.PrepareSpec, l layout) error {
	var bad []string
	add := func(cond bool, msg string) {
		if !cond {
			bad = append(bad, msg)
		}
	}
	add(len(in.EffectiveCaps) == 0 && len(in.BoundingCaps) == 0, "the sandbox keeps capabilities")
	add(!in.HostConfig.Privileged, "the sandbox is privileged")
	add(noNewPrivileges(in.HostConfig.SecurityOpt), "no-new-privileges is not set")
	add(in.HostConfig.ReadonlyRootfs, "the root filesystem is writable")
	add(in.HostConfig.NetworkMode == "none", "the network is not none")
	add(private(in.HostConfig.IpcMode), "the IPC namespace is shared")
	add(private(in.HostConfig.PidMode), "the PID namespace is shared")
	add(private(in.HostConfig.UTSMode), "the UTS namespace is shared")
	add(private(in.HostConfig.CgroupMode), "the cgroup namespace is shared")
	verifyLimits(in, spec, add)
	verifyMounts(in, l, add)
	verifyImageInputs(in, add)
	if len(bad) > 0 {
		return fmt.Errorf("%w: %s", ErrPolicyMismatch, strings.Join(bad, "; "))
	}
	return nil
}

func verifyLimits(in *inspection, spec sandbox.PrepareSpec, add func(bool, string)) {
	mem := spec.Resources.MemoryBytes
	if spec.DiskMode == config.DiskModeTmpfs {
		mem += spec.Resources.DiskBytes
	}
	add(in.HostConfig.Memory == mem, "the memory limit is wrong")
	add(in.HostConfig.MemorySwap == mem, "swap is not disabled")
	add(in.HostConfig.PidsLimit == spec.Resources.Pids, "the process limit is wrong")
	add(in.HostConfig.NanoCpus == spec.Resources.CPUMillis*nanosPerMilliCPU, "the CPU limit is wrong")
}

const nanosPerMilliCPU = 1_000_000

func verifyMounts(in *inspection, l layout, add func(bool, string)) {
	sources := map[string]string{agentMount: l.agentPath, connectMount: l.agentPath, controlMount: l.controlDir, workspacePath: l.workDir}
	if l.kind == sandbox.KindCheckout {
		sources[gitSSHMount] = l.agentPath
		sources[askpassMount] = l.agentPath
	}
	for _, m := range in.Mounts {
		if m.Type == "tmpfs" {
			continue
		}
		want, ok := sources[m.Destination]
		add(ok, "an unexpected mount is present at "+m.Destination)
		if ok {
			add(sameFile(m.Source, want), "the mount at "+m.Destination+" comes from an unexpected source")
		}
		if m.Destination != workspacePath {
			add(!m.RW, "a control mount is writable at "+m.Destination)
		}
	}
	_, hasWorkspace := in.HostConfig.Tmpfs[workspacePath]
	add(!hasWorkspace, "the workspace is a tmpfs of its own, not the attempt's shared workspace")
	add(tmpfsHas(in, tmpPath, "nosuid", "nodev"), "the /tmp tmpfs is missing or allows setuid or devices")
	add(tmpfsHas(in, secretsMount, "noexec", "nosuid", "nodev"), "the secrets tmpfs is missing or allows exec, setuid or devices")
}

func verifyImageInputs(in *inspection, add func(bool, string)) {
	for _, annotations := range []map[string]string{in.Config.Annotations, in.HostConfig.Annotations} {
		for key := range annotations {
			add(!runtimeAnnotation(key), "the image sets the runtime annotation "+key)
		}
	}
	for _, kv := range in.Config.Env {
		name, _, _ := strings.Cut(kv, "=")
		add(!runtimeEnv(name), "the agent would start with "+name+" from the image")
	}
}

func runtimeAnnotation(key string) bool {
	for _, prefix := range []string{"run.oci.", "module.wasm.image/", "io.containers.trace-", "org.criu."} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func runtimeEnv(name string) bool {
	return strings.HasPrefix(name, "GO") || strings.HasPrefix(name, "LD_")
}

func tmpfsHas(in *inspection, dst string, want ...string) bool {
	opts, ok := in.HostConfig.Tmpfs[dst]
	if !ok {
		return false
	}
	have := strings.Split(opts, ",")
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

func noNewPrivileges(opts []string) bool {
	for _, o := range opts {
		switch o {
		case "no-new-privileges", "no-new-privileges=true", "no-new-privileges:true":
			return true
		}
	}
	return false
}

func private(mode string) bool { return mode == "private" }
