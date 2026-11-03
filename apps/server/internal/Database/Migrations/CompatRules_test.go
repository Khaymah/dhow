package migrations

import (
	"regexp"
	"slices"
	"strings"
)

const (
	grandfatheredThrough = 48
	sqlIdent             = `(?:"(?:[^"]|"")+"|[A-Za-z_][A-Za-z0-9_$]*)`
	sqlName              = sqlIdent + `(?:\s*\.\s*` + sqlIdent + `)?`
)

func ci(pattern string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + pattern) }

var (
	rePhase        = ci(`^--\s*dhow:phase\s+(\S+)\s*$`)
	reFloor        = ci(`^--\s*dhow:floor\s+(\S+)\s*$`)
	reBackfill     = ci(`^--\s*dhow:backfill\s+small\s*$`)
	reSemver       = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	reNoTx         = ci(`^--\s*\+goose\s+NO\s+TRANSACTION\s*$`)
	reGooseUp      = ci(`^--\s*\+goose\s+Up\b`)
	reGooseDown    = ci(`^--\s*\+goose\s+Down\b`)
	reBegin        = ci(`^--\s*\+goose\s+StatementBegin\b`)
	reEnd          = ci(`^--\s*\+goose\s+StatementEnd\b`)
	reDollarTag    = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$`)
	reName         = ci(sqlName)
	reCreateTable  = ci(`^CREATE\s+(?:(?:GLOBAL|LOCAL)\s+)?(?:(?:TEMP|TEMPORARY|UNLOGGED)\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(` + sqlName + `)`)
	reCreateRoutin = ci(`^CREATE\s+(FUNCTION|TYPE)\s+(` + sqlName + `)`)
	reDropTable    = ci(`^DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?(.*)$`)
	reDropRoutine  = ci(`^DROP\s+(FUNCTION|TYPE)\s+(?:IF\s+EXISTS\s+)?(` + sqlName + `)`)
	reTruncate     = ci(`^TRUNCATE\s+(?:TABLE\s+)?(.*)$`)
	reDelete       = ci(`\bDELETE\s+FROM\s+(?:ONLY\s+)?(` + sqlName + `)`)
	reAlterTable   = ci(`^ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?(` + sqlName + `)\s*(.*)$`)
	reAlterOther   = ci(`^ALTER\s+`)
	reRename       = ci(`\bRENAME\b`)
	reActRename    = ci(`^RENAME\b`)
	reActDrop      = ci(`^DROP\s+`)
	reActDropCons  = ci(`^DROP\s+CONSTRAINT\b`)
	reActType      = ci(`^ALTER\s+(?:COLUMN\s+)?` + sqlIdent + `\s+(?:SET\s+DATA\s+)?TYPE\b`)
	reActNotNull   = ci(`^ALTER\s+(?:COLUMN\s+)?` + sqlIdent + `\s+SET\s+NOT\s+NULL\b`)
	reAddCons      = ci(`^ADD\s+(?:CONSTRAINT\b|CHECK\b|FOREIGN\s+KEY\b|UNIQUE\b|PRIMARY\s+KEY\b|EXCLUDE\b)`)
	reAddCheckFK   = ci(`^ADD\s+(?:CONSTRAINT\s+` + sqlIdent + `\s+)?(?:CHECK|FOREIGN\s+KEY)\b`)
	reAddColumn    = ci(`^ADD\s+(?:COLUMN\s+)?(?:IF\s+NOT\s+EXISTS\s+)?(` + sqlIdent + `)\s*(.*)$`)
	reNotNull      = ci(`\bNOT\s+NULL\b`)
	reHasValue     = ci(`\bDEFAULT\b|\bGENERATED\b`)
	reNotValid     = ci(`\bNOT\s+VALID\b`)
	reCreateIndex  = ci(`^CREATE\s+(?:UNIQUE\s+)?INDEX\s+(CONCURRENTLY\s+)?(IF\s+NOT\s+EXISTS\s+)?(?:` + sqlName + `\s+)?ON\s+(?:ONLY\s+)?(` + sqlName + `)`)
	reUpdate       = ci(`\bUPDATE\s+(?:ONLY\s+)?(` + sqlName + `)(?:\s+(?:AS\s+)?` + sqlIdent + `)?\s+SET\b`)
	reIdempotent   = ci(`\bIF\s+NOT\s+EXISTS\b|\bIF\s+EXISTS\b|\bVALIDATE\b`)
	reFloorUpdate  = ci(`\bUPDATE\s+"Instance"\s+SET\s+"SchemaFloor"\s*=\s*'([^']*)'`)
	listKeywords   = []string{"only", "cascade", "restrict", "restart", "continue", "identity"}
)

