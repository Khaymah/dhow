package apperrors_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
)

const (
	docsRoot       = "../../../../.."
	maxFixRunes    = 190
	codeLinePrefix = `(?m)^\s+Code[A-Za-z0-9]+\s*=\s*"([A-Z_0-9]+)"`
)

var fixAllowlist = map[string]string{
	"METHOD_NOT_ALLOWED": "Returned to an API client that used a wrong HTTP verb. The web app never sends it, so a sentence for a person would only mislead.",
	"WEBHOOK_REJECTED":   "Returned to the Git host that sent a webhook, not to a person. The fix is in the Git host webhook settings, which docs/guides/git.md covers.",
	"LENGTH_REQUIRED":    "Returned to the CLI and the CI action that upload a file. The message already names the Content-Length header, and no person reads this response.",
}

var codePattern = regexp.MustCompile(codeLinePrefix)

func allCodes(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(f)
		require.NoError(t, readErr)
		for _, m := range codePattern.FindAllStringSubmatch(string(body), -1) {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func TestEveryCodeHasAFixOrAnAllowlistReason(t *testing.T) {
	codes := allCodes(t)
	require.GreaterOrEqual(t, len(codes), 123)
	for _, code := range codes {
		reason, allowed := fixAllowlist[code]
		for _, locale := range []i18n.Locale{i18n.Ar, i18n.En} {
			fix := i18n.Fix(locale, code)
			if allowed {
				assert.NotEmpty(t, strings.TrimSpace(reason), code)
				assert.Empty(t, fix, "%s is allowlisted but has a %s fix; remove it from the allowlist", code, locale)
				continue
			}
			assert.NotEmpty(t, fix, "%s has no %s fix", code, locale)
		}
	}
}

func TestNoStaleCatalogEntries(t *testing.T) {
	known := map[string]bool{}
	for _, code := range allCodes(t) {
		known[code] = true
	}
	for code := range fixAllowlist {
		assert.True(t, known[code], "allowlist names an unknown code %s", code)
	}
	for _, locale := range []i18n.Locale{i18n.Ar, i18n.En} {
		for _, code := range i18n.FixCodes(locale) {
			assert.True(t, known[code], "%s fix names an unknown code %s", locale, code)
		}
	}
	for _, code := range i18n.DocsCodes() {
		assert.True(t, known[code], "docs path names an unknown code %s", code)
	}
	assert.ElementsMatch(t, i18n.FixCodes(i18n.Ar), i18n.FixCodes(i18n.En))
}

func hasArabicLetter(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}

func TestFixSentencesAreStaticAndShort(t *testing.T) {
	for _, locale := range []i18n.Locale{i18n.Ar, i18n.En} {
		for _, code := range i18n.FixCodes(locale) {
			fix := i18n.Fix(locale, code)
			assert.NotContains(t, fix, "%", "%s %s must not hold a placeholder", locale, code)
			assert.NotContains(t, fix, "{", "%s %s must not hold a placeholder", locale, code)
			assert.Equal(t, strings.TrimSpace(fix), fix, "%s %s has stray whitespace", locale, code)
			assert.LessOrEqual(t, len([]rune(fix)), maxFixRunes, "%s %s is too long", locale, code)
			assert.True(t, strings.HasSuffix(fix, "."), "%s %s must end with a full stop", locale, code)
			assert.Equal(t, locale == i18n.Ar, hasArabicLetter(fix), "%s %s has the wrong script", locale, code)
		}
	}
}

func slugOf(heading string) string {
	text := strings.ToLower(strings.ReplaceAll(heading, "`", ""))
	var out strings.Builder
	for _, r := range text {
		switch {
		case r == ' ':
			out.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r):
			out.WriteRune(r)
		}
	}
	return out.String()
}

func anchorsOf(t *testing.T, path string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err, path)
	anchors := map[string]bool{}
	fenced := false
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}
		anchors[slugOf(strings.TrimSpace(strings.TrimLeft(line, "#")))] = true
	}
	return anchors
}

func TestDocsPathsPointToRealPagesAndAnchors(t *testing.T) {
	roots := map[i18n.Locale]string{
		i18n.Ar: filepath.Join(docsRoot, "docs"),
		i18n.En: filepath.Join(docsRoot, "docs", "en"),
	}
	checked := 0
	for _, code := range i18n.DocsCodes() {
		for locale, root := range roots {
			path := i18n.DocsPath(locale, code)
			require.NotEmpty(t, path, "%s %s", locale, code)
			page, anchor := i18n.SplitDocsPath(path)
			assert.False(t, strings.HasPrefix(page, "/") || strings.Contains(page, ".."), "%s %s must be a relative path", locale, code)
			assert.True(t, strings.HasSuffix(page, ".md"), "%s %s must point to a markdown page", locale, code)
			file := filepath.Join(root, filepath.FromSlash(page))
			if _, err := os.Stat(file); err != nil {
				t.Errorf("%s %s: %s does not exist", locale, code, file)
				continue
			}
			if anchor != "" {
				assert.True(t, anchorsOf(t, file)[anchor], "%s %s: no heading gives the anchor #%s in %s", locale, code, anchor, file)
			}
			checked++
		}
	}
	assert.Positive(t, checked)
}

func TestDocsPathIsEmptyForACodeWithoutOne(t *testing.T) {
	assert.Empty(t, i18n.DocsPath(i18n.En, "METHOD_NOT_ALLOWED"))
	assert.Equal(t, "install/troubleshooting.md#setup-or-sign-in-fails-with-csrf_failed", i18n.DocsPath(i18n.En, "CSRF_FAILED"))
}
