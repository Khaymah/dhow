package policy

import (
	"slices"
	"strings"
)

var (
	topKeys = []string{"services", "volumes", "networks", "secrets", "configs", "version", "name"}

	serviceKeys = []string{
		"image", "command", "entrypoint", "environment", "env_file", "ports", "expose", "volumes", "networks", "depends_on",
		"healthcheck", "restart", "labels", "logging", "deploy", "user", "working_dir", "read_only", "tmpfs", "stop_signal",
		"stop_grace_period", "init", "hostname", "domainname", "mem_limit", "cpus", "ulimits", "shm_size", "secrets", "configs",
		"profiles", "cap_drop", "pids_limit", "mem_reservation", "memswap_limit", "cpu_shares",
	}

	unsafeServiceKeys = []string{
		"privileged", "pid", "ipc", "uts", "cgroup", "userns_mode", "network_mode", "cap_add", "devices", "device_cgroup_rules",
		"gpus", "security_opt", "use_api_socket", "post_start", "pre_stop", "cgroup_parent", "runtime", "sysctls", "group_add",
		"extra_hosts", "external_links", "models",
	}

	deployKeys = []string{"resources", "restart_policy"}
)

func always(code, service string) error {
	return &Violation{Code: unsafePrefix + code, Service: service}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func (c *checker) always(doc map[string]any, names []string, services map[string]any) error {
	if err := c.topAlways(doc); err != nil {
		return err
	}
	for _, name := range names {
		svc, _ := asMap(services[name])
		if err := c.serviceAlways(name, svc); err != nil {
			return err
		}
	}
	return nil
}

func (c *checker) topAlways(doc map[string]any) error {
	for _, key := range sortedKeys(doc) {
		switch {
		case key == "include":
			return always("INCLUDE", "")
		case key == "models" && !c.opt.AllowUnsafe:
			return always("MODELS", "")
		case key == "models", strings.HasPrefix(key, "x-"), slices.Contains(topKeys, key):
		default:
			return always("UNKNOWN_KEY", key)
		}
	}
	for _, key := range []string{"networks", "volumes"} {
		items, _ := asMap(doc[key])
		for _, name := range sortedKeys(items) {
			item, _ := asMap(items[name])
			if err := topItem(key, name, item); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"secrets", "configs"} {
		items, _ := asMap(doc[key])
		for _, name := range sortedKeys(items) {
			item, _ := asMap(items[name])
			for _, k := range sortedKeys(item) {
				if k != "file" && !strings.HasPrefix(k, "x-") {
					return always("SECRET_SOURCE", key+"."+name)
				}
			}
			if file, ok := item["file"].(string); ok {
				if code := c.fileCode(file); code != "" {
					return always(code, key+"."+name)
				}
			}
		}
	}
	return nil
}

func topItem(kind, name string, item map[string]any) error {
	where := kind + "." + name
	if _, ok := item["name"]; ok || truthy(item["external"]) {
		return always("EXTERNAL", where)
	}
	if _, ok := item["external"]; ok {
		return always("EXTERNAL", where)
	}
	if driver, ok := item["driver"]; ok && kind == "volumes" {
		if text, isText := driver.(string); !isText || text != "local" {
			return always("VOLUME_DRIVER", where)
		}
	}
	if _, ok := item["driver_opts"]; ok && kind == "volumes" {
		return always("VOLUME_OPTS", where)
	}
	return nil
}

func (c *checker) serviceAlways(name string, svc map[string]any) error {
	for _, key := range sortedKeys(svc) {
		switch {
		case key == "extends":
			return always("EXTENDS", name)
		case key == "provider":
			return always("PROVIDER", name)
		case key == "volumes_from":
			return always("VOLUMES_FROM", name)
		case key == "build", strings.HasPrefix(key, "x-"), slices.Contains(serviceKeys, key), slices.Contains(unsafeServiceKeys, key):
		default:
			return always("UNKNOWN_KEY", name+"."+key)
		}
	}
	for _, key := range []string{"pid", "network_mode", "ipc", "uts"} {
		text, _ := svc[key].(string)
		if strings.Contains(text, "$") {
			return always("INTERPOLATION", name)
		}
		text = strings.ToLower(strings.TrimSpace(text))
		if strings.HasPrefix(text, "container:") || strings.HasPrefix(text, "service:") || (key == "ipc" && text == "shareable") {
			return always("NAMESPACE_SHARED", name)
		}
	}
	deploy, _ := asMap(svc["deploy"])
	for _, key := range sortedKeys(deploy) {
		if !slices.Contains(deployKeys, key) && !strings.HasPrefix(key, "x-") {
			return always("UNKNOWN_KEY", name+".deploy."+key)
		}
	}
	return c.volumesAlways(name, svc)
}

func keyPresent(key string) func(map[string]any) bool {
	return func(s map[string]any) bool {
		_, ok := s[key]
		return ok
	}
}
