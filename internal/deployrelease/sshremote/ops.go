package sshremote

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

var _ deployrelease.Remote = (*Client)(nil)

func info(i os.FileInfo) deployrelease.FileInfo {
	return deployrelease.FileInfo{Name: i.Name(), Size: i.Size(), Mode: i.Mode(), ModTime: i.ModTime()}
}

func (c *Client) Lstat(name string) (deployrelease.FileInfo, error) {
	i, err := c.sftp.Lstat(name)
	if err != nil {
		return deployrelease.FileInfo{}, err
	}
	return info(i), nil
}

func (c *Client) ReadDir(name string) ([]deployrelease.FileInfo, error) {
	list, err := c.sftp.ReadDir(name)
	if err != nil {
		return nil, err
	}
	out := make([]deployrelease.FileInfo, 0, len(list))
	for _, i := range list {
		out = append(out, info(i))
	}
	return out, nil
}

func (c *Client) Readlink(name string) (string, error) { return c.sftp.ReadLink(name) }

func (c *Client) Mkdir(name string, mode fs.FileMode) error {
	if err := c.sftp.Mkdir(name); err != nil {
		if _, statErr := c.sftp.Lstat(name); statErr == nil {
			return fmt.Errorf("%w: %s", fs.ErrExist, name)
		}
		return err
	}
	return c.sftp.Chmod(name, mode)
}

func (c *Client) MkdirAll(name string, _ fs.FileMode) error { return c.sftp.MkdirAll(name) }

func (c *Client) Symlink(target, name string) error { return c.sftp.Symlink(target, name) }

func (c *Client) Rename(oldName, newName string) error { return c.sftp.PosixRename(oldName, newName) }

func (c *Client) Remove(name string) error { return c.sftp.Remove(name) }

func (c *Client) RemoveAll(name string) error {
	i, err := c.sftp.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !i.IsDir() {
		return c.sftp.Remove(name)
	}
	return c.sftp.RemoveAll(name)
}

func (c *Client) Create(name string, mode fs.FileMode) (io.WriteCloser, error) {
	f, err := c.sftp.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (c *Client) ReadFile(name string, limit int64) ([]byte, error) {
	f, err := c.sftp.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, limit))
}

func (c *Client) FreeBytes(name string) (uint64, error) {
	v, err := c.sftp.StatVFS(name)
	if err != nil {
		return 0, err
	}
	return v.Bavail * v.Frsize, nil
}
