package output

import (
	"fmt"
	"maps"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	LangEn = "en"
	LangAr = "ar"
)

var localeVars = []string{"LC_ALL", "LC_MESSAGES", "LANG"}

type Catalog struct {
	Lang  string
	table map[Key]string
	codes map[string]string
}

func NewCatalog(lang string) Catalog {
	if lang == LangAr {
		return Catalog{Lang: LangAr, table: merged(arMessages, arPushMessages), codes: arCodes}
	}
	return Catalog{Lang: LangEn, table: merged(enMessages, enPushMessages)}
}

func merged(base, extra map[Key]string) map[Key]string {
	out := maps.Clone(base)
	maps.Copy(out, extra)
	return out
}

func ValidLang(lang string) bool {
	return lang == LangEn || lang == LangAr
}

func ChooseLang(flagLang string, getenv func(string) string) string {
	if ValidLang(flagLang) {
		return flagLang
	}
	if getenv == nil {
		return LangEn
	}
	for _, name := range localeVars {
		v := strings.ToLower(getenv(name))
		if v == "" {
			continue
		}
		utf := strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		if utf && strings.HasPrefix(v, "ar") {
			return LangAr
		}
		return LangEn
	}
	return LangEn
}

func (c Catalog) T(k Key, args ...any) string {
	s, ok := c.table[k]
	if !ok {
		s, ok = enMessages[k]
	}
	if !ok {
		s = string(k)
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

func (c Catalog) Issue(i pipelinespec.Issue) string {
	if m, ok := c.codes[i.Code]; ok {
		return m
	}
	return i.Message
}

func (c Catalog) Severity(s pipelinespec.Severity) string {
	if s == pipelinespec.SeverityWarning {
		return c.T(KeySeverityWarn)
	}
	return c.T(KeySeverityError)
}

func Printable(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\\':
			b.WriteString(`\\`)
		case unicode.IsPrint(r):
			b.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
		i += size
	}
	return b.String()
}
