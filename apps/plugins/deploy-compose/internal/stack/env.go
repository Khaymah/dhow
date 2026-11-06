package stack

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*`)

func checkEnv(raw string) error {
	text := strings.TrimPrefix(raw, "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for strings.HasPrefix(line, "export") && len(line) > len("export") && unicode.IsSpace(rune(line[len("export")])) {
			line = strings.TrimSpace(line[len("export"):])
		}
		key := envKey.FindString(line)
		rest := strings.TrimSpace(line[len(key):])
		if key == "" || (rest != "" && rest[0] != '=' && rest[0] != ':') {
			return refusal(&policy.Violation{Code: CodeEnvForbidden, Detail: "a line of shared/.env cannot be parsed"})
		}
		for _, prefix := range []string{"COMPOSE_", "DOCKER_", "LD_"} {
			if strings.HasPrefix(strings.ToUpper(key), prefix) {
				return refusal(&policy.Violation{Code: CodeEnvForbidden, Detail: "shared/.env sets a " + prefix + " variable"})
			}
		}
	}
	return nil
}
