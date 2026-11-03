package keys

import (
	"crypto/rand"
	"encoding/hex"
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
	tempSuffixBytes   = 6
	rootUID           = 0
	FileProviderName  = "file"
	readKeyringPrefix = "read the keyring file"
)

type FileRules struct {
	AllowGroupRead bool
}

func ReadKeyringFile(path string, rules FileRules) (*Keyring, error) {
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
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // The path is the operator's own configuration and is checked below.
	if err != nil {
		return nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	if !os.SameFile(link, info) {
		return nil, newError(CodeKeyringInsecure, 0, path+" changed while it was opened")
	}
	if err := checkFileInfo(path, info, rules); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxKeyringBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	defer clear(data)
	return ParseKeyring(data)
}

func checkFileInfo(path string, info fs.FileInfo, rules FileRules) error {
	if !info.Mode().IsRegular() {
		return newError(CodeKeyringInsecure, 0, path+" is not a regular file")
	}
	perm := info.Mode().Perm()
	forbidden := fs.FileMode(otherUsersBits | groupBits)
	if rules.AllowGroupRead {
		forbidden = otherUsersBits | 0o030
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

func ReplaceKeyringFile(path string, ring *Keyring) error {
	current, err := ReadKeyringFile(path, FileRules{})
	if err != nil {
		return err
	}
	current.Destroy()
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", readKeyringPrefix, err)
	}
	suffix := make([]byte, tempSuffixBytes)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("read random bytes: %w", err)
	}
	tmp := path + ".tmp-" + hex.EncodeToString(suffix)
	data := ring.Marshal()
	defer clear(data)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, filePerm) //nolint:gosec // A new file next to the keyring the operator chose.
	if err != nil {
		return fmt.Errorf("write the keyring file: %w", err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && os.Geteuid() == rootUID {
		if err := f.Chown(int(st.Uid), int(st.Gid)); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("keep the owner of the keyring file: %w", err)
		}
	}
	if err := writeAndSync(f, data); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace the keyring file: %w", err)
	}
	return syncDir(filepath.Dir(path))
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
