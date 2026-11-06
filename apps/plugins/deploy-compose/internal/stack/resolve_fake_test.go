package stack_test

import (
	"encoding/json"
	"os"
	"path"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func isConfig(argv []string) bool {
	return len(argv) >= 3 && argv[len(argv)-3] == "config" && argv[len(argv)-1] == "json"
}

func flagValue(argv []string, prefix string) []string {
	var out []string
	for _, a := range argv {
		if v, ok := strings.CutPrefix(a, prefix); ok {
			out = append(out, v)
		}
	}
	return out
}

func (f *fakeHost) resolvedConfig(argv []string) deployrelease.ExecResult {
	project := flagValue(argv, "--project-name=")[0]
	dir := flagValue(argv, "--project-directory=")[0]
	files := flagValue(argv, "--file=")
	var compose map[string]any
	var override struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	raw, err := os.ReadFile(f.path(files[0]))
	if err != nil || yaml.Unmarshal(raw, &compose) != nil {
		return deployrelease.ExecResult{ExitCode: 1, Output: []byte("cannot read the compose file")}
	}
	if raw, err = os.ReadFile(f.path(files[1])); err != nil || yaml.Unmarshal(raw, &override) != nil {
		return deployrelease.ExecResult{ExitCode: 1, Output: []byte("cannot read the override")}
	}
	model := map[string]any{"name": project, "services": map[string]any{}}
	services, _ := compose["services"].(map[string]any)
	for name, v := range services {
		svc, _ := v.(map[string]any)
		out := map[string]any{"image": override.Services[name].Image}
		for _, key := range []string{"privileged", "cap_add", "devices", "security_opt", "network_mode", "pid", "ipc", "uts"} {
			if val, ok := svc[key]; ok {
				out[key] = val
			}
		}
		out["volumes"] = resolvedVolumes(dir, svc["volumes"])
		model["services"].(map[string]any)[name] = out
	}
	if volumes, ok := compose["volumes"]; ok {
		model["volumes"] = volumes
	}
	if f.resolve != nil {
		f.resolve(model)
	}
	body, _ := json.MarshalIndent(model, "", "  ")
	return deployrelease.ExecResult{Output: append([]byte("WARN[0000] a warning before the model\n"), body...)}
}

func resolvedVolumes(dir string, v any) []any {
	var out []any
	list, _ := v.([]any)
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			out = append(out, item)
			continue
		}
		parts := strings.Split(text, ":")
		if len(parts) < 2 || !strings.ContainsAny(parts[0][:1], "./") {
			out = append(out, map[string]any{"type": "volume", "source": parts[0], "target": parts[len(parts)-1]})
			continue
		}
		source := parts[0]
		if !strings.HasPrefix(source, "/") {
			source = path.Join(dir, source)
		}
		out = append(out, map[string]any{"type": "bind", "source": source, "target": parts[1], "read_only": len(parts) == 3 && strings.Contains(parts[2], "ro")})
	}
	return out
}
