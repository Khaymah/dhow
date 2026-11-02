package gitproof

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	EnvPath      = "PIPELINE_PATH"
	ObjectPrefix = "dhow-object:"
	DataPrefix   = "dhow-data:"
	MaxDepth     = 32
	MaxLogBytes  = 8 << 20
)

const Script = `set -eu
emit() {
	printf 'dhow-object:%s\n' "$1"
	git cat-file "$1" "$2" | base64 | sed 's/^/dhow-data:/'
}
fits() {
	[ "$(git cat-file -s "$1")" -le "$2" ] || { printf 'dhow: %s is larger than %s bytes\n' "$3" "$2" >&2; exit 3; }
}
missing() {
	printf 'dhow_found=false\n' >> "$DHOW_OUTPUT"
	exit 0
}
commit=$(git rev-parse --verify --end-of-options 'HEAD^{commit}')
fits "$commit" 1048576 'the commit'
emit commit "$commit"
tree=$(git rev-parse --verify --end-of-options "$commit^{tree}")
rest="$PIPELINE_PATH"
while :; do
	fits "$tree" 1048576 'a tree on the pipeline path'
	emit tree "$tree"
	name="${rest%%/*}"
	entry=$(GIT_LITERAL_PATHSPECS=1 git ls-tree "$tree" -- "$name")
	[ -n "$entry" ] || missing
	mode="${entry%% *}"
	oid="${entry#* * }"
	oid="${oid%%	*}"
	if [ "$rest" = "$name" ]; then
		case "$mode" in 100644 | 100755) ;; *) missing ;; esac
		fits "$oid" 262144 'the pipeline file'
		printf 'dhow_found=true\n' >> "$DHOW_OUTPUT"
		emit blob "$oid"
		exit 0
	fi
	[ "$mode" = 040000 ] || missing
	tree="$oid"
	rest="${rest#*/}"
done
`

type Object struct {
	Kind string
	Data []byte
}

type Proof struct {
	Found  bool
	Source []byte
}

func ParseLog(log []byte) ([]Object, error) {
	if len(log) > MaxLogBytes {
		return nil, fmt.Errorf("%w: the discovery log is too large", ErrUnverified)
	}
	var kinds []string
	var encoded [][]byte
	for line := range bytes.SplitSeq(log, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if kind, ok := bytes.CutPrefix(line, []byte(ObjectPrefix)); ok {
			if len(kinds) == MaxDepth+2 {
				return nil, fmt.Errorf("%w: too many objects", ErrUnverified)
			}
			kinds = append(kinds, string(kind))
			encoded = append(encoded, nil)
			continue
		}
		if data, ok := bytes.CutPrefix(line, []byte(DataPrefix)); ok {
			if len(encoded) == 0 {
				return nil, fmt.Errorf("%w: object data before any object", ErrMalformed)
			}
			encoded[len(encoded)-1] = append(encoded[len(encoded)-1], data...)
		}
	}
	out := make([]Object, 0, len(kinds))
	for i, kind := range kinds {
		data, err := base64.StdEncoding.DecodeString(string(encoded[i]))
		if err != nil {
			return nil, fmt.Errorf("%w: object %d is not base64", ErrMalformed, i)
		}
		out = append(out, Object{Kind: kind, Data: data})
	}
	return out, nil
}

type prover struct {
	f       Format
	objects []Object
	next    int
}

func (p *prover) take(kind, id string, limit int) ([]byte, error) {
	if p.next >= len(p.objects) {
		return nil, fmt.Errorf("%w: the %s %s is missing", ErrUnverified, kind, id)
	}
	o := p.objects[p.next]
	p.next++
	switch {
	case o.Kind != kind:
		return nil, fmt.Errorf("%w: expected a %s, got a %q", ErrUnverified, kind, o.Kind)
	case len(o.Data) > limit:
		return nil, fmt.Errorf("%w: the %s %s is larger than %d bytes", ErrUnverified, kind, id, limit)
	case p.f.ID(kind, o.Data) != id:
		return nil, fmt.Errorf("%w: the %s does not hash to %s", ErrUnverified, kind, id)
	}
	return o.Data, nil
}

func (p *prover) end(proof Proof) (Proof, error) {
	if p.next != len(p.objects) {
		return Proof{}, fmt.Errorf("%w: objects follow the end of the path", ErrUnverified)
	}
	return proof, nil
}

func Prove(sha, path string, objects []Object, maxFile int) (Proof, error) {
	f, err := FormatOf(sha)
	if err != nil {
		return Proof{}, err
	}
	segments := strings.Split(path, "/")
	if len(segments) > MaxDepth || strings.Contains(path, "\x00") {
		return Proof{}, fmt.Errorf("%w: the pipeline path is too deep", ErrUnverified)
	}
	for _, s := range segments {
		if s == "" || s == "." || s == ".." {
			return Proof{}, fmt.Errorf("%w: the pipeline path is not clean", ErrUnverified)
		}
	}
	p := &prover{f: f, objects: objects}
	commit, err := p.take(KindCommit, sha, MaxObjectBytes)
	if err != nil {
		return Proof{}, err
	}
	want, err := f.CommitTree(commit)
	if err != nil {
		return Proof{}, err
	}
	for i, name := range segments {
		body, err := p.take(KindTree, want, MaxObjectBytes)
		if err != nil {
			return Proof{}, err
		}
		entries, err := f.Tree(body)
		if err != nil {
			return Proof{}, err
		}
		e, found := lookup(entries, name)
		last := i == len(segments)-1
		switch {
		case !found, !last && e.Mode != ModeTree, last && e.Mode != ModeFile && e.Mode != ModeExecutable:
			return p.end(Proof{})
		case !last:
			want = e.ID
			continue
		}
		source, err := p.take(KindBlob, e.ID, maxFile)
		if err != nil {
			return Proof{}, err
		}
		return p.end(Proof{Found: true, Source: source})
	}
	return Proof{}, fmt.Errorf("%w: the pipeline path is empty", ErrUnverified)
}

func lookup(entries []Entry, name string) (Entry, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}
