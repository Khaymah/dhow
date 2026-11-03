package pathglob

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	globStar  = "**"
	MaxLength = 256
)

func ValidPattern(p string) bool {
	if p == "" || len(p) > MaxLength || !utf8.ValidString(p) || strings.ContainsAny(p, "[]{}\\") {
		return false
	}
	if strings.IndexFunc(p, unicode.IsControl) >= 0 || strings.Contains(p, "${{") {
		return false
	}
	for seg := range strings.SplitSeq(p, "/") {
		if strings.Contains(seg, globStar) && seg != globStar {
			return false
		}
	}
	return true
}

func MatchPattern(pattern, name string) bool {
	if !ValidPattern(pattern) {
		return false
	}
	ps := strings.Split(pattern, "/")
	ns := strings.Split(name, "/")
	reach := make([]bool, len(ns)+1)
	reach[0] = true
	for _, p := range ps {
		next := make([]bool, len(ns)+1)
		for j := range reach {
			if !reach[j] {
				continue
			}
			if p == globStar {
				for k := j; k <= len(ns); k++ {
					next[k] = true
				}
				continue
			}
			if j < len(ns) && matchSegment(p, ns[j]) {
				next[j+1] = true
			}
		}
		reach = next
	}
	return reach[len(ns)]
}

func matchSegment(p, s string) bool {
	sr := []rune(s)
	row := make([]bool, len(sr)+1)
	row[0] = true
	for _, c := range p {
		next := make([]bool, len(sr)+1)
		if c == '*' {
			next[0] = row[0]
		}
		for j := 1; j <= len(sr); j++ {
			switch c {
			case '*':
				next[j] = row[j] || next[j-1]
			case '?':
				next[j] = row[j-1]
			default:
				next[j] = row[j-1] && sr[j-1] == c
			}
		}
		row = next
	}
	return row[len(sr)]
}
