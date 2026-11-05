package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"unicode"
)

type Kind int

const (
	KindDir Kind = iota + 1
	KindFile
	KindSymlink
)

type Entry struct {
	Name string
	Kind Kind
	Mode fs.FileMode
	Size int64
	Link string
}

type Visitor func(e Entry, body io.Reader) error

type Totals struct {
	Entries int
	Files   int
	Bytes   int64
}

func hasControl(name string) bool {
	return strings.ContainsFunc(name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
}

func underLink(name string, links map[string]bool) bool {
	for dir := path.Dir(name); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if links[dir] {
			return true
		}
	}
	return false
}

func Walk(r io.Reader, limits Limits, visit Visitor) (Totals, error) {
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = MaxEntries
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return Totals{}, fmt.Errorf("%w: not a gzip stream", ErrUnsafe)
	}
	defer func() { _ = gz.Close() }()
	gz.Multistream(false)
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	links := map[string]bool{}
	var totals Totals
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return totals, nil
		}
		if err != nil {
			return totals, fmt.Errorf("%w: %w", ErrUnsafe, err)
		}
		if totals.Entries >= limits.MaxEntries {
			return totals, ErrTooLarge
		}
		e, err := walkEntry(h, seen, links)
		if err != nil {
			return totals, err
		}
		if e.Kind == KindFile {
			if limits.MaxBytes > 0 && totals.Bytes+e.Size > limits.MaxBytes {
				return totals, ErrTooLarge
			}
			totals.Files++
			totals.Bytes += e.Size
		}
		totals.Entries++
		if err := visit(e, io.LimitReader(tr, e.Size)); err != nil {
			return totals, err
		}
	}
}

func walkEntry(h *tar.Header, seen, links map[string]bool) (Entry, error) {
	name, ok := validName(h.Name)
	if !ok || hasControl(name) {
		return Entry{}, fmt.Errorf("%w: %q", ErrUnsafe, h.Name)
	}
	if seen[name] || underLink(name, links) {
		return Entry{}, fmt.Errorf("%w: %q repeats an entry or sits under a link", ErrUnsafe, name)
	}
	seen[name] = true
	e := Entry{Name: name}
	switch h.Typeflag {
	case tar.TypeDir:
		e.Kind, e.Mode = KindDir, dirMode
	case tar.TypeReg:
		if h.Size < 0 {
			return Entry{}, fmt.Errorf("%w: %s has a negative size", ErrUnsafe, name)
		}
		e.Kind, e.Mode, e.Size = KindFile, fileMode, h.Size
		if h.Mode&0o111 != 0 {
			e.Mode = execMode
		}
	case tar.TypeSymlink:
		if !LinkStaysInside(name, h.Linkname) || hasControl(h.Linkname) {
			return Entry{}, fmt.Errorf("%w: the link %s leaves the release", ErrUnsafe, name)
		}
		e.Kind, e.Mode, e.Link = KindSymlink, execMode, h.Linkname
		links[name] = true
	default:
		return Entry{}, fmt.Errorf("%w: %s has type %q", ErrUnsafe, name, h.Typeflag)
	}
	return e, nil
}
