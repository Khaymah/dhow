package deployrelease

import (
	"context"
	"io"
	"io/fs"
	"time"
)

type FileInfo struct {
	Name    string
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
}

func (f FileInfo) IsDir() bool     { return f.Mode.IsDir() }
func (f FileInfo) IsSymlink() bool { return f.Mode&fs.ModeSymlink != 0 }

type ExecResult struct {
	ExitCode int
	Output   []byte
}

type Remote interface {
	Lstat(name string) (FileInfo, error)
	ReadDir(name string) ([]FileInfo, error)
	Readlink(name string) (string, error)
	Mkdir(name string, mode fs.FileMode) error
	MkdirAll(name string, mode fs.FileMode) error
	Symlink(target, name string) error
	Rename(oldName, newName string) error
	Remove(name string) error
	RemoveAll(name string) error
	Create(name string, mode fs.FileMode) (io.WriteCloser, error)
	ReadFile(name string, limit int64) ([]byte, error)
	FreeBytes(name string) (uint64, error)
	Exec(ctx context.Context, argv []string) (ExecResult, error)
	Close() error
}

func WriteFile(r Remote, name string, data []byte, mode fs.FileMode) error {
	tmp := name + ".tmp"
	_ = r.Remove(tmp)
	w, err := r.Create(tmp, mode)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		_ = r.Remove(tmp)
		return err
	}
	if err := w.Close(); err != nil {
		_ = r.Remove(tmp)
		return err
	}
	return r.Rename(tmp, name)
}
