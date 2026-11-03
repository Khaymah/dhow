package keys_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

func encodedKey(seed byte) string {
	key := make([]byte, keys.KeyBytes)
	for i := range key {
		key[i] = seed + byte(i*7)
	}
	return base64.RawURLEncoding.EncodeToString(key)
}

func line(version int, key string) string { return fmt.Sprintf("kek %d %s", version, key) }

func TestParseKeyringAcceptsTheStrictFormat(t *testing.T) {
	text := line(1, encodedKey(1)) + "\n" + line(2, encodedKey(2)) + "\n" + line(7, encodedKey(3)) + "\n"
	ring, err := keys.ParseKeyring([]byte(text))
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 2, 7}, ring.Versions())
	assert.Equal(t, int32(7), ring.Latest())
	assert.True(t, ring.Has(2))
	assert.False(t, ring.Has(3))
	assert.Equal(t, text, string(ring.Marshal()))

	noNewline, err := keys.ParseKeyring([]byte(line(1, encodedKey(1))))
	require.NoError(t, err)
	assert.Equal(t, []int32{1}, noNewline.Versions())
}

func TestParseKeyringRejectsEveryMalformedLine(t *testing.T) {
	good := encodedKey(1)
	repeated := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xab}, keys.KeyBytes))
	many := make([]string, 0, keys.MaxVersions+1)
	for i := range keys.MaxVersions + 1 {
		many = append(many, line(i+1, encodedKey(byte(i+1))))
	}
	tests := []struct {
		name, text, want string
	}{
		{"empty", "", "is empty"},
		{"only a newline", "\n", "line 1"},
		{"blank line between", line(1, good) + "\n\n" + line(2, encodedKey(2)), "line 2"},
		{"two trailing newlines", line(1, good) + "\n\n", "line 2"},
		{"carriage return", line(1, good) + "\r\n", "line 1"},
		{"tab", "kek\t1\t" + good, "line 1"},
		{"double space", "kek  1 " + good, "line 1"},
		{"leading space", " " + line(1, good), "line 1"},
		{"comment", "# keys\n" + line(1, good), "line 1"},
		{"wrong keyword", "KEK 1 " + good, "line 1"},
		{"version zero", line(0, good), "version"},
		{"leading zero", "kek 01 " + good, "version"},
		{"negative", "kek -1 " + good, "version"},
		{"plus sign", "kek +1 " + good, "version"},
		{"too large", "kek 9999999999 " + good, "version"},
		{"short key", line(1, good[:42]), "43 characters"},
		{"long key", line(1, good+"A"), "43 characters"},
		{"padded key", line(1, base64.URLEncoding.EncodeToString(make([]byte, 31))+"x"), "43 characters"},
		{"standard base64", line(1, "///"+good[3:]), "43 characters"},
		{"hex key", line(1, hex.EncodeToString(make([]byte, 32))), "43 characters"},
		{"not random", line(1, repeated), "too little variety"},
		{"duplicate version", line(1, good) + "\n" + line(1, encodedKey(2)), "ascending"},
		{"descending", line(2, good) + "\n" + line(1, encodedKey(2)), "ascending"},
		{"same key twice", line(1, good) + "\n" + line(2, good), "same key"},
		{"too many versions", strings.Join(many, "\n"), "more than 16"},
		{"too large file", strings.Repeat("k", keys.MaxKeyringBytes+1), "larger than"},
		{"byte order mark", "\xef\xbb\xbf" + line(1, good), "line 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := keys.ParseKeyring([]byte(tt.text))
			require.Error(t, err)
			assert.Equal(t, keys.CodeKeyringMalformed, keys.CodeOf(err))
			assert.Contains(t, err.Error(), tt.want)
			assertNoKeyMaterial(t, err.Error(), good, encodedKey(2), repeated)
			var ke *keys.Error
			require.ErrorAs(t, err, &ke)
			assert.Equal(t, keys.ExitKeyring, ke.ExitCode())
		})
	}
}

