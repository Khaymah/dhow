package keys

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

func AppendKeyringVersion(path string, rules FileRules) (int32, error) {
	f, err := openChecked(path, os.O_RDWR|os.O_APPEND, rules)
	if err != nil {
		return 0, err
	}
	data, err := readAll(f)
	if err != nil {
		_ = f.Close()
		return 0, err
	}
	defer clear(data)
	ring, err := ParseKeyring(data)
	if err != nil {
		_ = f.Close()
		return 0, err
	}
	defer ring.Destroy()
	next, version, err := ring.WithNextVersion()
	if err != nil {
		_ = f.Close()
		return 0, err
	}
	defer next.Destroy()
	line := next.line(version)
	defer clear(line)
	if data[len(data)-1] != '\n' {
		line = append([]byte{'\n'}, line...)
	}
	if err := writeAndSync(f, line); err != nil {
		return 0, err
	}
	return version, nil
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
