package executor

import (
	"maps"
	"slices"

	masking "github.com/Khaymah/dhow/apps/runner/internal/Masking"
)

func maskOutputs(m *masking.Masker, raw map[string]string) map[string]string {
	keys := slices.Sorted(maps.Keys(raw))
	out := make(map[string]string, len(raw))
	for _, k := range keys {
		out[k] = m.MaskString(raw[k])
	}
	hidden := func(s string) bool { return m.MaskString(s) != s }
	drop := map[string]bool{}
	for i := range keys {
		joined := out[keys[i]]
		for j := i + 1; j < len(keys); j++ {
			joined += out[keys[j]]
			if hidden(joined) {
				for _, k := range keys[i : j+1] {
					drop[k] = true
				}
				break
			}
		}
	}
	for _, a := range keys {
		for _, b := range keys {
			if a != b && hidden(out[a]+out[b]) {
				drop[a], drop[b] = true, true
			}
		}
	}
	for k := range drop {
		delete(out, k)
	}
	return out
}
