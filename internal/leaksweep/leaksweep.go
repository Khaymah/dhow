package leaksweep

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	MinValueBytes = 12
	chunkBytes    = 4 << 20
	archiveSep    = "!"
)

var (
	ErrShortValue = errors.New("a credential to look for is empty or shorter than 12 bytes")
	ErrNoFiles    = errors.New("a folder to sweep has no file")
	ErrArchive    = errors.New("an archive is past the limits of the sweep")
	gzipMagic     = []byte{0x1f, 0x8b}
)

type Needle struct {
	Name    string
	Forms   [][]byte
	Allowed []string
}

type Pattern struct {
	Name    string
	Re      *regexp.Regexp
	Allowed []string
}

type Hit struct {
	Name string
	File string
}

type Limits struct {
	ArchiveEntries int
	ArchiveBytes   int64
}

type Stats struct {
	Files    int
	Archives int
	Entries  int
}

func DefaultLimits() Limits {
	return Limits{ArchiveEntries: 100_000, ArchiveBytes: 4 << 30}
}

func Encoded(name string, value []byte) (Needle, error) {
	if len(value) < MinValueBytes {
		return Needle{}, fmt.Errorf("%w: %s", ErrShortValue, name)
	}
	n := Needle{Name: name, Forms: [][]byte{
		bytes.Clone(value), []byte(hex.EncodeToString(value)), []byte(strings.ToUpper(hex.EncodeToString(value))),
		escapeJSON(value),
	}}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		for shift := range 3 {
			text := enc.EncodeToString(append(make([]byte, shift), value...))
			start := 0
			if shift > 0 {
				start = 4
			}
			if end := len(text) - 4; end-start >= MinValueBytes {
				n.Forms = append(n.Forms, []byte(text[start:end]))
			}
		}
	}
	return n, nil
}