type compatContext struct {
	phase    string
	noTx     bool
	backfill bool
	tables   map[string]bool
	routines map[string]map[string]bool
}

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

func splitSections(text string) ([]string, string) {
	var header, up []string
	state := "header"
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case reGooseUp.MatchString(trimmed):
			state = "up"
		case reGooseDown.MatchString(trimmed):
			state = "down"
		case state == "header" && trimmed != "":
			header = append(header, trimmed)
		case state == "up":
			up = append(up, line)
		}
	}
	return header, strings.Join(up, "\n")
}

func splitStatements(up string) []string {
	var out, buf []string
	flush := func() {
		if stmt := strings.Join(strings.Fields(strings.Join(buf, " ")), " "); stmt != "" {
			out = append(out, stmt)
		}
		buf = buf[:0]
	}
	block := false
	masked := strings.Split(maskSQL(up), "\n")
	for k, raw := range strings.Split(up, "\n") {
		trimmed := strings.TrimSpace(raw)
		switch {
		case reBegin.MatchString(trimmed):
			flush()
			block = true
		case reEnd.MatchString(trimmed):
			flush()
			block = false
		case block:
			buf = append(buf, masked[k])
		default:
			parts := strings.Split(masked[k], ";")
			for p, part := range parts {
				buf = append(buf, part)
				if p < len(parts)-1 {
					flush()
				}
			}
		}
	}
	flush()
	return out
}

func lastPart(name string) string {
	cur := strings.Builder{}
	for i := 0; i < len(name); {
		switch name[i] {
		case '"':
			j := identifierEnd(name, i+1)
			cur.WriteString(name[i:j])
			i = j
		case '.':
			cur.Reset()
			i++
		default:
			cur.WriteByte(name[i])
			i++
		}
	}
	last := strings.TrimSpace(cur.String())
	if strings.HasPrefix(last, `"`) {
		return strings.ReplaceAll(last[1:len(last)-1], `""`, `"`)
	}
	return strings.ToLower(last)
}

func objectNames(text string) []string {
	var out []string
	for _, m := range reName.FindAllString(text, -1) {
		if !slices.Contains(listKeywords, strings.ToLower(m)) {
			out = append(out, lastPart(m))
		}
	}
	return out
}

func topLevel(text string) string {
	out, depth := strings.Builder{}, 0
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '(':
			depth++
		case text[i] == ')':
			depth = max(0, depth-1)
		case depth == 0:
			out.WriteByte(text[i])
		}
	}
	return out.String()
}

func splitActions(text string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth = max(0, depth-1)
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(text[start:]))
	return slices.DeleteFunc(parts, func(p string) bool { return p == "" })
}

func (c *compatContext) existingAlter(stmt string) []string {
	m := reAlterTable.FindStringSubmatch(stmt)
	if m == nil || c.tables[lastPart(m[1])] {
		return nil
	}
	return splitActions(m[2])
}

func (c *compatContext) recordNew(stmt string) {
	if m := reCreateTable.FindStringSubmatch(stmt); m != nil {
		c.tables[lastPart(m[1])] = true
	}
	if m := reCreateRoutin.FindStringSubmatch(stmt); m != nil {
		c.routines[strings.ToLower(m[1])][lastPart(m[2])] = true
	}
}

func alterAction(action string) bool {
	if reActRename.MatchString(action) || reActType.MatchString(action) || reActNotNull.MatchString(action) {
		return true
	}
	return reActDrop.MatchString(action) && !reActDropCons.MatchString(action)
}

func (c *compatContext) anyOld(text string) bool {
	return slices.ContainsFunc(objectNames(text), func(n string) bool { return !c.tables[n] })
}

func (c *compatContext) m2(stmt string) bool {
	if c.phase != "expand" {
		return false
	}
	for _, re := range []*regexp.Regexp{reDropTable, reTruncate} {
		if m := re.FindStringSubmatch(stmt); m != nil && c.anyOld(m[1]) {
			return true
		}
	}
	if m := reDropRoutine.FindStringSubmatch(stmt); m != nil && !c.routines[strings.ToLower(m[1])][lastPart(m[2])] {
		return true
	}
	if m := reDelete.FindStringSubmatch(stmt); m != nil && !c.tables[lastPart(m[1])] {
		return true
	}
	if reAlterTable.MatchString(stmt) {
		return slices.ContainsFunc(c.existingAlter(stmt), alterAction)
	}
	return reAlterOther.MatchString(stmt) && reRename.MatchString(stmt)
}

