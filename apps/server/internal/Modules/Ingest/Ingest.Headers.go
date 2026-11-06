package ingest

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
	"github.com/Khaymah/dhow/internal/gitref"
)

const (
	fieldName       = "Name"
	fieldRef        = "Ref"
	fieldRefKind    = "RefKind"
	fieldSha        = "Sha"
	fieldTrigger    = "Trigger"
	fieldProvenance = "Provenance"
	maxHeaderBytes  = 4 << 10
	valueTrue       = "true"
	valueFalse      = "false"
)

var (
	ErrHeader = errors.New("an ingest header is missing or not valid")

	artifactName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	provenanceKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
)

type HeaderError struct {
	Field string
}

func (e *HeaderError) Error() string { return ErrHeader.Error() + ": " + e.Field }

func (e *HeaderError) Unwrap() error { return ErrHeader }

type Meta struct {
	Name       string
	Ref        string
	RefName    string
	RefKind    string
	Sha        string
	Trigger    bool
	Provenance map[string]string
}

func single(h http.Header, name string) (string, bool) {
	values := h.Values(name)
	switch len(values) {
	case 0:
		return "", true
	case 1:
		v := values[0]
		return v, len(v) <= maxHeaderBytes && utf8.ValidString(v) && noInvisible(v)
	default:
		return "", false
	}
}

func ParseHeaders(h http.Header) (Meta, error) {
	var m Meta
	name, ok := single(h, config.IngestHeaderName)
	if !ok || !artifactName.MatchString(name) {
		return m, &HeaderError{Field: fieldName}
	}
	m.Name = name
	kind, ok := single(h, config.IngestHeaderRefKind)
	if !ok {
		return m, &HeaderError{Field: fieldRefKind}
	}
	if kind == "" {
		kind = models.RefKindBranch
	}
	if kind != models.RefKindBranch && kind != models.RefKindTag {
		return m, &HeaderError{Field: fieldRefKind}
	}
	m.RefKind = kind
	if err := parseRef(h, &m); err != nil {
		return m, err
	}
	sha, ok := single(h, config.IngestHeaderSha)
	if !ok || !gitref.ValidSha(sha) || gitref.IsZeroSha(sha) {
		return m, &HeaderError{Field: fieldSha}
	}
	m.Sha = sha
	trigger, ok := single(h, config.IngestHeaderTrigger)
	switch {
	case !ok, trigger != "" && trigger != valueTrue && trigger != valueFalse:
		return m, &HeaderError{Field: fieldTrigger}
	default:
		m.Trigger = trigger != valueFalse
	}
	raw, ok := single(h, config.IngestHeaderProvenance)
	if !ok {
		return m, &HeaderError{Field: fieldProvenance}
	}
	prov, err := parseProvenance(raw)
	if err != nil {
		return m, err
	}
	m.Provenance = prov
	return m, nil
}

func parseRef(h http.Header, m *Meta) error {
	raw, ok := single(h, config.IngestHeaderRef)
	if !ok || raw == "" {
		return &HeaderError{Field: fieldRef}
	}
	prefix := gitref.PrefixBranch
	want := gitref.KindBranch
	if m.RefKind == models.RefKindTag {
		prefix, want = gitref.PrefixTag, gitref.KindTag
	}
	full := raw
	if !strings.HasPrefix(raw, "refs/") {
		full = prefix + raw
	}
	ref, err := gitref.Parse(full)
	if err != nil || ref.Kind != want {
		return &HeaderError{Field: fieldRef}
	}
	m.Ref, m.RefName = ref.Full, ref.Name
	return nil
}

func parseProvenance(raw string) (map[string]string, error) {
	out := map[string]string{}
	if raw == "" {
		return out, nil
	}
	if len(raw) > config.IngestProvenanceMax {
		return nil, &HeaderError{Field: fieldProvenance}
	}
	var in map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &in); err != nil || len(in) > config.IngestProvenanceFields {
		return nil, &HeaderError{Field: fieldProvenance}
	}
	for k, v := range in {
		var s string
		if !provenanceKey.MatchString(k) || json.Unmarshal(v, &s) != nil || !plainText(s, config.IngestProvenanceValue) {
			return nil, &HeaderError{Field: fieldProvenance}
		}
		out[k] = s
	}
	return out, nil
}

func plainText(s string, most int) bool {
	return len(s) <= most && utf8.ValidString(s) && noInvisible(s)
}

func noInvisible(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) < 0
}
