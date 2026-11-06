package policy

import (
	"slices"
	"strings"
)

const invalidEntry = "INVALID_ENTRY"

var (
	boolKeys   = []string{"privileged", "use_api_socket", "init", "read_only"}
	modeFlags  = []string{"ro", "rw", "z", "Z", "shared", "slave", "private", "rshared", "rslave", "rprivate", "nocopy", "delegated", "cached", "consistent"}
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
