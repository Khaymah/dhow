package deployrelease

import (
	"errors"
	"regexp"
	"strings"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	maxCommandBytes = 16 << 10
	maxArgBytes     = 4 << 10
)

var (
	ErrCommand = errors.New("the remote command has an argument that cannot be sent safely")

	bareArg = regexp.MustCompile(`^[A-Za-z0-9._/=:@%+,-]+$`)
)

func Quote(arg string) string {
	if bareArg.MatchString(arg) && !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "~") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func Command(argv ...string) (string, error) {
	if len(argv) == 0 || argv[0] == "" || !bareArg.MatchString(argv[0]) || strings.HasPrefix(argv[0], "-") {
		return "", ErrCommand
	}
	parts := make([]string, len(argv))
	for i, a := range argv {
		if len(a) > maxArgBytes || strings.ContainsRune(a, 0) {
			return "", ErrCommand
		}
		parts[i] = Quote(a)
	}
	line := strings.Join(parts, " ")
	if len(line) > maxCommandBytes {
		return "", ErrCommand
	}
	return line, nil
}

func ValidPath(p string) bool { return jobspec.ValidDeployPath(p) }

var segment = regexp.MustCompile(`^[A-Za-z0-9._-]{1,200}$`)

func joinPath(base string, parts ...string) (string, error) {
	out := base
	for _, p := range parts {
		if !segment.MatchString(p) || p == "." || p == ".." {
			return "", ErrCommand
		}
		out += "/" + p
	}
	return out, nil
}
