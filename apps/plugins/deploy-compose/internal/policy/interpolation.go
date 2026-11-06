package policy

import (
	"slices"
	"strings"
)

var valueKeys = []string{"environment", "command", "entrypoint", "labels"}

func hasInterpolation(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '$' {
			i++
			continue
		}
		return true
	}
	return false
}

func valuePosition(path []string) bool {
	if len(path) < 3 || path[0] != "services" {
		return false
	}
	if slices.Contains(valueKeys, path[2]) {
		return true
	}
	return path[2] == "healthcheck" && len(path) >= 4 && path[3] == "test"
}

func (c *checker) interpolation(doc map[string]any) error {
	return walkInterpolation([]string{}, doc)
}

func walkInterpolation(path []string, v any) error {
	if valuePosition(path) {
		return nil
	}
	switch x := v.(type) {
	case string:
		if hasInterpolation(x) {
			return always("INTERPOLATION", strings.Join(path, "."))
		}
	case []any:
		for i := range x {
			if err := walkInterpolation(append(path, "[]"), x[i]); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, k := range sortedKeys(x) {
			if hasInterpolation(k) {
				return always("INTERPOLATION", strings.Join(path, "."))
			}
			if err := walkInterpolation(append(path, k), x[k]); err != nil {
				return err
			}
		}
	case map[any]any:
		return invalid("a mapping has a key that is not text at %s", strings.Join(path, "."))
	}
	return nil
}
