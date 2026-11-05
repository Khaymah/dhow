package archive_test

import (
	"archive/tar"
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	archive "github.com/Khaymah/dhow/internal/archive"
)

type seen struct {
	entries []archive.Entry
	bodies  map[string]string
}

func walk(t testing.TB, data []byte, limits archive.Limits) (seen, archive.Totals, error) {
	t.Helper()
	s := seen{bodies: map[string]string{}}
	totals, err := archive.Walk(bytes.NewReader(data), limits, func(e archive.Entry, body io.Reader) error {
		s.entries = append(s.entries, e)
		if e.Kind == archive.KindFile {
			raw, err := io.ReadAll(body)
			s.bodies[e.Name] = string(raw)
			return err
		}
		return nil
	})
	return s, totals, err
}

func TestWalkVisitsDirectoriesFilesAndLinksWithNormalizedModes(t *testing.T) {
	data := craft(t,
		tarEntry{name: "app/", typeflag: tar.TypeDir},
		tarEntry{name: "app/index.html", typeflag: tar.TypeReg, body: "hello"},
		tarEntry{name: "app/current", typeflag: tar.TypeSymlink, link: "index.html"},
	)
	got, totals, err := walk(t, data, archive.Limits{})
	require.NoError(t, err)
	require.Len(t, got.entries, 3)
	assert.Equal(t, archive.Entry{Name: "app", Kind: archive.KindDir, Mode: 0o755}, got.entries[0])
	assert.Equal(t, "hello", got.bodies["app/index.html"])
	assert.Equal(t, archive.Entry{Name: "app/current", Kind: archive.KindSymlink, Mode: 0o755, Link: "index.html"}, got.entries[2])
	assert.Equal(t, archive.Totals{Entries: 3, Files: 1, Bytes: 5}, totals)
}

func TestWalkRefusesWhatWouldNotBeSafeOnAServer(t *testing.T) {
	for name, entries := range map[string][]tarEntry{
		"dot dot":                           {{name: "../x", typeflag: tar.TypeReg, body: "x"}},
		"dot dot in the middle":             {{name: "a/../../x", typeflag: tar.TypeReg, body: "x"}},
		"absolute":                          {{name: "/etc/passwd", typeflag: tar.TypeReg, body: "x"}},
		"backslash":                         {{name: `a\b`, typeflag: tar.TypeReg, body: "x"}},
		"newline in a name":                 {{name: "a\nb", typeflag: tar.TypeReg, body: "x"}},
		"control character":                 {{name: "a\x07", typeflag: tar.TypeReg, body: "x"}},
		"absolute link":                     {{name: "l", typeflag: tar.TypeSymlink, link: "/etc"}},
		"link up":                           {{name: "a/l", typeflag: tar.TypeSymlink, link: "../../x"}},
		"link chain through a link":         {{name: "a/q", typeflag: tar.TypeSymlink, link: ".."}, {name: "p", typeflag: tar.TypeSymlink, link: "a/q/../.."}},
		"link chain declared the other way": {{name: "p", typeflag: tar.TypeSymlink, link: "a/q/../.."}, {name: "a/q", typeflag: tar.TypeSymlink, link: ".."}},
		"dot dot after a name":              {{name: "d/l", typeflag: tar.TypeSymlink, link: "x/../y"}},
		"hard link":                         {{name: "a", typeflag: tar.TypeReg, body: "x"}, {name: "b", typeflag: tar.TypeLink, link: "a"}},
		"fifo":                              {{name: "f", typeflag: tar.TypeFifo}},
		"device":                            {{name: "d", typeflag: tar.TypeChar}},
		"entry under a link":                {{name: "l", typeflag: tar.TypeSymlink, link: "d"}, {name: "l/x", typeflag: tar.TypeReg, body: "x"}},
		"same name twice":                   {{name: "a", typeflag: tar.TypeReg, body: "x"}, {name: "a", typeflag: tar.TypeReg, body: "y"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := walk(t, craft(t, entries...), archive.Limits{})
			assert.ErrorIs(t, err, archive.ErrUnsafe)
		})
	}
}

func TestWalkRefusesAStreamThatIsNotGzip(t *testing.T) {
	_, _, err := walk(t, []byte("plain text"), archive.Limits{})
	assert.ErrorIs(t, err, archive.ErrUnsafe)
}

func TestWalkStopsAtTheLimitsBeforeTheBodyIsRead(t *testing.T) {
	data := craft(t, tarEntry{name: "a", typeflag: tar.TypeReg, body: strings.Repeat("x", 100)}, tarEntry{name: "b", typeflag: tar.TypeReg, body: "y"})
	_, _, err := walk(t, data, archive.Limits{MaxBytes: 50})
	assert.ErrorIs(t, err, archive.ErrTooLarge)
	_, _, err = walk(t, data, archive.Limits{MaxEntries: 1})
	assert.ErrorIs(t, err, archive.ErrTooLarge)
}

func TestWalkPassesTheErrorOfTheVisitor(t *testing.T) {
	data := craft(t, tarEntry{name: "a", typeflag: tar.TypeReg, body: "x"})
	want := io.ErrClosedPipe
	_, err := archive.Walk(bytes.NewReader(data), archive.Limits{}, func(archive.Entry, io.Reader) error { return want })
	assert.ErrorIs(t, err, want)
}

func FuzzWalk(f *testing.F) {
	f.Add(craft(f, tarEntry{name: "a/b", typeflag: tar.TypeReg, body: "x"}))
	f.Add(craft(f, tarEntry{name: "../x", typeflag: tar.TypeReg, body: "x"}))
	f.Add(craft(f, tarEntry{name: "l", typeflag: tar.TypeSymlink, link: "d"}, tarEntry{name: "l/x", typeflag: tar.TypeReg}))
	f.Add(craft(f, tarEntry{name: "a/q", typeflag: tar.TypeSymlink, link: ".."}, tarEntry{name: "p", typeflag: tar.TypeSymlink, link: "a/q/../.."}))
	f.Add([]byte{0x1f, 0x8b})
	f.Fuzz(func(t *testing.T, data []byte) {
		links := map[string]bool{}
		_, _ = archive.Walk(bytes.NewReader(data), archive.Limits{MaxEntries: 64, MaxBytes: 1 << 20}, func(e archive.Entry, _ io.Reader) error {
			if e.Name == "" || strings.HasPrefix(e.Name, "/") || slices.Contains(strings.Split(e.Name, "/"), "..") || strings.ContainsAny(e.Name, "\\\n\x00") {
				t.Fatalf("an unsafe name reached the visitor: %q", e.Name)
			}
			for dir := e.Name; strings.Contains(dir, "/"); {
				dir = dir[:strings.LastIndex(dir, "/")]
				if links[dir] {
					t.Fatalf("%q sits under the link %q", e.Name, dir)
				}
			}
			if e.Kind == archive.KindSymlink {
				if strings.HasPrefix(e.Link, "/") {
					t.Fatalf("an absolute link reached the visitor: %q", e.Link)
				}
				seenName := false
				for _, seg := range strings.Split(e.Link, "/") {
					if seg == ".." && seenName {
						t.Fatalf("the link %q goes up after a name: %q", e.Name, e.Link)
					}
					seenName = seenName || (seg != ".." && seg != "." && seg != "")
				}
				links[e.Name] = true
			}
			return nil
		})
	})
}
