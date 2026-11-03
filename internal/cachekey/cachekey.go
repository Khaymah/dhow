package cachekey

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Khaymah/dhow/internal/pathglob"
)

const (
	MaxHashes        = 8
	MaxPatterns      = 32
	MaxKeyBytes      = 512
	MaxTemplateBytes = 4096
	MaxHashedFiles   = 100_000
	escape           = '%'
)

var (
	ErrTemplate = errors.New("the cache key template is not valid")
	ErrKey      = errors.New("a cache key is 1 to 512 bytes of printable text")
	ErrPattern  = errors.New("a hash_files pattern is not valid")
	ErrTooMany  = errors.New("hash_files matched too many files")
)

type Builder struct {
	b      strings.Builder
	hashes int
}

func (b *Builder) Text(s string) {
	for _, r := range s {
		if r == escape {
			b.b.WriteRune(escape)
		}
		b.b.WriteRune(r)
	}
}

func (b *Builder) Hash(index int) {
	b.b.WriteRune(escape)
	b.b.WriteString(strconv.Itoa(index))
	b.hashes++
}

func (b *Builder) String() string { return b.b.String() }

func Escape(s string) string {
	var b Builder
	b.Text(s)
	return b.String()
}

func Render(template string, hashes []string) (string, error) {
	if len(template) > MaxTemplateBytes || !utf8.ValidString(template) || len(hashes) > MaxHashes {
		return "", ErrTemplate
	}
	used := make([]bool, len(hashes))
	var out strings.Builder
	for i := 0; i < len(template); i++ {
		c := template[i]
		if c != escape {
			out.WriteByte(c)
			continue
		}
		if i+1 >= len(template) {
			return "", ErrTemplate
		}
		next := template[i+1]
		i++
		if next == escape {
			out.WriteByte(escape)
			continue
		}
		n := int(next - '0')
		if next < '0' || next > '9' || n >= len(hashes) || used[n] {
			return "", ErrTemplate
		}
		used[n] = true
		out.WriteString(hashes[n])
	}
	for _, u := range used {
		if !u {
			return "", ErrTemplate
		}
	}
	key := out.String()
	if !ValidKey(key) {
		return "", ErrKey
	}
	return key, nil
}

func ValidKey(k string) bool {
	return k != "" && len(k) <= MaxKeyBytes && utf8.ValidString(k) && strings.IndexFunc(k, unicode.IsControl) < 0
}

func ValidPatterns(patterns []string) bool {
	if len(patterns) == 0 || len(patterns) > MaxPatterns {
		return false
	}
	for _, p := range patterns {
		if !pathglob.ValidPattern(p) || strings.HasPrefix(p, "/") {
			return false
		}
	}
	return true
}

func HashFiles(fsys fs.FS, patterns []string) (string, error) {
	if !ValidPatterns(patterns) {
		return "", ErrPattern
	}
	var files []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." || !d.Type().IsRegular() {
			return nil
		}
		for _, pat := range patterns {
			if pathglob.MatchPattern(pat, p) {
				files = append(files, p)
				break
			}
		}
		if len(files) > MaxHashedFiles {
			return ErrTooMany
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	total := sha256.New()
	for _, name := range files {
		sum, err := hashFile(fsys, name)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(total, name+"\x00"+sum+"\n")
	}
	return hex.EncodeToString(total.Sum(nil)), nil
}

func hashFile(fsys fs.FS, name string) (string, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
