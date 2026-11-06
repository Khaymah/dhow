package policy

import (
	"path"
	"slices"
	"strings"
)

func (c *checker) volumesAlways(name string, svc map[string]any) error {
	list, isList := svc["volumes"].([]any)
	if present(svc["volumes"]) && !isList {
		return always("BIND_OUTSIDE", name)
	}
	for _, item := range list {
		code := c.mount(item)
		if code == invalidEntry {
			return invalid("a volume entry of service %s is not valid", name)
		}
		if code != "" {
			return always(code, name)
		}
	}
	if code := c.envFiles(svc["env_file"]); code != "" {
		return always(code, name)
	}
	return nil
}

func (c *checker) mount(item any) string {
	switch v := item.(type) {
	case string:
		if strings.Contains(v, "$") {
			return "INTERPOLATION"
		}
		parts := strings.Split(v, ":")
		if len(parts) > 3 {
			return invalidEntry
		}
		readOnly := false
		if len(parts) == 3 {
			var ok bool
			if readOnly, ok = parseMode(parts[2]); !ok {
				return invalidEntry
			}
		}
		if len(parts) < 2 || !isBindSource(parts[0]) {
			return ""
		}
		return c.mountCode(parts[0], !readOnly)
	case map[string]any:
		return c.longMount(v)
	}
	return "BIND_OUTSIDE"
}

func (c *checker) longMount(m map[string]any) string {
	if !validMountTexts(m) {
		return invalidEntry
	}
	if code := c.bindOptions(m); code != "" {
		return code
	}
	kind, _ := m["type"].(string)
	source, _ := m["source"].(string)
	target, _ := m["target"].(string)
	if strings.Contains(source, "$") || strings.Contains(target, "$") {
		return "INTERPOLATION"
	}
	switch strings.ToLower(kind) {
	case "volume", "tmpfs", "image", "cluster":
		return ""
	case "bind":
		return c.mountCode(source, !truthy(m["read_only"]))
	case "":
		if isBindSource(source) {
			return c.mountCode(source, !truthy(m["read_only"]))
		}
		return ""
	}
	return "BIND_OUTSIDE"
}

func isBindSource(source string) bool {
	return source != "" && (strings.ContainsAny(source[:1], "/.~$") || strings.Contains(source, "/") || strings.Contains(source, "$"))
}

func (c *checker) envFiles(v any) string {
	switch f := v.(type) {
	case nil:
		return ""
	case string:
		return c.fileCode(f)
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
		return c.fileCode(f)
	case map[string]any:
		p, _ := f["path"].(string)
		return c.fileCode(p)
	}
	return "BIND_OUTSIDE"
}

func (c *checker) mountCode(p string, writable bool) string {
	if p == "" {
		return invalidEntry
	}
	code := c.structCode(p, true)
	if hardCodes[code] {
		return code
	}
	if c.opt.AllowUnsafe {
		c.noteBind(p)
		return ""
	}
	switch {
	case code != "":
		return code
	case strings.HasPrefix(p, "/") && c.isShared(path.Clean(p)):
		return "BIND_SHARED_DATA"
	case strings.HasPrefix(p, "/"):
		return "BIND_OUTSIDE"
	case writable:
		return "BIND_WRITABLE"
	}
	c.noteBind(p)
	return ""
}

func (c *checker) fileCode(p string) string {
	if p == "" {
		return invalidEntry
	}
	code := c.structCode(p, false)
	if hardCodes[code] {
		return code
	}
	if c.opt.AllowUnsafe {
		c.note(&c.files, p)
		return ""
	}
	if code != "" {
		return code
	}
	if strings.HasPrefix(p, "/") && path.Clean(p) != path.Clean(c.opt.DeployPath)+"/"+sharedDir+"/.env" {
		return "BIND_OUTSIDE"
	}
	c.note(&c.files, p)
	return ""
}

func (c *checker) isShared(p string) bool {
	return strings.HasPrefix(p, path.Clean(c.opt.DeployPath)+"/"+sharedDir+"/")
}

func (c *checker) noteBind(p string) {
	if !strings.HasPrefix(p, "/") {
		c.note(&c.binds, p)
	}
}

func (c *checker) note(into *[]string, p string) {
	p = path.Clean(p)
	if !slices.Contains(*into, p) {
		*into = append(*into, p)
	}
}
