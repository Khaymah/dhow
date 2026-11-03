package artifacts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"time"

	"github.com/google/uuid"

	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
)

const (
	grantInput byte = 1
	grantCache byte = 2
	grantBytes      = 1 + 8 + sha256.Size
)

type grants struct {
	key []byte
}

func (g grants) mac(kind byte, expires int64, subject uuid.UUID, d blob.Digest) []byte {
	m := hmac.New(sha256.New, g.key)
	var head [9]byte
	head[0] = kind
	binary.BigEndian.PutUint64(head[1:], uint64(expires)) //nolint:gosec // Unix seconds of a near future time are positive.
	_, _ = m.Write(head[:])
	_, _ = m.Write(subject[:])
	_, _ = m.Write(d[:])
	return m.Sum(nil)
}

func (g grants) issue(kind byte, subject uuid.UUID, d blob.Digest, expires time.Time) string {
	out := make([]byte, 0, grantBytes)
	out = append(out, kind)
	out = binary.BigEndian.AppendUint64(out, uint64(expires.Unix())) //nolint:gosec // Unix seconds of a near future time are positive.
	out = append(out, g.mac(kind, expires.Unix(), subject, d)...)
	return base64.RawURLEncoding.EncodeToString(out)
}

func (g grants) verify(token string, d blob.Digest, input, lease uuid.UUID, now time.Time) bool {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != grantBytes {
		return false
	}
	kind := raw[0]
	expires := int64(binary.BigEndian.Uint64(raw[1:9])) //nolint:gosec // Compared against now below; a huge value fails the MAC.
	var subject uuid.UUID
	switch kind {
	case grantInput:
		subject = input
	case grantCache:
		subject = lease
	default:
		return false
	}
	if subject == uuid.Nil || now.Unix() >= expires {
		return false
	}
	return hmac.Equal(raw[9:], g.mac(kind, expires, subject, d))
}
