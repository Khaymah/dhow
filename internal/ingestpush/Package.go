package ingestpush

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/Khaymah/dhow/internal/archive"
)

const (
	packPattern = "dhow-push-*.tar.gz"
	gzipMagic0  = 0x1f
	gzipMagic1  = 0x8b
)

type Package struct {
	Path    string
	Digest  string
	Size    int64
	Packed  bool
	Entries int

	cleanup func()
}

func (p *Package) Close() {
	if p != nil && p.cleanup != nil {
		p.cleanup()
		p.cleanup = nil
	}
}

func Prepare(source string) (*Package, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	switch {
	case info.IsDir():
		return packFolder(source)
	case info.Mode().IsRegular():
		return inspectFile(source, false, nil)
	default:
		return nil, ErrSource
	}
}

func packFolder(dir string) (*Package, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	if len(entries) == 0 {
		return nil, ErrEmpty
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	tmp, err := os.CreateTemp("", packPattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	remove := func() { _ = os.Remove(tmp.Name()) }
	if err := archive.Pack(dir, names, tmp); err != nil {
		_ = tmp.Close()
		remove()
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	if err := tmp.Close(); err != nil {
		remove()
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	p, err := inspectFile(tmp.Name(), true, remove)
	if err != nil {
		remove()
		return nil, err
	}
	return p, nil
}

func inspectFile(path string, packed bool, cleanup func()) (*Package, error) {
	f, err := os.Open(path) //nolint:gosec // The path is the file the user named or our own temporary archive.
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	if info.Size() == 0 {
		return nil, ErrEmpty
	}
	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil || magic[0] != gzipMagic0 || magic[1] != gzipMagic1 {
		return nil, ErrNotGzip
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	entries, err := preflight(f)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSource, err)
	}
	return &Package{Path: path, Digest: hex.EncodeToString(h.Sum(nil)), Size: n, Packed: packed, Entries: entries, cleanup: cleanup}, nil
}

func preflight(r io.Reader) (int, error) {
	totals, err := archive.Walk(r, archive.Limits{}, func(archive.Entry, io.Reader) error { return nil })
	switch {
	case err == nil:
		return totals.Entries, nil
	case errors.Is(err, archive.ErrUnsafe), errors.Is(err, archive.ErrTooLarge):
		return 0, &Error{Class: ClassInput, Code: CodeArchiveUnsafe, Err: err}
	default:
		return 0, fmt.Errorf("%w: %w", ErrSource, err)
	}
}
