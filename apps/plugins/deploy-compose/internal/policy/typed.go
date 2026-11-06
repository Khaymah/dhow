package policy

import (
	"slices"
	"strings"
)

const invalidEntry = "INVALID_ENTRY"

var (
	boolKeys   = []string{"privileged", "use_api_socket", "init", "read_only"}
	modeFlags  = []string{"ro", "rw", "private", "rprivate", "nocopy", "delegated", "cached", "consistent"}
	mountTexts = []string{"type", "source", "target"}
)

func typedBool(m map[string]any, key, where string) error {
	if v, ok := m[key]; ok && v != nil {
		if _, isBool := v.(bool); !isBool {
			return invalid("%s.%s must be true or false", where, key)
		}
	}
	return nil
}

func (c *checker) typed(name string, svc map[string]any) error {
	for _, key := range boolKeys {
		if err := typedBool(svc, key, name); err != nil {
			return err
		}
	}
	for _, key := range []string{"post_start", "pre_stop"} {
		list, _ := svc[key].([]any)
		for _, item := range list {
			hook, _ := asMap(item)
			if err := typedBool(hook, "privileged", name+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *checker) bindOptions(m map[string]any) string {
	raw, ok := m["bind"]
	if !ok || raw == nil {
		return ""
	}
	bind, isMap := asMap(raw)
	if !isMap {
		return invalidEntry
	}
	if v, has := bind["create_host_path"]; has && v != nil {
		if _, isBool := v.(bool); !isBool {
			return invalidEntry
		}
	}
	if c.opt.AllowUnsafe {
		return ""
	}
	for _, key := range []string{"propagation", "selinux", "recursive"} {
		if _, has := bind[key]; has {
			return "BIND_OPTIONS"
		}
	}
	if truthy(bind["create_host_path"]) {
		return "BIND_OPTIONS"
	}
	return ""
}

func parseMode(text string) (readOnly, ok bool) {
	var ro, rw bool
	for _, flag := range strings.Split(text, ",") {
		if !slices.Contains(modeFlags, flag) {
			return false, false
		}
		ro = ro || flag == "ro"
		rw = rw || flag == "rw"
	}
	return ro && !rw, true
}

func validMountTexts(m map[string]any) bool {
	for _, key := range mountTexts {
		if v, ok := m[key]; ok {
			if _, isText := v.(string); !isText {
				return false
			}
		}
	}
	if v, ok := m["read_only"]; ok && v != nil {
		if _, isBool := v.(bool); !isBool {
			return false
		}
	}
	return true
}
