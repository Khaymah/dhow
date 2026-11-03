package keys

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
)

const (
	filePerm          = 0o600
	otherUsersBits    = 0o007
	groupBits         = 0o070
	groupWriteExec    = 0o030
	rootUID           = 0
	FileProviderName  = "file"
	readKeyringPrefix = "read the keyring file"
)

type FileRules struct {
	AllowGroupRead bool
}

func ReadKeyringFile(path string, rules FileRules) (*Keyring, error) {
	f, err := openChecked(path, os.O_RDONLY, rules)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := readAll(f)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	return ParseKeyring(data)
}

func openChecked(path string, flags int, rules FileRules) (*os.File, error) {
	if path == "" {
		return nil, newError(CodeKeyringMissing, 0, "no keyring file is configured; set DHOW_KEYRING_FILE or the systemd credential dhow-keyring")
	}
	link, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, newError(CodeKeyringMissing, 0, path+" does not exist; create it with dhow-server keys init")
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	if link.Mode()&fs.ModeSymlink != 0 {
		return nil, newError(CodeKeyringInsecure, 0, path+" is a symbolic link; point the setting at the file itself")
	}
	if !link.Mode().IsRegular() {
		return nil, newError(CodeKeyringInsecure, 0, path+" is not a regular file")
	}
	f, err := os.OpenFile(path, flags|syscall.O_NOFOLLOW, 0) //nolint:gosec // The path is the operator's own configuration and is checked below.
	if err != nil {
		return nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	info, err := f.Stat()
	if err == nil && !os.SameFile(link, info) {
		err = newError(CodeKeyringInsecure, 0, path+" changed while it was opened")
	}
	if err == nil {
		err = checkFileInfo(path, info, rules)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func readAll(f *os.File) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(f, MaxKeyringBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	return data, nil
}

func checkFileInfo(path string, info fs.FileInfo, rules FileRules) error {
	if !info.Mode().IsRegular() {
		return newError(CodeKeyringInsecure, 0, path+" is not a regular file")
	}
	perm := info.Mode().Perm()
	forbidden := fs.FileMode(otherUsersBits | groupBits)
	if rules.AllowGroupRead {
		forbidden = otherUsersBits | groupWriteExec
	}
	if perm&forbidden != 0 {
		return newError(CodeKeyringInsecure, 0, fmt.Sprintf("%s is open to other users (mode %04o); run chmod 600 %s", path, perm, path))
	}
	if info.Size() > MaxKeyringBytes {
		return newError(CodeKeyringInsecure, 0, fmt.Sprintf("%s is larger than %d bytes", path, MaxKeyringBytes))
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return newError(CodeKeyringInsecure, 0, path+": the owner of the file cannot be read")
	}
	euid := os.Geteuid()
	if int(st.Uid) != euid && st.Uid != rootUID {
		return newError(CodeKeyringInsecure, 0, fmt.Sprintf("%s belongs to user %d; it must belong to the user that runs dhow-server (%d) or to root", path, st.Uid, euid))
	}
	return nil
}

func CreateKeyringFile(path string, ring *Keyring) error {
	if path == "" {
		return newError(CodeKeyringMissing, 0, "no keyring file path was given")
	}
	data := ring.Marshal()
	defer clear(data)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, filePerm) //nolint:gosec // The operator chose the path; O_EXCL never replaces a file.
	if err != nil {
		return fmt.Errorf("create the keyring file: %w", err)
	}
	if err := writeAndSync(f, data); err != nil {
		_ = os.Remove(path)
		return err
	}
	return syncDir(filepath.Dir(path))
}

type writeAtFunc func(f *os.File, data []byte, off int64) (int, error)

func AppendKeyringVersion(path string, rules FileRules) (int32, error) {
	return appendKeyringVersion(path, rules, (*os.File).WriteAt)
}

func appendKeyringVersion(path string, rules FileRules, writeAt writeAtFunc) (int32, error) {
	f, err := openChecked(path, os.O_RDWR, rules)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return 0, fmt.Errorf("lock the keyring file: %w", err)
	}
	size, data, err := readLocked(f)
	if err != nil {
		return 0, err
	}
	defer clear(data)
	ring, err := ParseKeyring(data)
	if err != nil {
		return 0, err
	}
	defer ring.Destroy()
	next, version, err := ring.WithNextVersion()
	if err != nil {
		return 0, err
	}
	defer next.Destroy()
	line := next.line(version)
	defer clear(line)
	if data[len(data)-1] != '\n' {
		line = append([]byte{'\n'}, line...)
	}
	if err := writeVerified(f, writeAt, line, size, next.Versions()); err != nil {
		if rollbackErr := rollback(f, size); rollbackErr != nil {
			return 0, errors.Join(err, rollbackErr)
		}
		return 0, err
	}
	return version, nil
}

func readLocked(f *os.File) (int64, []byte, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	size := info.Size()
	if size > MaxKeyringBytes {
		return 0, nil, newError(CodeKeyringInsecure, 0, fmt.Sprintf("the keyring file is larger than %d bytes", MaxKeyringBytes))
	}
	data := make([]byte, size)
	if _, err := f.ReadAt(data, 0); err != nil && !errors.Is(err, io.EOF) {
		clear(data)
		return 0, nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	return size, data, nil
}

func writeVerified(f *os.File, writeAt writeAtFunc, line []byte, at int64, want []int32) error {
	n, err := writeAt(f, line, at)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("write the keyring file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync the keyring file: %w", err)
	}
	_, written, err := readLocked(f)
	if err != nil {
		return err
	}
	defer clear(written)
	check, err := ParseKeyring(written)
	if err != nil {
		return fmt.Errorf("the keyring file did not read back after the write: %w", err)
	}
	defer check.Destroy()
	if !slices.Equal(check.Versions(), want) {
		return newError(CodeKeyringMalformed, 0, "the keyring file did not read back as written")
	}
	return nil
}

func rollback(f *os.File, size int64) error {
	if err := f.Truncate(size); err != nil {
		return fmt.Errorf("restore the keyring file to its size before the write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync the restored keyring file: %w", err)
	}
	return nil
}

func writeAndSync(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write the keyring file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync the keyring file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close the keyring file: %w", err)
	}
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // The directory of the keyring file the operator chose.
	if err != nil {
		return fmt.Errorf("sync the keyring directory: %w", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync the keyring directory: %w", err)
	}
	return nil
}
