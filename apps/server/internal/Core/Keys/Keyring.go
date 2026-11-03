package keys

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
)

const (
	KeyBytes         = 32
	MaxVersions      = 16
	MaxKeyringBytes  = 64 << 10
	maxVersionDigits = 9
	encodedKeyLen    = 43
	minDistinctBytes = 16
	lineFields       = 3
	lineKeyword      = "kek"
)

var keyEncoding = base64.RawURLEncoding.Strict()

type Keyring struct {
	versions []int32
	keys     map[int32][]byte
}

func ParseKeyring(data []byte) (*Keyring, error) {
	if len(data) == 0 {
		return nil, newError(CodeKeyringMalformed, 0, "the keyring file is empty")
	}
	if len(data) > MaxKeyringBytes {
		return nil, newError(CodeKeyringMalformed, 0, fmt.Sprintf("the keyring file is larger than %d bytes", MaxKeyringBytes))
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) > MaxVersions {
		return nil, newError(CodeKeyringMalformed, 0, fmt.Sprintf("the keyring file has more than %d versions", MaxVersions))
	}
	ring := &Keyring{keys: make(map[int32][]byte, len(lines))}
	for i, line := range lines {
		version, key, err := parseLine(line)
		if err == nil {
			err = ring.add(version, key)
		}
		if err != nil {
			ring.Destroy()
			return nil, &Error{Code: CodeKeyringMalformed, Version: version, Line: i + 1, Detail: lineDetail(err, i+1, len(lines))}
		}
	}
	return ring, nil
}

func lineDetail(err error, line, last int) string {
	if line != last || last == 1 {
		return err.Error()
	}
	return fmt.Sprintf("%s; this is the last line, so if a keys generate was cut off, delete line %d and run it again", err, line)
}

type lineError string

func (e lineError) Error() string { return string(e) }

const (
	errLineShape   = lineError(`each line must be "kek <version> <key>" with single spaces and nothing else`)
	errLineVersion = lineError("the version must be a whole number from 1 to 999999999 without leading zeros")
	errLineKey     = lineError("the key must be 32 bytes in unpadded base64url (43 characters)")
	errLineRandom  = lineError("the key has too little variety to be random; generate it with dhow-server keys generate")
	errLineOrder   = lineError("versions must be listed once each, in ascending order")
	errLineReused  = lineError("the same key appears under two versions")
)

func parseLine(line []byte) (int32, []byte, error) {
	fields := bytes.Split(line, []byte(" "))
	if len(fields) != lineFields || !bytes.Equal(fields[0], []byte(lineKeyword)) {
		return 0, nil, errLineShape
	}
	version, err := parseVersion(fields[1])
	if err != nil {
		return 0, nil, err
	}
	if len(fields[2]) != encodedKeyLen {
		return version, nil, errLineKey
	}
	key := make([]byte, keyEncoding.DecodedLen(encodedKeyLen))
	n, err := keyEncoding.Decode(key, fields[2])
	if err != nil || n != KeyBytes {
		clear(key)
		return version, nil, errLineKey
	}
	key = key[:n]
	if !looksRandom(key) {
		clear(key)
		return version, nil, errLineRandom
	}
	return version, key, nil
}

func parseVersion(s []byte) (int32, error) {
	if len(s) == 0 || len(s) > maxVersionDigits || s[0] == '0' {
		return 0, errLineVersion
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errLineVersion
		}
		n = n*10 + int64(c-'0')
	}
	return int32(n), nil
}

func looksRandom(key []byte) bool {
	seen := make(map[byte]struct{}, len(key))
	for _, b := range key {
		seen[b] = struct{}{}
	}
	return len(seen) >= minDistinctBytes
}

func (k *Keyring) add(version int32, key []byte) error {
	if n := len(k.versions); n > 0 && version <= k.versions[n-1] {
		clear(key)
		return errLineOrder
	}
	for _, other := range k.keys {
		if subtle.ConstantTimeCompare(other, key) == 1 {
			clear(key)
			return errLineReused
		}
	}
	k.versions = append(k.versions, version)
	k.keys[version] = key
	return nil
}

func GenerateKeyring(versions ...int32) (*Keyring, error) {
	if len(versions) == 0 {
		versions = []int32{1}
	}
	ring := &Keyring{keys: map[int32][]byte{}}
	sorted := slices.Clone(versions)
	slices.Sort(sorted)
	for _, v := range sorted {
		key, err := randomKey()
		if err != nil {
			return nil, err
		}
		if err := ring.add(v, key); err != nil {
			return nil, fmt.Errorf("generate KEK version %d: %w", v, err)
		}
	}
	return ring, nil
}

func randomKey() ([]byte, error) {
	for {
		key := make([]byte, KeyBytes)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("read random bytes: %w", err)
		}
		if looksRandom(key) {
			return key, nil
		}
	}
}

func (k *Keyring) WithNextVersion() (*Keyring, int32, error) {
	if len(k.versions) >= MaxVersions {
		return nil, 0, newError(CodeKeyringFull, 0,
			fmt.Sprintf("the keyring already holds %d versions; remove a retired version first (see keys status)", MaxVersions))
	}
	next := k.Latest() + 1
	key, err := randomKey()
	if err != nil {
		return nil, 0, err
	}
	out := &Keyring{keys: make(map[int32][]byte, len(k.versions)+1)}
	for _, v := range k.versions {
		out.versions = append(out.versions, v)
		out.keys[v] = slices.Clone(k.keys[v])
	}
	if err := out.add(next, key); err != nil {
		out.Destroy()
		return nil, 0, err
	}
	return out, next, nil
}

func (k *Keyring) Versions() []int32 { return slices.Clone(k.versions) }

func (k *Keyring) Latest() int32 {
	if len(k.versions) == 0 {
		return 0
	}
	return k.versions[len(k.versions)-1]
}

func (k *Keyring) Has(version int32) bool {
	_, ok := k.keys[version]
	return ok
}

func (k *Keyring) key(version int32) ([]byte, bool) {
	key, ok := k.keys[version]
	return key, ok
}

func (k *Keyring) Marshal() []byte {
	var b bytes.Buffer
	for _, v := range k.versions {
		b.Write(k.line(v))
	}
	return b.Bytes()
}

func (k *Keyring) line(version int32) []byte {
	out := make([]byte, 0, len(lineKeyword)+maxVersionDigits+encodedKeyLen+3)
	out = append(out, lineKeyword...)
	out = append(out, ' ')
	out = strconv.AppendInt(out, int64(version), 10)
	out = append(out, ' ')
	out = keyEncoding.AppendEncode(out, k.keys[version])
	return append(out, '\n')
}

func (k *Keyring) Destroy() {
	if k == nil {
		return
	}
	for v, key := range k.keys {
		clear(key)
		delete(k.keys, v)
	}
	k.versions = nil
}

func (k *Keyring) describe() string {
	if k == nil {
		return "keyring(nil)"
	}
	parts := make([]string, 0, len(k.versions))
	for _, v := range k.versions {
		parts = append(parts, strconv.FormatInt(int64(v), 10))
	}
	return "keyring(versions " + strings.Join(parts, ",") + ")"
}

func (k *Keyring) String() string { return k.describe() }

func (k *Keyring) GoString() string { return k.describe() }

func (k *Keyring) Format(f fmt.State, _ rune) {
	if _, err := io.WriteString(f, k.describe()); err != nil {
		return
	}
}

func (k *Keyring) LogValue() slog.Value { return slog.StringValue(k.describe()) }

func (k *Keyring) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote(k.describe())), nil }

func (k *Keyring) MarshalText() ([]byte, error) { return []byte(k.describe()), nil }
