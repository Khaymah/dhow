package policy

import (
	"path"
	"strings"
)

func (c *checker) mounts(svc map[string]any) string {
	list, _ := svc["volumes"].([]any)
	for _, item := range list {
		if code := c.mount(item); code != "" {
			return code
		}
	}
	if code := c.envFiles(svc["env_file"]); code != "" {
		return code
	}
	if present(svc["volumes"]) && list == nil {
		return "BIND_OUTSIDE"
	}
	return ""
}

func (c *checker) mount(item any) string {
	switch v := item.(type) {
	case string:
		source, _, ok := strings.Cut(v, ":")
		if !ok || !isBindSource(source) {
			return ""
		}
		return c.bindCode(source)
	case map[string]any:
		return c.longMount(v)
	}
	return "BIND_OUTSIDE"
}

func (c *checker) longMount(m map[string]any) string {
	kind, _ := m["type"].(string)
	source, _ := m["source"].(string)
	switch strings.ToLower(kind) {
	case "volume", "tmpfs", "image", "cluster":
		return ""
	case "bind":
		return c.bindCode(source)
	case "":
		if isBindSource(source) {
			return c.bindCode(source)
		}
		return ""
	}
	return "BIND_OUTSIDE"
}

func (c *checker) envFiles(v any) string {
	switch f := v.(type) {
	case nil:
		return ""
	case string:
		return c.bindCode(f)
	case []any:
		for _, item := range f {
			if code := c.envFileItem(item); code != "" {
				return code
			}
		}
		return ""
	}
	return "BIND_OUTSIDE"
}

func (c *checker) envFileItem(item any) string {
	switch f := item.(type) {
	case string:
		return c.bindCode(f)
	case map[string]any:
		p, _ := f["path"].(string)
		return c.bindCode(p)
	}
	return "BIND_OUTSIDE"
}

func isBindSource(source string) bool {
	return source != "" && (strings.ContainsAny(source[:1], "/.~$") || strings.Contains(source, "/") || strings.Contains(source, "$"))
}

func (c *checker) bindCode(p string) string {
	switch {
	case p == "":
		return "BIND_OUTSIDE"
	case strings.Contains(p, "$"):
		return "INTERPOLATION"
	case strings.Contains(strings.ToLower(p), "docker.sock"):
		return "DOCKER_SOCKET"
	case strings.HasPrefix(p, "~"):
		return "BIND_OUTSIDE"
	case strings.HasPrefix(p, "/"):
		return c.absoluteCode(path.Clean(p))
	}
	return c.relativeCode(path.Clean(p))
}

func (c *checker) absoluteCode(p string) string {
	deploy := path.Clean(c.opt.DeployPath)
	if p == "/" {
		return "BIND_ROOT"
	}
	if deploy == "/" || deploy == "." || !strings.HasPrefix(p, deploy+"/") {
		return "BIND_OUTSIDE"
	}
	rest := strings.TrimPrefix(p, deploy+"/")
	if rest == ".dhow" || strings.HasPrefix(rest, ".dhow/") {
		return "BIND_OUTSIDE"
	}
	return ""
}

func (c *checker) relativeCode(p string) string {
	if p == ".." || strings.HasPrefix(p, "../") {
		return "BIND_OUTSIDE"
	}
	for _, link := range c.opt.Links {
		if p == link || strings.HasPrefix(p, link+"/") {
			return "BIND_SYMLINK"
		}
	}
	return ""
}
