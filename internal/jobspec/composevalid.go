package jobspec

import (
	"regexp"
	"strings"
)

var strictProjectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func ValidComposeProject(name string) bool { return strictProjectPattern.MatchString(name) }

func ValidComposeFile(file string) bool {
	return composeFilePattern.MatchString(file) && !strings.HasPrefix(file, "/") && !strings.Contains(file, "..") && !strings.Contains(file, "//")
}
