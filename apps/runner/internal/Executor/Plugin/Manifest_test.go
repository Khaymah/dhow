package plugin

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const good = `{"Version":1,"Entrypoint":["/usr/local/bin/notify","--quiet"],"Inputs":{"channel":{"Required":true},"message":{"Default":"done"},"color":{}},"Outputs":["url"]}`

func TestParseAndEnv(t *testing.T) {
	m, err := Parse(good)
	require.NoError(t, err)
	env, err := m.Env(map[string]string{"channel": "#ops"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"DHOW_INPUT_CHANNEL": "#ops", "DHOW_INPUT_MESSAGE": "done"}, env)
	env, err = m.Env(map[string]string{"channel": "$(touch /x)", "color": "red", "message": ""})
	require.NoError(t, err)
	assert.Equal(t, "$(touch /x)", env["DHOW_INPUT_CHANNEL"])
	assert.Empty(t, env["DHOW_INPUT_MESSAGE"])

	_, err = m.Env(map[string]string{"channel": "x", "token": "y"})
	assert.ErrorIs(t, err, ErrInputUnknown)
	_, err = m.Env(map[string]string{"message": "y"})
	assert.ErrorIs(t, err, ErrInputMissing)
}

func TestParseRefusesBadManifests(t *testing.T) {
	for _, bad := range []string{
		"",
		"not json",
		`{"Version":2,"Entrypoint":["/x"]}`,
		`{"Version":1,"Entrypoint":[]}`,
		`{"Version":1,"Entrypoint":["notify"]}`,
		`{"Version":1,"Entrypoint":["/a/../b"]}`,
		`{"Version":1,"Entrypoint":["/x"],"Shell":"sh -c"}`,
		`{"Version":1,"Entrypoint":["/x"],"Inputs":{"Bad-Name":{}}}`,
		`{"Version":1,"Entrypoint":["/x"],"Outputs":["a b"]}`,
		`{"Version":1,"Entrypoint":["/x"]} {}`,
		`{"Version":1,"Entrypoint":["/x\u0000"]}`,
		`{"Version":1,"Entrypoint":["/x"],"Inputs":{"a":{"Default":"` + strings.Repeat("x", 33<<10) + `"}}}`,
	} {
		_, err := Parse(bad)
		assert.True(t, errors.Is(err, ErrManifest), bad)
	}
}

func TestParseOutputs(t *testing.T) {
	got, err := ParseOutputs([]byte("url=https://example.com/a=b\n\nid=42\n"), nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"url": "https://example.com/a=b", "id": "42"}, got)
	_, err = ParseOutputs([]byte("id=42\n"), []string{"url"})
	assert.ErrorIs(t, err, ErrOutputs)
	for _, bad := range []string{"no-equals\n", "1x=y\n", "x=" + strings.Repeat("y", MaxOutputValue+1), "x=a\rb\n"} {
		_, err := ParseOutputs([]byte(bad), nil)
		assert.ErrorIs(t, err, ErrOutputs, bad)
	}
	var many strings.Builder
	for i := range MaxOutputs + 1 {
		many.WriteString("k" + strings.Repeat("a", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "=v\n")
	}
	_, err = ParseOutputs([]byte(many.String()), nil)
	assert.ErrorIs(t, err, ErrOutputs)
}
