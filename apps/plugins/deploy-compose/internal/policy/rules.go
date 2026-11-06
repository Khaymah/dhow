package policy

import (
	"strings"
)

type rule struct {
	name string
	hit  func(svc map[string]any) bool
}

var rules = []rule{
	{"PRIVILEGED", func(s map[string]any) bool { return truthy(s["privileged"]) }},
	{"PID_HOST", hostKey("pid")},
	{"IPC_HOST", hostKey("ipc")},
	{"UTS_HOST", hostKey("uts")},
	{"CGROUP_HOST", hostKey("cgroup")},
	{"USERNS_HOST", hostKey("userns_mode")},
	{"NETWORK_HOST", hostKey("network_mode")},
	{"DOCKER_SOCKET", func(s map[string]any) bool { return truthy(s["use_api_socket"]) }},
	{"CAP_ADD", func(s map[string]any) bool { return present(s["cap_add"]) }},
	{"DEVICES", deviceAccess},
	{"SECURITY_OPT", securityOpt},
	{"PRIVILEGED", lifecycleHooks},
	{"CGROUP_PARENT", keyPresent("cgroup_parent")},
	{"RUNTIME", keyPresent("runtime")},
	{"SYSCTLS", keyPresent("sysctls")},
	{"GROUP_ADD", keyPresent("group_add")},
	{"EXTRA_HOSTS", keyPresent("extra_hosts")},
	{"EXTERNAL_LINKS", keyPresent("external_links")},
	{"MODELS", keyPresent("models")},
	{"LOGGING", loggingUnsafe},
}

func present(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case string:
		return x != ""
	}
	return true
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "", "false", "no", "n", "off", "0":
			return false
		}
		return true
	case int:
		return x != 0
	}
	return true
}

func hostKey(key string) func(map[string]any) bool {
	return func(s map[string]any) bool {
		v, ok := s[key]
		if !ok || v == nil {
			return false
		}
		text, isText := v.(string)
		if !isText {
			return true
		}
		text = strings.ToLower(strings.TrimSpace(text))
		return text == "host" || strings.Contains(text, "$")
	}
}

func deviceAccess(s map[string]any) bool {
	if present(s["devices"]) || present(s["device_cgroup_rules"]) || present(s["gpus"]) {
		return true
	}
	deploy, _ := asMap(s["deploy"])
	resources, _ := asMap(deploy["resources"])
	for _, side := range []string{"reservations", "limits"} {
		part, _ := asMap(resources[side])
		if present(part["devices"]) {
			return true
		}
	}
	return false
}

func securityOpt(s map[string]any) bool {
	v, ok := s["security_opt"]
	if !ok || v == nil {
		return false
	}
	list, isList := v.([]any)
	if !isList {
		return true
	}
	for _, item := range list {
		text, isText := item.(string)
		if !isText {
			return true
		}
		norm := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(text), ":", "="))
		if norm != "no-new-privileges" && norm != "no-new-privileges=true" {
			return true
		}
	}
	return false
}

func lifecycleHooks(s map[string]any) bool {
	for _, key := range []string{"post_start", "pre_stop"} {
		list, isList := s[key].([]any)
		if !isList {
			if present(s[key]) {
				return true
			}
			continue
		}
		for _, item := range list {
			hook, ok := asMap(item)
			if !ok || truthy(hook["privileged"]) {
				return true
			}
		}
	}
	return false
}
