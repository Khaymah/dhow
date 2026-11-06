package jobspec

import "strings"

func ValidComposeProject(name string) bool { return composeProjectPattern.MatchString(name) }

func ValidComposeFile(file string) bool {
	return composeFilePattern.MatchString(file) && !strings.HasPrefix(file, "/") && !strings.Contains(file, "..") && !strings.Contains(file, "//")
}