func assertNoKeyMaterial(t *testing.T, text string, encoded ...string) {
	t.Helper()
	for _, e := range encoded {
		raw, err := base64.RawURLEncoding.DecodeString(e)
		require.NoError(t, err)
		for _, needle := range []string{e, e[:16], hex.EncodeToString(raw), base64.StdEncoding.EncodeToString(raw)[:16], string(raw)} {
			assert.NotContains(t, text, needle)
		}
	}
}

func TestKeyringNeverPrintsItsKeys(t *testing.T) {
	text := line(1, encodedKey(1)) + "\n" + line(2, encodedKey(2)) + "\n"
	ring, err := keys.ParseKeyring([]byte(text))
	require.NoError(t, err)

	var out bytes.Buffer
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
		fmt.Fprintf(&out, verb+"\n", ring)
	}
	fmt.Fprintln(&out, ring)
	js, err := json.Marshal(map[string]any{"ring": ring})
	require.NoError(t, err)
	out.Write(js)
	logger := slog.New(slog.NewJSONHandler(&out, nil))
	logger.Info("loaded", "ring", ring, "any", any(ring))
	slog.New(slog.NewTextHandler(&out, nil)).Info("loaded", "ring", ring)

	assert.Contains(t, out.String(), "keyring(versions 1,2)")
	assertNoKeyMaterial(t, out.String(), encodedKey(1), encodedKey(2))
}

func TestWithNextVersionAddsOneAndStopsAtTheLimit(t *testing.T) {
	ring, err := keys.GenerateKeyring(1, 3)
	require.NoError(t, err)
	next, v, err := ring.WithNextVersion()
	require.NoError(t, err)
	assert.Equal(t, int32(4), v)
	assert.Equal(t, []int32{1, 3, 4}, next.Versions())
	assert.Equal(t, []int32{1, 3}, ring.Versions(), "the original is left alone")
	parsed, err := keys.ParseKeyring(next.Marshal())
	require.NoError(t, err)
	assert.Equal(t, next.Marshal(), parsed.Marshal())

	versions := make([]int32, keys.MaxVersions)
	for i := range versions {
		versions[i] = int32(i + 1)
	}
	full, err := keys.GenerateKeyring(versions...)
	require.NoError(t, err)
	_, _, err = full.WithNextVersion()
	assert.Equal(t, keys.CodeKeyringFull, keys.CodeOf(err))
}

func TestDestroyClearsTheKeys(t *testing.T) {
	ring, err := keys.GenerateKeyring(1)
	require.NoError(t, err)
	ring.Destroy()
	assert.Empty(t, ring.Versions())
	assert.Empty(t, ring.Marshal())
}

func FuzzKeyring(f *testing.F) {
	f.Add([]byte(line(1, encodedKey(1)) + "\n"))
	f.Add([]byte(line(1, encodedKey(1)) + "\n" + line(2, encodedKey(2))))
	f.Add([]byte("kek 01 " + encodedKey(1)))
	f.Add([]byte("kek 1 " + encodedKey(1) + "\r\n"))
	f.Add([]byte("kek 2147483648 " + encodedKey(1)))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		ring, err := keys.ParseKeyring(data)
		if err != nil {
			assert.Equal(t, keys.CodeKeyringMalformed, keys.CodeOf(err))
			for _, field := range strings.Fields(string(data)) {
				if len(field) >= 43 {
					assert.NotContains(t, err.Error(), field)
				}
			}
			return
		}
		versions := ring.Versions()
		assert.NotEmpty(t, versions)
		assert.LessOrEqual(t, len(versions), keys.MaxVersions)
		for i := 1; i < len(versions); i++ {
			assert.Less(t, versions[i-1], versions[i])
		}
		assert.Equal(t, string(bytes.TrimSuffix(data, []byte("\n")))+"\n", string(ring.Marshal()))
		again, err := keys.ParseKeyring(ring.Marshal())
		require.NoError(t, err)
		assert.Equal(t, versions, again.Versions())
		assert.NotContains(t, fmt.Sprintf("%v %+v %#v %x", ring, ring, ring, ring), strings.Fields(string(data))[2])
	})
}
