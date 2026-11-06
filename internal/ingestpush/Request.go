package ingestpush

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Khaymah/dhow/internal/gitref"
)

const (
	HeaderName       = "Dhow-Ingest-Name"
	HeaderRef        = "Dhow-Ingest-Ref"
	HeaderRefKind    = "Dhow-Ingest-Ref-Kind"
	HeaderSha        = "Dhow-Ingest-Sha"
	HeaderTrigger    = "Dhow-Ingest-Trigger"
	HeaderProvenance = "Dhow-Ingest-Provenance"

	RefKindBranch = "branch"
	RefKindTag    = "tag"

	TokenPrefix = "dhow_ing_"
	tokenRandom = 43

	ProvenanceFields = 16
	ProvenanceValue  = 256
	ProvenanceBytes  = 4 << 10

	contentTypeGzip = "application/gzip"
	pathPrefix      = "ingest/v1/artifacts"
)

var (
	namePattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	provenanceKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
	tokenPattern         = regexp.MustCompile(`^` + regexp.QuoteMeta(TokenPrefix) + `[A-Za-z0-9_-]{43}$`)
)

type Meta struct {
	Name       string
	Ref        string
	RefKind    string
	Sha        string
	NoTrigger  bool
	Provenance map[string]string
}

func (m Meta) Validate() error {
	if !namePattern.MatchString(m.Name) {
		return ErrName
	}
	kind := m.RefKind
	if kind == "" {
		kind = RefKindBranch
	}
	if kind != RefKindBranch && kind != RefKindTag {
		return ErrRefKind
	}
	if err := checkRef(m.Ref, kind); err != nil {
		return err
	}
	if !gitref.ValidSha(m.Sha) || gitref.IsZeroSha(m.Sha) {
		return ErrSha
	}
	_, err := ProvenanceJSON(m.Provenance)
	return err
}

func checkRef(raw, kind string) error {
	if raw == "" {
		return ErrRef
	}
	prefix, want := gitref.PrefixBranch, gitref.KindBranch
	if kind == RefKindTag {
		prefix, want = gitref.PrefixTag, gitref.KindTag
	}
	full := raw
	if !strings.HasPrefix(raw, "refs/") {
		full = prefix + raw
	}
	ref, err := gitref.Parse(full)
	if err != nil || ref.Kind != want {
		return ErrRef
	}
	return nil
}

func ProvenanceJSON(in map[string]string) (string, error) {
	if len(in) == 0 {
		return "", nil
	}
	if len(in) > ProvenanceFields {
		return "", ErrProvenance
	}
	for k, v := range in {
		if !provenanceKeyPattern.MatchString(k) || !plainText(v, ProvenanceValue) {
			return "", ErrProvenance
		}
	}
	raw, err := json.Marshal(in)
	if err != nil || len(raw) > ProvenanceBytes || !plainText(string(raw), ProvenanceBytes) {
		return "", ErrProvenance
	}
	return string(raw), nil
}

func plainText(s string, most int) bool {
	if len(s) > most || !utf8.ValidString(s) {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) < 0
}

func ValidToken(token string) bool { return tokenPattern.MatchString(token) }

func (m Meta) Headers(size int64) (http.Header, error) {
	provenance, err := ProvenanceJSON(m.Provenance)
	if err != nil {
		return nil, err
	}
	kind := m.RefKind
	if kind == "" {
		kind = RefKindBranch
	}
	h := http.Header{}
	h.Set("Content-Type", contentTypeGzip)
	h.Set(HeaderName, m.Name)
	h.Set(HeaderRef, m.Ref)
	h.Set(HeaderRefKind, kind)
	h.Set(HeaderSha, m.Sha)
	if m.NoTrigger {
		h.Set(HeaderTrigger, "false")
	}
	if provenance != "" {
		h.Set(HeaderProvenance, provenance)
	}
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	return h, nil
}

func Endpoint(server, digest string) (*url.URL, error) {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(server, "#?") {
		return nil, ErrServer
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && loopback(u.Hostname()):
	default:
		return nil, ErrServer
	}
	return u.JoinPath(pathPrefix, digest), nil
}

func loopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}
