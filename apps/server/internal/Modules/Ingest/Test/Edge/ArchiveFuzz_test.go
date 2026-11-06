package edge_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"slices"
	"strings"
	"testing"

	ingest "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest"
	"github.com/Khaymah/dhow/internal/archive"
)

const (
	fuzzMaxEntries = 64
	fuzzMaxBytes   = 1 << 20
)

var gzipMagic = []byte{0x1f, 0x8b}

type member struct {
	name     string
	typeflag byte
	link     string
	body     string
}

func craftArchive(t testing.TB, members ...member) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		h := &tar.Header{Name: m.name, Typeflag: m.typeflag, Linkname: m.link, Mode: 0o644, Size: int64(len(m.body)), Format: tar.FormatPAX}
		if m.typeflag != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if m.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(m.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func FuzzIngestArchive(f *testing.F) {
	f.Add(craftArchive(f, member{name: "index.html", typeflag: tar.TypeReg, body: "<h1>x</h1>"}))
	f.Add(craftArchive(f, member{name: "../x", typeflag: tar.TypeReg, body: "x"}))
	f.Add(craftArchive(f, member{name: "/etc/passwd", typeflag: tar.TypeReg, body: "x"}))
	f.Add(craftArchive(f, member{name: "l", typeflag: tar.TypeSymlink, link: "../.."}, member{name: "l/x", typeflag: tar.TypeReg, body: "x"}))
	f.Add(craftArchive(f, member{name: "h", typeflag: tar.TypeLink, link: "/etc/shadow"}))
	f.Add(craftArchive(f, member{name: "a", typeflag: tar.TypeReg, body: strings.Repeat("z", fuzzMaxBytes+1)}))
	f.Add(craftArchive(f, member{name: "dev", typeflag: tar.TypeChar}))
	f.Add(gzipMagic)
	f.Add([]byte("not an archive"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		limits := archive.Limits{MaxEntries: fuzzMaxEntries, MaxBytes: fuzzMaxBytes}
		err := ingest.CheckArchive(bytes.NewReader(data), limits)
		var seen int
		var total int64
		_, walkErr := archive.Walk(bytes.NewReader(data), limits, func(e archive.Entry, body io.Reader) error {
			seen++
			n, copyErr := io.Copy(io.Discard, body)
			total += n
			if e.Name == "" || strings.HasPrefix(e.Name, "/") || slices.Contains(strings.Split(e.Name, "/"), "..") || strings.ContainsAny(e.Name, "\\\x00") {
				t.Fatalf("an unsafe name reached the check: %q", e.Name)
			}
			if e.Kind == archive.KindSymlink && strings.HasPrefix(e.Link, "/") {
				t.Fatalf("an absolute link reached the check: %q", e.Link)
			}
			return copyErr
		})
		if err == nil {
			if walkErr != nil {
				t.Fatalf("the upload check accepted what the walk refuses: %v", walkErr)
			}
			if seen > fuzzMaxEntries || total > fuzzMaxBytes {
				t.Fatalf("accepted %d entries and %d bytes beyond the limits", seen, total)
			}
			return
		}
		if !ingest.IsUnsafeArchive(err) && !bytes.HasPrefix(data, gzipMagic) {
			t.Fatalf("a body that is not gzip must be refused as unsafe, got %v", err)
		}
	})
}