func (c *compatContext) m3(stmt string) bool {
	if c.phase != "expand" {
		return false
	}
	for _, action := range c.existingAlter(stmt) {
		if reAddCons.MatchString(action) {
			continue
		}
		m := reAddColumn.FindStringSubmatch(action)
		if m == nil {
			continue
		}
		rest := topLevel(m[2])
		if reNotNull.MatchString(rest) && !reHasValue.MatchString(rest) {
			return true
		}
	}
	return false
}

func (c *compatContext) m4(stmt string) bool {
	return slices.ContainsFunc(c.existingAlter(stmt), func(action string) bool {
		return reAddCheckFK.MatchString(action) && !reNotValid.MatchString(action)
	})
}

func (c *compatContext) m5(stmt string) bool {
	m := reCreateIndex.FindStringSubmatch(stmt)
	if m == nil {
		return false
	}
	concurrently, ifNotExists := m[1] != "", m[2] != ""
	if concurrently && !c.noTx {
		return true
	}
	return !c.tables[lastPart(m[3])] && (!concurrently || !ifNotExists)
}

func (c *compatContext) m6(stmt string) bool {
	if c.phase != "expand" || c.backfill {
		return false
	}
	m := reUpdate.FindStringSubmatch(stmt)
	return m != nil && !c.tables[lastPart(m[1])]
}

func (c *compatContext) m9(stmt string) bool {
	return c.noTx && !reIdempotent.MatchString(stmt)
}

func phaseOf(directives []string) string {
	if len(directives) == 0 {
		return ""
	}
	first := directives[0]
	if reNoTx.MatchString(first) && len(directives) > 1 {
		first = directives[1]
	}
	if m := rePhase.FindStringSubmatch(first); m != nil && (m[1] == "expand" || m[1] == "contract") {
		return m[1]
	}
	return ""
}

func floorOf(directives []string) (string, bool) {
	for _, d := range directives {
		if m := reFloor.FindStringSubmatch(d); m != nil {
			return m[1], true
		}
	}
	return "", false
}

func floorUpdates(up string) []string {
	var out []string
	for _, loc := range reFloorUpdate.FindAllStringSubmatchIndex(maskSQL(up), -1) {
		out = append(out, up[loc[2]:loc[3]])
	}
	return out
}

func headerVerdict(directives []string, phase string, tags map[string]bool) string {
	if phase == "" {
		return "M1"
	}
	if phase != "contract" {
		return ""
	}
	floor, ok := floorOf(directives)
	if !ok || !reSemver.MatchString(floor) {
		return "M7"
	}
	if floor != "0.0.0" && !tags["v"+floor] {
		return "M8"
	}
	return ""
}

func compatVerdict(text string, tags map[string]bool) string {
	directives, up := splitSections(text)
	phase := phaseOf(directives)
	if rule := headerVerdict(directives, phase, tags); rule != "" {
		return rule
	}
	c := &compatContext{
		phase:    phase,
		noTx:     slices.ContainsFunc(strings.Split(text, "\n"), func(l string) bool { return reNoTx.MatchString(strings.TrimSpace(l)) }),
		backfill: slices.ContainsFunc(directives, reBackfill.MatchString),
		tables:   map[string]bool{},
		routines: map[string]map[string]bool{"function": {}, "type": {}},
	}
	rules := []struct {
		id    string
		check func(string) bool
	}{{"M2", c.m2}, {"M3", c.m3}, {"M4", c.m4}, {"M5", c.m5}, {"M6", c.m6}, {"M9", c.m9}}
	for _, stmt := range splitStatements(up) {
		c.recordNew(stmt)
		for _, r := range rules {
			if r.check(stmt) {
				return r.id
			}
		}
	}
	if phase == "contract" {
		floor, _ := floorOf(directives)
		values := floorUpdates(up)
		if slices.ContainsFunc(values, func(v string) bool { return v != floor }) || (floor != "0.0.0" && len(values) == 0) {
			return "M7"
		}
	}
	return ""
}
