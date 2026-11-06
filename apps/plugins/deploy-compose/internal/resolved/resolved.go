package resolved

import (
	"bytes"
	"encoding/json"
	"path"
	"strings"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

const Code = "COMPOSE_RESOLVED_UNSAFE"

type Expect struct {
	Project     string
	ReleaseDir  string
	Images      map[string]string
	AllowUnsafe bool
}

func bad(detail string) error { return &policy.Violation{Code: Code, Detail: detail} }

func extract(out []byte) (map[string]any, error) {
	start := -1
	if bytes.HasPrefix(out, []byte("{")) {
		start = 0
	} else if i := bytes.Index(out, []byte("\n{")); i >= 0 {
		start = i + 1
	}
	if start < 0 {
		return nil, bad("the resolved model cannot be read")
	}
	var model map[string]any
	if err := json.NewDecoder(bytes.NewReader(out[start:])).Decode(&model); err != nil || model == nil {
		return nil, bad("the resolved model cannot be read")
	}
	return model, nil
}

func Check(out []byte, e Expect) error {
	model, err := extract(out)
	if err != nil {
		return err
	}
	if name, _ := model["name"].(string); name != e.Project || name == "" {
		return bad("the project name is not the one of the target")
	}
	services, _ := model["services"].(map[string]any)
	if len(services) == 0 || len(services) != len(e.Images) {
		return bad("the services are not the ones that were checked")
	}
	for name, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			return bad("a service is not a mapping")
		}
		if err := checkService(name, svc, e); err != nil {
			return err
		}
	}
	return checkTop(model)
}

func checkService(name string, svc map[string]any, e Expect) error {
	if image, _ := svc["image"].(string); image == "" || image != e.Images[name] {
		return bad("the image of service " + name + " is not the pinned one")
	}
	if _, ok := svc["build"]; ok {
		return bad("service " + name + " builds an image")
	}
	if err := checkVolumes(name, svc["volumes"], e); err != nil {
		return err
	}
	if e.AllowUnsafe {
		return nil
	}
	return checkSecurity(name, svc)
}

func nonEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case string:
		return x != ""
	case bool:
		return x
	}
	return true
}

func checkSecurity(name string, svc map[string]any) error {
	for _, key := range []string{"privileged", "use_api_socket", "cap_add", "devices", "volumes_from", "device_cgroup_rules", "gpus"} {
		if nonEmpty(svc[key]) {
			return bad("service " + name + " sets " + key)
		}
	}
	opts, _ := svc["security_opt"].([]any)
	for _, o := range opts {
		text, _ := o.(string)
		if text != "no-new-privileges" && text != "no-new-privileges=true" && text != "no-new-privileges:true" {
			return bad("service " + name + " has a security option")
		}
	}
	for _, key := range []string{"pid", "ipc", "uts"} {
		text, _ := svc[key].(string)
		if text == "host" || text == "shareable" || strings.HasPrefix(text, "container:") || strings.HasPrefix(text, "service:") {
			return bad("service " + name + " shares a namespace through " + key)
		}
	}
	if mode, _ := svc["network_mode"].(string); mode != "" && mode != "none" {
		return bad("service " + name + " has a network mode")
	}
	return nil
}

func checkVolumes(name string, v any, e Expect) error {
	list, _ := v.([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return bad("a volume of service " + name + " is not a mapping")
		}
		kind, _ := m["type"].(string)
		switch kind {
		case "volume", "tmpfs", "image":
		case "bind":
			if err := checkBind(name, m, e); err != nil {
				return err
			}
		default:
			return bad("service " + name + " has a mount of a type that is not allowed")
		}
	}
	return nil
}

func checkBind(name string, m map[string]any, e Expect) error {
	source, _ := m["source"].(string)
	if !path.IsAbs(source) {
		return bad("a bind of service " + name + " is not an absolute path")
	}
	if e.AllowUnsafe {
		return nil
	}
	clean := path.Clean(source)
	if !strings.HasPrefix(clean, path.Clean(e.ReleaseDir)+"/") {
		return bad("a bind of service " + name + " is outside the release")
	}
	if ro, _ := m["read_only"].(bool); !ro {
		return bad("a bind of service " + name + " is writable")
	}
	return nil
}

func checkTop(model map[string]any) error {
	volumes, _ := model["volumes"].(map[string]any)
	for name, raw := range volumes {
		v, _ := raw.(map[string]any)
		if driver, _ := v["driver"].(string); driver != "" && driver != "local" {
			return bad("volume " + name + " has a driver")
		}
		if nonEmpty(v["driver_opts"]) || nonEmpty(v["external"]) {
			return bad("volume " + name + " has driver options or is external")
		}
	}
	networks, _ := model["networks"].(map[string]any)
	for name, raw := range networks {
		n, _ := raw.(map[string]any)
		if nonEmpty(n["external"]) {
			return bad("network " + name + " is external")
		}
	}
	return nil
}
