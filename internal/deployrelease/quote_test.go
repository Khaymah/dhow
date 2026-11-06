package deployrelease_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

var errLex = errors.New("the line is not one simple command")

func lex(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	have := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return nil, errLex
			}
			cur.WriteString(line[i+1 : i+1+end])
			have = true
			i += end + 1
		case c == '\\' && i+1 < len(line) && line[i+1] == '\'':
			cur.WriteByte('\'')
			have = true
			i++
		case c == ' ':
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		case strings.IndexByte("|&;<>()$`\\\"*?[]#~{}!\n\t\r", c) >= 0:
			return nil, errLex
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if have {
		args = append(args, cur.String())
	}
	return args, nil
}

func TestQuotedArgumentsComeBackExactlyAsOneWordEach(t *testing.T) {
	for _, argv := range [][]string{
		{"ls", "-la", "/srv/app"},
		{"echo", "it's", "a \"test\"", "$(id)", "`id`", "a;b", "a|b", "a&&b", "*", "~root", "", " ", "\n", "-n", "=rm", "a=b", "~"},
		{"env", "A=b c", "sh", "-c", `cd "$X" && eval "$1"`, "h", "rm -rf / ; echo 'x'"},
	} {
		line, err := deployrelease.Command(argv...)
		require.NoError(t, err)
		got, err := lex(line)
		require.NoError(t, err, line)
		assert.Equal(t, argv, got, line)
	}
}

func TestACommandNeedsASafeProgramAndNoNulBytes(t *testing.T) {
	for _, argv := range [][]string{
		nil, {""}, {"-rf"}, {"=ls"}, {"rm -rf"}, {"a;b"}, {"$(id)"}, {"ls", "a\x00b"}, {"ls", strings.Repeat("a", 5000)},
	} {
		_, err := deployrelease.Command(argv...)
		assert.ErrorIs(t, err, deployrelease.ErrCommand, "%q", argv)
	}
}

func FuzzRemoteCommand(f *testing.F) {
	f.Add("a", "b c", "it's")
	f.Add("$(id)", "`x`", "a;b|c&d")
	f.Add("", "\n", "\\'")
	f.Fuzz(func(t *testing.T, a, b, c string) {
		argv := []string{"env", a, b, c, "sh"}
		line, err := deployrelease.Command(argv...)
		if err != nil {
			return
		}
		got, err := lex(line)
		if err != nil {
			t.Fatalf("a metacharacter escaped the quoting: %q gave %q", argv, line)
		}
		if len(got) != len(argv) {
			t.Fatalf("%q came back as %q through %q", argv, got, line)
		}
		for i := range argv {
			if got[i] != argv[i] {
				t.Fatalf("argument %d: %q came back as %q through %q", i, argv[i], got[i], line)
			}
		}
	})
}
