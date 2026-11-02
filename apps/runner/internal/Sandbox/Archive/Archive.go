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
	"sort"
	"strings"
	"time"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	dirMode       = 0o755
	fileMode      = 0o644
	execMode      = 0o755
	MaxEntries    = 200_000
	MaxNameLength = 4096
	copyBuffer    = 128 << 10
)

var (
	ErrUnsafe      = errors.New("the archive has an entry that would leave the workspace, or a device, FIFO or unknown type")
	ErrMissing     = errors.New("an artifact path does not exist in the workspace")
	ErrUnsupported = errors.New("an artifact path holds a device, FIFO, socket or a link that leaves the workspace")
	ErrTooLarge    = errors.New("the archive is larger than its limit")
	ErrPath        = errors.New("an artifact path is not a relative path inside the workspace")
)

var epoch = time.Unix(0, 0).UTC()

type entry struct {
	name string
	info fs.FileInfo
	link string
}

func Pack(dir string, paths []string, w io.Writer) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open the workspace: %w", err)
	}
	defer func() { _ = root.Close() }()
	entries := map[string]entry{}
	for _, p := range paths {
		if !jobspec.ValidWorkspacePath(p) {
			return ErrPath
		}
		if err := collect(root, path.Clean(p), entries); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		if err := writeEntry(root, tw, entries[n]); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func collect(root *os.Root, start string, out map[string]entry) error {
	info, err := root.Lstat(start)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrMissing, start)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return add(root, start, info, out)
	}
	return fs.WalkDir(root.FS(), start, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := root.Lstat(p)
		if err != nil {
			return err
		}
		return add(root, p, info, out)
	})
}

func add(root *os.Root, p string, info fs.FileInfo, out map[string]entry) error {
	if len(out) >= MaxEntries {
		return ErrTooLarge
	}
	e := entry{name: p, info: info}
	switch {
	case info.Mode().IsDir(), info.Mode().IsRegular():
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := root.Readlink(p)
		if err != nil {
			return err
		}
		if !LinkStaysInside(p, target) {
			return fmt.Errorf("%w: %s", ErrUnsupported, p)
		}
		e.link = target
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, p)
	}
	if p != "." {
		out[p] = e
	}
	return nil
}

func LinkStaysInside(name, target string) bool {
	if target == "" || path.IsAbs(target) || strings.ContainsRune(target, 0) || len(target) > MaxNameLength {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	return resolved != ".." && !strings.HasPrefix(resolved, "../")
}

func writeEntry(root *os.Root, tw *tar.Writer, e entry) error {
	h := &tar.Header{Name: e.name, ModTime: epoch, Format: tar.FormatPAX}
	mode := e.info.Mode()
	switch {
	case mode.IsDir():
		h.Typeflag, h.Name, h.Mode = tar.TypeDir, e.name+"/", dirMode
	case mode&fs.ModeSymlink != 0:
		h.Typeflag, h.Linkname, h.Mode = tar.TypeSymlink, e.link, execMode
	default:
		h.Typeflag, h.Mode, h.Size = tar.TypeReg, fileMode, e.info.Size()
		if mode&0o111 != 0 {
			h.Mode = execMode
		}
	}
	if err := tw.WriteHeader(h); err != nil {
		return err
	}
	if h.Typeflag != tar.TypeReg {
		return nil
	}
	f, err := root.OpenFile(e.name, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	n, err := io.CopyBuffer(tw, io.LimitReader(f, h.Size), make([]byte, copyBuffer))
	if err != nil {
		return err
	}
	if n != h.Size {
		return fmt.Errorf("%s changed while it was packed", e.name)
	}
	return nil
}