func escapeJSON(value []byte) []byte {
	r := strings.NewReplacer("\\", `\\`, "\"", `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return []byte(r.Replace(string(value)))
}

func PEMKey(name string, pemText []byte) ([]Needle, error) {
	whole, err := Encoded(name+" (PEM)", pemText)
	if err != nil {
		return nil, err
	}
	raw, err := ssh.ParseRawPrivateKey(pemText)
	if err != nil {
		return nil, fmt.Errorf("%s does not parse as a private key: %w", name, err)
	}
	var key ed25519.PrivateKey
	switch k := raw.(type) {
	case *ed25519.PrivateKey:
		key = *k
	case ed25519.PrivateKey:
		key = k
	default:
		return nil, fmt.Errorf("%s is not an Ed25519 key", name)
	}
	seed, err := Encoded(name+" (private seed)", key.Seed())
	if err != nil {
		return nil, err
	}
	return []Needle{whole, seed}, nil
}

func (n Needle) allowedIn(file string) bool { return allowed(n.Allowed, file) }

func allowed(list []string, file string) bool {
	for _, a := range list {
		if file == a || strings.HasSuffix(file, "/"+a) {
			return true
		}
	}
	return false
}

type scanner struct {
	needles  []Needle
	patterns []Pattern
	overlap  int
	limits   Limits
	stats    Stats
	hits     map[Hit]bool
}

func Sweep(dirs []string, needles []Needle, patterns []Pattern, limits Limits) ([]Hit, Stats, error) {
	s := &scanner{needles: needles, patterns: patterns, limits: limits, hits: map[Hit]bool{}, overlap: 256}
	for _, n := range needles {
		if len(n.Forms) == 0 {
			return nil, s.stats, fmt.Errorf("%w: %s", ErrShortValue, n.Name)
		}
		for _, f := range n.Forms {
			if len(f) < MinValueBytes {
				return nil, s.stats, fmt.Errorf("%w: %s", ErrShortValue, n.Name)
			}
			s.overlap = max(s.overlap, len(f))
		}
	}
	for _, dir := range dirs {
		if err := s.dir(dir); err != nil {
			return nil, s.stats, err
		}
	}
	out := make([]Hit, 0, len(s.hits))
	for h := range s.hits {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File+out[i].Name < out[j].File+out[j].Name })
	return out, s.stats, nil
}

func (s *scanner) dir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("the folder to sweep %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("the folder to sweep %s is not a folder", dir)
	}
	before := s.stats.Files
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		s.stats.Files++
		return s.file(path)
	})
	if err != nil {
		return err
	}
	if s.stats.Files == before {
		return fmt.Errorf("%w: %s", ErrNoFiles, dir)
	}
	return nil
}

func (s *scanner) file(path string) error {
	f, err := os.Open(path) //nolint:gosec // The sweep reads every file under the folders it is given, on purpose.
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(f, 64<<10)
	head, err := br.Peek(len(gzipMagic))
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := s.stream(path, br); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !bytes.Equal(head, gzipMagic) {
		return nil
	}
	s.stats.Archives++
	isTar, err := s.tarball(path, f)
	if err != nil || isTar {
		return err
	}
	return s.gunzip(path, f)
}

func (s *scanner) unzip(path string, f *os.File) (*gzip.Reader, *io.LimitedReader, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s starts like gzip but is not: %w", ErrArchive, path, err)
	}
	return zr, &io.LimitedReader{R: zr, N: s.limits.ArchiveBytes + 1}, nil
}

func (s *scanner) gunzip(path string, f *os.File) error {
	zr, budget, err := s.unzip(path, f)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	if err := s.stream(path+archiveSep, budget); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrArchive, path, err)
	}
	if budget.N <= 0 {
		return fmt.Errorf("%w: %s holds more than %d bytes", ErrArchive, path, s.limits.ArchiveBytes)
	}
	return nil
}

func (s *scanner) tarball(path string, f *os.File) (bool, error) {
	zr, budget, err := s.unzip(path, f)
	if err != nil {
		return false, err
	}
	defer func() { _ = zr.Close() }()
	tr := tar.NewReader(budget)
	for entries := 0; ; entries++ {
		h, err := tr.Next()
		switch {
		case errors.Is(err, io.EOF):
			return true, nil
		case err != nil && entries == 0:
			return false, nil
		case err != nil:
			return true, fmt.Errorf("%w: %s: %w", ErrArchive, path, err)
		case entries >= s.limits.ArchiveEntries:
			return true, fmt.Errorf("%w: %s holds more than %d entries", ErrArchive, path, s.limits.ArchiveEntries)
		}
		s.stats.Entries++
		name := path + archiveSep + h.Name
		if err := s.stream(name, strings.NewReader(h.Name+"\n"+h.Linkname)); err != nil {
			return true, err
		}
		if h.Typeflag == tar.TypeReg {
			if err := s.stream(name, tr); err != nil {
				return true, fmt.Errorf("%w: %s: %w", ErrArchive, name, err)
			}
		}
		if budget.N <= 0 {
			return true, fmt.Errorf("%w: %s holds more than %d bytes", ErrArchive, path, s.limits.ArchiveBytes)
		}
	}
}

func (s *scanner) stream(name string, r io.Reader) error {
	buf := make([]byte, 0, chunkBytes+s.overlap)
	chunk := make([]byte, chunkBytes)
	for {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		s.match(name, buf)
		if len(buf) > s.overlap {
			buf = append(buf[:0], buf[len(buf)-s.overlap:]...)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *scanner) match(name string, buf []byte) {
	for _, n := range s.needles {
		if n.allowedIn(name) {
			continue
		}
		for _, form := range n.Forms {
			if bytes.Contains(buf, form) {
				s.hits[Hit{Name: n.Name, File: name}] = true
				break
			}
		}
	}
	for _, p := range s.patterns {
		if !allowed(p.Allowed, name) && p.Re.Match(buf) {
			s.hits[Hit{Name: p.Name, File: name}] = true
		}
	}
}
