package policy

import (
	"slices"
	"strings"
)

var loggingOptions = []string{"max-size", "max-file", "compress"}

func (c *checker) top(doc map[string]any) error {
	items, _ := asMap(doc["networks"])
	for _, name := range sortedKeys(items) {
		item, _ := asMap(items[name])
		driver, hasDriver := item["driver"]
		if text, isText := driver.(string); hasDriver && (!isText || (text != "" && text != "bridge")) {
			return &Violation{Code: unsafePrefix + "NETWORK_DRIVER", Service: "networks." + name}
		}
		if present(item["driver_opts"]) || present(item["ipam"]) {
			return &Violation{Code: unsafePrefix + "NETWORK_OPTS", Service: "networks." + name}
		}
	}
	return nil
}

func (c *checker) unsafe(name string, svc map[string]any) error {
	for _, r := range rules {
		if r.hit(svc) {
			return &Violation{Code: unsafePrefix + r.name, Service: name}
		}
	}
	return nil
}

func loggingUnsafe(s map[string]any) bool {
	v, ok := s["logging"]
	if !ok || v == nil {
		return false
	}
	logging, isMap := asMap(v)
	if !isMap {
		return true
	}
	for key, val := range logging {
		switch key {
		case "driver":
			if text, isText := val.(string); !isText || (text != "json-file" && text != "local") {
				return true
			}
		case "options":
			opts, optsMap := asMap(val)
			if !optsMap && val != nil {
				return true
			}
			for k := range opts {
				if !slices.Contains(loggingOptions, k) {
					return true
				}
			}
		default:
			if !strings.HasPrefix(key, "x-") {
				return true
			}
		}
	}
	return false
}
