package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

type Limits struct {
	MaxEntries int
	MaxBytes   int64
}

func validName(name string) (string, bool) {
	if name == "" || len(name) > MaxNameLength || strings.ContainsRune(name, 0) || strings.Contains(name, "\\") {
		return "", false
	}
	if path.IsAbs(name) {
		return "", false
	}
	trimmed := strings.TrimSuffix(name, "/")
	for seg := range strings.SplitSeq(trimmed, "/") {
		if seg == ".." {
			return "", false
		}
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

type unpacker struct {
	root    *os.Root
	limits  Limits
	written int64
}

func Unpack(r io.Reader, dir string, limits Limits) error {
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = MaxEntries
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open the workspace: %w", err)
	}
	defer func() { _ = root.Close() }()
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("%w: not a gzip stream", ErrUnsafe)
	}
	gz.Multistream(false)
	u := &unpacker{root: root, limits: limits}
	tr := tar.NewReader(gz)
	for n := 0; ; n++ {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnsafe, err)
		}
		if n >= limits.MaxEntries {
			return ErrTooLarge
		}
		if err := u.entry(h, tr); err != nil {
			return err
		}
	}
}

func (u *unpacker) parents(name string) error {
	dir := path.Dir(name)
	if dir == "." {
		return nil
	}
	parts := strings.Split(dir, "/")
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		info, err := u.root.Lstat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := u.root.Mkdir(p, dirMode); err != nil {
				return fmt.Errorf("%w: %w", ErrUnsafe, err)
			}
		case err != nil:
			return fmt.Errorf("%w: %w", ErrUnsafe, err)
		case !info.IsDir():
			return fmt.Errorf("%w: %s goes through a link or a file", ErrUnsafe, p)
		}
	}
	return nil
}

func (u *unpacker) clear(name string) error {
	info, err := u.root.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case info.IsDir():
		return fmt.Errorf("%w: %s is a directory", ErrUnsafe, name)
	default:
		return u.root.Remove(name)
	}
}

func (u *unpacker) entry(h *tar.Header, r io.Reader) error {
	name, ok := validName(h.Name)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnsafe, h.Name)
	}
	if err := u.parents(name); err != nil {
		return err
	}
	switch h.Typeflag {
	case tar.TypeDir:
		info, err := u.root.Lstat(name)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%w: %s is not a directory", ErrUnsafe, name)
			}
			return nil
		}
		return u.root.Mkdir(name, dirMode)
	case tar.TypeReg:
		return u.file(name, h, r)
	case tar.TypeSymlink:
		if !LinkStaysInside(name, h.Linkname) {
			return fmt.Errorf("%w: the link %s leaves the workspace", ErrUnsafe, name)
		}
		if err := u.clear(name); err != nil {
			return err
		}
		return u.root.Symlink(h.Linkname, name)
	case tar.TypeLink:
		return u.hardlink(name, h.Linkname)
	default:
		return fmt.Errorf("%w: %s has type %q", ErrUnsafe, name, h.Typeflag)
	}
}

func (u *unpacker) file(name string, h *tar.Header, r io.Reader) error {
	if h.Size < 0 {
		return fmt.Errorf("%w: %s has a negative size", ErrUnsafe, name)
	}
	if u.limits.MaxBytes > 0 && u.written+h.Size > u.limits.MaxBytes {
		return ErrTooLarge
	}
	if err := u.clear(name); err != nil {
		return err
	}
	mode := os.FileMode(fileMode)
	if h.Mode&0o111 != 0 {
		mode = execMode
	}
	f, err := u.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnsafe, err)
	}
	n, err := io.CopyBuffer(f, io.LimitReader(r, h.Size), make([]byte, copyBuffer))
	closeErr := f.Close()
	u.written += n
	if err != nil {
		return err
	}
	if n != h.Size {
		return fmt.Errorf("%w: %s is cut short", ErrUnsafe, name)
	}
	return closeErr
}

func (u *unpacker) hardlink(name, target string) error {
	clean, ok := validName(target)
	if !ok {
		return fmt.Errorf("%w: the hard link %s points out", ErrUnsafe, name)
	}
	info, err := u.root.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: the hard link %s does not point to a file of the archive", ErrUnsafe, name)
	}
	if err := u.parents(clean); err != nil {
		return err
	}
	if err := u.clear(name); err != nil {
		return err
	}
	return u.root.Link(clean, name)
}
