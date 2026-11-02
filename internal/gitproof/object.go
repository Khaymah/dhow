package gitproof

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // Git names SHA-1 objects by this hash; a forged object would need a second preimage of a commit id the server already holds.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strconv"

	"github.com/Khaymah/dhow/internal/gitref"
)

const (
	KindCommit = "commit"
	KindTree   = "tree"
	KindBlob   = "blob"

	ModeTree       = "40000"
	ModeFile       = "100644"
	ModeExecutable = "100755"
	ModeSymlink    = "120000"
	ModeSubmodule  = "160000"

	MaxObjectBytes = 1 << 20
)

var (
	ErrUnverified = errors.New("the git objects do not prove the file")
	ErrMalformed  = fmt.Errorf("%w: a git object is malformed", ErrUnverified)
)

type Format int

const (
	SHA1 Format = iota + 1
	SHA256
)

func FormatOf(sha string) (Format, error) {
	if !gitref.ValidSha(sha) {
		return 0, fmt.Errorf("%w: the commit id is not a lowercase hex object id", ErrUnverified)
	}
	if len(sha) == gitref.ShaBytesSHA256 {
		return SHA256, nil
	}
	return SHA1, nil
}

func (f Format) size() int {
	if f == SHA256 {
		return sha256.Size
	}
	return sha1.Size
}

func (f Format) hash() hash.Hash {
	if f == SHA256 {
		return sha256.New()
	}
	return sha1.New() //nolint:gosec // Git names SHA-1 objects by this hash; a forged object would need a second preimage.
}

func (f Format) HexSize() int { return 2 * f.size() }

func (f Format) ID(kind string, body []byte) string {
	h := f.hash()
	h.Write([]byte(kind + " " + strconv.Itoa(len(body)) + "\x00"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

type Entry struct {
	Mode string
	Name string
	ID   string
}

func (f Format) CommitTree(body []byte) (string, error) {
	rest, ok := bytes.CutPrefix(body, []byte("tree "))
	if !ok || len(rest) < f.HexSize()+1 || rest[f.HexSize()] != '\n' {
		return "", fmt.Errorf("%w: a commit must start with its tree", ErrMalformed)
	}
	id := string(rest[:f.HexSize()])
	if !gitref.ValidSha(id) {
		return "", fmt.Errorf("%w: the tree id of the commit is not lowercase hex", ErrMalformed)
	}
	return id, nil
}

func (f Format) Tree(body []byte) ([]Entry, error) {
	var out []Entry
	seen := map[string]bool{}
	for len(body) > 0 {
		sp := bytes.IndexByte(body, ' ')
		if sp <= 0 {
			return nil, fmt.Errorf("%w: a tree entry has no mode", ErrMalformed)
		}
		mode := string(body[:sp])
		if !validMode(mode) {
			return nil, fmt.Errorf("%w: a tree entry has an unknown mode", ErrMalformed)
		}
		body = body[sp+1:]
		nul := bytes.IndexByte(body, 0)
		if nul <= 0 || len(body) < nul+1+f.size() {
			return nil, fmt.Errorf("%w: a tree entry is cut short", ErrMalformed)
		}
		name := string(body[:nul])
		if name == "." || name == ".." || bytes.IndexByte(body[:nul], '/') >= 0 || seen[name] {
			return nil, fmt.Errorf("%w: a tree entry has an invalid or repeated name", ErrMalformed)
		}
		seen[name] = true
		out = append(out, Entry{Mode: mode, Name: name, ID: hex.EncodeToString(body[nul+1 : nul+1+f.size()])})
		body = body[nul+1+f.size():]
	}
	return out, nil
}

func validMode(mode string) bool {
	switch mode {
	case ModeTree, ModeFile, ModeExecutable, ModeSymlink, ModeSubmodule:
		return true
	}
	return false
}
