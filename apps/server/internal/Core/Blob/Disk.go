package blob

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	tempPrefix  = ".upload-"
	shardMode   = 0o700
	fileMode    = 0o600
	copyBuffer  = 256 << 10
	privateBits = 0o077
)

type DiskOptions struct {
	Dir        string
	Create     bool
	Reserve    int64
	CheckEvery int64
	FreeSpace  func(dir string) (int64, error)
}

type Disk struct {
	dir        string
	reserve    int64
	checkEvery int64
	free       func(dir string) (int64, error)
}

var _ Store = (*Disk)(nil)

func NewDisk(o DiskOptions) (*Disk, error) {
	if !filepath.IsAbs(o.Dir) {
		return nil, fmt.Errorf("%w: %s is not an absolute path", ErrDirUnsafe, o.Dir)
	}
	if o.Create {
		if err := os.MkdirAll(o.Dir, shardMode); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrDirUnsafe, err)
		}
	}
	if err := checkPrivateDir(o.Dir); err != nil {
		return nil, err
	}
	d := &Disk{dir: o.Dir, reserve: o.Reserve, checkEvery: o.CheckEvery, free: o.FreeSpace}
	if d.free == nil {
		d.free = freeBytes
	}
	if d.checkEvery <= 0 {
		d.checkEvery = 64 << 20
	}
	return d, nil
}

func checkPrivateDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDirUnsafe, dir, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	switch {
	case !info.IsDir():
		return fmt.Errorf("%w: %s is not a directory", ErrDirUnsafe, dir)
	case !ok || int(st.Uid) != os.Geteuid():
		return fmt.Errorf("%w: %s is not owned by this user", ErrDirUnsafe, dir)
	case info.Mode().Perm()&privateBits != 0:
		return fmt.Errorf("%w: %s is open to group or others (%04o)", ErrDirUnsafe, dir, info.Mode().Perm())
	}
	if err := unix.Access(dir, unix.W_OK|unix.X_OK); err != nil {
		return fmt.Errorf("%w: %s is not writable: %w", ErrDirUnsafe, dir, err)
	}
	return nil
}

func freeBytes(dir string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * st.Bsize, nil //nolint:gosec // Free blocks times the block size fits in an int64 on any real file system.
}

func (d *Disk) path(dg Digest) (string, string) {
	s := dg.String()
	shard := filepath.Join(d.dir, s[:2])
	return shard, filepath.Join(shard, s)
}

func (d *Disk) roomFor(n int64) error {
	free, err := d.free(d.dir)
	if err != nil {
		return fmt.Errorf("read the free space of the blob store: %w", err)
	}
	if free < n+d.reserve {
		return ErrNoSpace
	}
	return nil
}

type staged struct {
	tmp    *os.File
	shard  string
	final  string
	stored Stored
	done   bool
}

func (s *staged) Commit() (Stored, error) {
	if s.done {
		return Stored{}, ErrNotFound
	}
	s.done = true
	if err := os.Rename(s.tmp.Name(), s.final); err != nil {
		_ = os.Remove(s.tmp.Name())
		return Stored{}, fmt.Errorf("move the blob into place: %w", err)
	}
	if err := syncDir(s.shard); err != nil {
		return Stored{}, err
	}
	return s.stored, nil
}

func (s *staged) Abort() {
	if !s.done {
		s.done = true
		_ = os.Remove(s.tmp.Name())
	}
}

