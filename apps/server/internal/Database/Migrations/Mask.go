package migrations

import (
	"regexp"
	"strings"
)

var (
	reDollarTag  = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$`)
	reTxControl  = regexp.MustCompile(`(?i)^(?:BEGIN|START\s+TRANSACTION|COMMIT|END|ROLLBACK|ABORT|SAVEPOINT|RELEASE|PREPARE\s+TRANSACTION)\b`)
	reAtomicBody = regexp.MustCompile(`(?is)^CREATE\s+(?:OR\s+REPLACE\s+)?(?:FUNCTION|PROCEDURE)\b.*\bBEGIN\s+ATOMIC\b`)
)

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func blankRange(out []byte, start, end int, fill byte) {
	for k := start; k < end; k++ {
		if out[k] != '\n' {
			out[k] = fill
		}
	}
}

func stringEnd(text string, i int, escaped bool) int {
	for i < len(text) {
		switch {
		case escaped && text[i] == '\\':
			i += 2
		case strings.HasPrefix(text[i:], "''"):
			i += 2
		case text[i] == '\'':
			return i + 1
		default:
			i++
		}
	}
	return len(text)
}

func identifierEnd(text string, i int) int {
	for i < len(text) {
		switch {
		case strings.HasPrefix(text[i:], `""`):
			i += 2
		case text[i] == '"':
			return i + 1
		default:
			i++
		}
	}
	return len(text)
}

func endOrLen(idx, from, extra, n int) int {
	if idx < 0 {
		return n
	}
	return from + idx + extra
}

func maskSQL(text string) string {
	out, n := []byte(text), len(text)
	for i := 0; i < n; {
		c := text[i]
		switch {
		case strings.HasPrefix(text[i:], "--"):
			j := endOrLen(strings.IndexByte(text[i:], '\n'), i, 0, n)
			blankRange(out, i, j, ' ')
			i = j
		case strings.HasPrefix(text[i:], "/*"):
			j := endOrLen(strings.Index(text[i+2:], "*/"), i+2, 2, n)
			blankRange(out, i, j, ' ')
			i = j
		case c == '\'':
			escaped := i > 0 && (text[i-1] == 'e' || text[i-1] == 'E') && (i < 2 || !isIdentByte(text[i-2]))
			j := stringEnd(text, i+1, escaped)
			blankRange(out, i+1, max(i+1, j-1), 'x')
			i = j
		case c == '"':
			i = identifierEnd(text, i+1)
		case c == '$' && (i == 0 || !isIdentByte(text[i-1])) && reDollarTag.MatchString(text[i:]):
			tag := reDollarTag.FindString(text[i:])
			j := endOrLen(strings.Index(text[i+len(tag):], tag), i+len(tag), len(tag), n)
			blankRange(out, i, j, ' ')
			i = j
		default:
			i++
		}
	}
	return string(out)
}

func TransactionControl(stmt string) bool {
	masked := strings.TrimSpace(maskSQL(stmt))
	if reAtomicBody.MatchString(masked) {
		return false
	}
	for _, piece := range strings.Split(masked, ";") {
		if reTxControl.MatchString(strings.TrimSpace(piece)) {
			return true
		}
	}
	return false
}
