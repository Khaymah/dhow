package deployrelease

import (
	"context"
	"io"
	"io/fs"
)

func call[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := fn()
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

func callErr(ctx context.Context, fn func() error) error {
	_, err := call(ctx, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

type boundRemote struct {
	Remote
	ctx context.Context
}

func (b boundRemote) Lstat(name string) (FileInfo, error) {
	return call(b.ctx, func() (FileInfo, error) { return b.Remote.Lstat(name) })
}

func (b boundRemote) ReadDir(name string) ([]FileInfo, error) {
	return call(b.ctx, func() ([]FileInfo, error) { return b.Remote.ReadDir(name) })
}

func (b boundRemote) Readlink(name string) (string, error) {
	return call(b.ctx, func() (string, error) { return b.Remote.Readlink(name) })
}

func (b boundRemote) Mkdir(name string, mode fs.FileMode) error {
	return callErr(b.ctx, func() error { return b.Remote.Mkdir(name, mode) })
}

func (b boundRemote) MkdirAll(name string, mode fs.FileMode) error {
	return callErr(b.ctx, func() error { return b.Remote.MkdirAll(name, mode) })
}

func (b boundRemote) Symlink(target, name string) error {
	return callErr(b.ctx, func() error { return b.Remote.Symlink(target, name) })
}

func (b boundRemote) Rename(oldName, newName string) error {
	return callErr(b.ctx, func() error { return b.Remote.Rename(oldName, newName) })
}

func (b boundRemote) Remove(name string) error {
	return callErr(b.ctx, func() error { return b.Remote.Remove(name) })
}

func (b boundRemote) RemoveAll(name string) error {
	return callErr(b.ctx, func() error { return b.Remote.RemoveAll(name) })
}

func (b boundRemote) Create(name string, mode fs.FileMode) (io.WriteCloser, error) {
	return call(b.ctx, func() (io.WriteCloser, error) { return b.Remote.Create(name, mode) })
}

func (b boundRemote) ReadFile(name string, limit int64) ([]byte, error) {
	return call(b.ctx, func() ([]byte, error) { return b.Remote.ReadFile(name, limit) })
}

func (b boundRemote) FreeBytes(name string) (uint64, error) {
	return call(b.ctx, func() (uint64, error) { return b.Remote.FreeBytes(name) })
}

func (b boundRemote) Exec(ctx context.Context, argv []string) (ExecResult, error) {
	merged, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel) //nolint:contextcheck // The bound context only ends this call early.
	defer stop()
	return call(merged, func() (ExecResult, error) { return b.Remote.Exec(merged, argv) })
}

func (b boundRemote) Close() error {
	return callErr(b.ctx, b.Remote.Close)
}

func (d *run) bounded(ctx context.Context, fn func()) {
	orig := d.r
	if orig == nil {
		fn()
		return
	}
	d.r = boundRemote{Remote: orig, ctx: ctx}
	defer func() { d.r = orig }()
	fn()
}