func (d *Disk) Stage(ctx context.Context, p Put) (Pending, error) {
	if p.MaxBytes <= 0 || p.Size > p.MaxBytes {
		return nil, ErrTooLarge
	}
	if p.Size >= 0 {
		if err := d.roomFor(p.Size); err != nil {
			return nil, err
		}
	}
	shard, final := d.path(p.Digest)
	if err := os.Mkdir(shard, shardMode); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("create the blob shard: %w", err)
	}
	if err := checkPrivateDir(shard); err != nil {
		return nil, err
	}
	tmp, err := createTemp(shard)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (Pending, error) {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	n, sum, err := d.copy(ctx, tmp, p)
	switch {
	case err != nil:
		return fail(err)
	case p.Size >= 0 && n != p.Size:
		return fail(ErrSizeMismatch)
	case sum != p.Digest:
		return fail(ErrDigestMismatch)
	}
	if err := tmp.Sync(); err != nil {
		return fail(fmt.Errorf("sync the blob: %w", err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("close the blob: %w", err)
	}
	return &staged{tmp: tmp, shard: shard, final: final, stored: Stored{Digest: p.Digest, Size: n, Key: p.Digest.Key()}}, nil
}

func createTemp(shard string) (*os.File, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("name the upload: %w", err)
	}
	name := filepath.Join(shard, tempPrefix+hex.EncodeToString(b[:]))
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, fileMode) //nolint:gosec // The store root, a digest shard and random bytes.
	if err != nil {
		return nil, fmt.Errorf("create the upload file: %w", err)
	}
	return f, nil
}

func (d *Disk) copy(ctx context.Context, w io.Writer, p Put) (int64, Digest, error) {
	h := sha256.New()
	buf := make([]byte, copyBuffer)
	var n, sinceCheck int64
	for {
		if err := ctx.Err(); err != nil {
			return n, Digest{}, err
		}
		k, rerr := p.Body.Read(buf)
		if k > 0 {
			n += int64(k)
			if n > p.MaxBytes {
				return n, Digest{}, ErrTooLarge
			}
			sinceCheck += int64(k)
			if sinceCheck >= d.checkEvery {
				sinceCheck = 0
				if err := d.roomFor(0); err != nil {
					return n, Digest{}, err
				}
			}
			_, _ = h.Write(buf[:k])
			if _, err := w.Write(buf[:k]); err != nil {
				if errors.Is(err, unix.ENOSPC) {
					return n, Digest{}, ErrNoSpace
				}
				return n, Digest{}, fmt.Errorf("write the blob: %w", err)
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return n, Digest{}, fmt.Errorf("read the upload: %w", rerr)
		}
	}
	var sum Digest
	copy(sum[:], h.Sum(nil))
	return n, sum, nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir) //nolint:gosec // The path comes from the store root and a digest only.
	if err != nil {
		return fmt.Errorf("open the blob shard: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync the blob shard: %w", err)
	}
	return nil
}

type fileReader struct {
	*os.File
	size int64
}

func (r fileReader) Size() int64 { return r.size }

func (d *Disk) Open(_ context.Context, dg Digest) (Reader, error) {
	_, final := d.path(dg)
	f, err := os.OpenFile(final, os.O_RDONLY|unix.O_NOFOLLOW, 0) //nolint:gosec // The path comes from the store root and a parsed digest only.
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, unix.ELOOP) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open the blob: %w", err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, ErrNotFound
	}
	return fileReader{File: f, size: info.Size()}, nil
}

func (d *Disk) Remove(_ context.Context, dg Digest) error {
	_, final := d.path(dg)
	if err := os.Remove(final); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the blob: %w", err)
	}
	return nil
}

func (d *Disk) SweepTemp(ctx context.Context, olderThan int64) (int, error) {
	shards, err := os.ReadDir(d.dir)
	if err != nil {
		return 0, fmt.Errorf("list the blob store: %w", err)
	}
	removed := 0
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(d.dir, shard.Name()))
		if err != nil {
			return removed, fmt.Errorf("list a blob shard: %w", err)
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return removed, ctx.Err()
			}
			if !strings.HasPrefix(e.Name(), tempPrefix) {
				continue
			}
			info, err := e.Info()
			if err != nil || info.ModTime().UnixNano() > olderThan {
				continue
			}
			if err := os.Remove(filepath.Join(d.dir, shard.Name(), e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
