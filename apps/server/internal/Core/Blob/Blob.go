package blob

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
)

var (
	ErrDigestInvalid  = errors.New("a blob digest is 64 lowercase hex characters")
	ErrDigestMismatch = errors.New("the uploaded bytes do not match the digest")
	ErrTooLarge       = errors.New("the blob is larger than its limit")
	ErrSizeMismatch   = errors.New("the uploaded bytes do not match the declared size")
	ErrNoSpace        = errors.New("the blob store does not have enough free space")
	ErrNotFound       = errors.New("the blob is not in the store")
	ErrDirUnsafe      = errors.New("the blob directory must exist, be a real directory owned by this user and closed to group and others (0700)")
)

type Digest [32]byte

func ParseDigest(s string) (Digest, error) {
	var d Digest
	if len(s) != hex.EncodedLen(len(d)) {
		return d, ErrDigestInvalid
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return d, ErrDigestInvalid
		}
	}
	if _, err := hex.Decode(d[:], []byte(s)); err != nil {
		return d, ErrDigestInvalid
	}
	return d, nil
}

func DigestOf(b []byte) (Digest, bool) {
	var d Digest
	if len(b) != len(d) {
		return d, false
	}
	copy(d[:], b)
	return d, true
}

func (d Digest) String() string { return hex.EncodeToString(d[:]) }

func (d Digest) Key() string {
	s := d.String()
	return s[:2] + "/" + s
}

type Put struct {
	Digest   Digest
	Size     int64
	MaxBytes int64
	Body     io.Reader
}

type Stored struct {
	Digest Digest
	Size   int64
	Key    string
}

type Reader interface {
	io.ReadCloser
	Size() int64
}

type Pending interface {
	Commit() (Stored, error)
	Abort()
}

type Store interface {
	Stage(ctx context.Context, p Put) (Pending, error)
	Open(ctx context.Context, d Digest) (Reader, error)
	Remove(ctx context.Context, d Digest) error
	SweepTemp(ctx context.Context, olderThan int64) (int, error)
}
