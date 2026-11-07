package response

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
)

const etagBytes = 16

func ETagOf(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return `"` + base64.RawURLEncoding.EncodeToString(sum[:etagBytes]) + `"`, nil
}

func ETagMatches(header *string, etag string) bool {
	if header == nil || etag == "" {
		return false
	}
	for _, part := range strings.Split(*header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}
