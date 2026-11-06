package edge_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	ingest "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest"
	"github.com/Khaymah/dhow/internal/gitref"
)

const sha = "1111111111111111111111111111111111111111"

var nameRule = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func headers(name, ref, kind, sha, trigger, provenance string) http.Header {
	h := http.Header{}
	for k, v := range map[string]string{
		config.IngestHeaderName: name, config.IngestHeaderRef: ref, config.IngestHeaderRefKind: kind, config.IngestHeaderSha: sha,
		config.IngestHeaderTrigger: trigger, config.IngestHeaderProvenance: provenance,
	} {
		if v != "" {
			h.Set(k, v)
		}
	}
	return h
}

func TestGoodHeadersGiveAFullRefAndAShortName(t *testing.T) {
	m, err := ingest.ParseHeaders(headers("web", "release/1.x", "branch", sha, "", `{"Actor":"sara","RunId":"7"}`))
	require.NoError(t, err)
	assert.Equal(t, "refs/heads/release/1.x", m.Ref)
	assert.Equal(t, "release/1.x", m.RefName)
	assert.True(t, m.Trigger, "a run starts unless the uploader says false")
	assert.Equal(t, map[string]string{"Actor": "sara", "RunId": "7"}, m.Provenance)

	m, err = ingest.ParseHeaders(headers("web", "refs/tags/v1.0.0", "tag", sha, "false", ""))
	require.NoError(t, err)
	assert.Equal(t, "refs/tags/v1.0.0", m.Ref)
	assert.False(t, m.Trigger)
}

func TestBadHeadersNameTheFieldAndNeverBecomeAnotherRef(t *testing.T) {
	tests := map[string]http.Header{
		"no name":                   headers("", "main", "branch", sha, "", ""),
		"a name that is a path":     headers("../web", "main", "branch", sha, "", ""),
		"a tag ref for a branch":    headers("web", "refs/tags/v1", "branch", sha, "", ""),
		"a pull request ref":        headers("web", "refs/pull/4/head", "branch", sha, "", ""),
		"an unknown kind":           headers("web", "main", "pull", sha, "", ""),
		"no ref":                    headers("web", "", "branch", sha, "", ""),
		"a ref with a dot dot":      headers("web", "a/../b", "branch", sha, "", ""),
		"a short sha":               headers("web", "main", "branch", "abc123", "", ""),
		"the zero sha":              headers("web", "main", "branch", strings.Repeat("0", 40), "", ""),
		"an upper case sha":         headers("web", "main", "branch", strings.ToUpper(sha), "", ""),
		"a trigger that is a word":  headers("web", "main", "branch", sha, "yes", ""),
		"provenance that is a list": headers("web", "main", "branch", sha, "", `["a"]`),
		"provenance with a number":  headers("web", "main", "branch", sha, "", `{"RunId":7}`),
		"provenance with a bad key": headers("web", "main", "branch", sha, "", `{"bad key":"x"}`),
		"too much provenance":       headers("web", "main", "branch", sha, "", `{"A":"`+strings.Repeat("x", 300)+`"}`),
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ingest.ParseHeaders(h)
			require.ErrorIs(t, err, ingest.ErrHeader)
		})
	}
	twice := headers("web", "main", "branch", sha, "", "")
	twice.Add(config.IngestHeaderName, "api")
	_, err := ingest.ParseHeaders(twice)
	require.ErrorIs(t, err, ingest.ErrHeader, "a header sent twice is refused, never merged")
}

func FuzzIngestHeaders(f *testing.F) {
	f.Add("web", "main", "branch", sha, "true", `{"Actor":"sara"}`)
	f.Add("web", "refs/tags/v1", "tag", sha, "false", "")
	f.Add("../web", "refs/heads/../x", "branch", "zz", "maybe", `{"a":1}`)
	f.Add("web\r\nX-Evil: 1", "main\x00", "branch", sha, "", `{"A":"\u0000"}`)
	f.Fuzz(func(t *testing.T, name, ref, kind, commit, trigger, provenance string) {
		m, err := ingest.ParseHeaders(headers(name, ref, kind, commit, trigger, provenance))
		if err != nil {
			return
		}
		if !nameRule.MatchString(m.Name) {
			t.Fatalf("accepted the name %q", m.Name)
		}
		parsed, perr := gitref.Parse(m.Ref)
		if perr != nil || parsed.Name != m.RefName {
			t.Fatalf("accepted the ref %q as %q", ref, m.Ref)
		}
		if (m.RefKind == "branch") != (parsed.Kind == gitref.KindBranch) || (m.RefKind == "tag") != (parsed.Kind == gitref.KindTag) {
			t.Fatalf("the kind %q does not match the ref %q", m.RefKind, m.Ref)
		}
		if !gitref.ValidSha(m.Sha) || gitref.IsZeroSha(m.Sha) {
			t.Fatalf("accepted the sha %q", m.Sha)
		}
		if len(m.Provenance) > config.IngestProvenanceFields {
			t.Fatalf("accepted %d provenance fields", len(m.Provenance))
		}
		for k, v := range m.Provenance {
			if len(v) > config.IngestProvenanceValue || strings.IndexFunc(k+v, unicode.IsControl) >= 0 {
				t.Fatalf("accepted the provenance %q=%q", k, v)
			}
		}
	})
}
