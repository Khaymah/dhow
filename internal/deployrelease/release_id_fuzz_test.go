package deployrelease

import (
	"path"
	"regexp"
	"strings"
	"testing"
	"time"
)

var safeReleaseID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func releaseIDConfig(id string) Config {
	return Config{
		DeployPath: "/srv/app", ReleaseID: id, Keep: 5, HostID: "h", HostName: "web-1", FirstFailure: FirstFailureKeep,
		HookTimeout: time.Minute, ArchivePath: "/tmp/a.tar.gz",
	}
}

func FuzzReleaseID(f *testing.F) {
	for _, seed := range []string{
		"10-aaaaaaaa-0", "r1", "a", "1.2.3", "v1_2-rc.1", "", ".", "..", "-rf", ".hidden", "a/b", "../x", "a b", "a\nb", "a;b", "$(id)",
		"a\x00b", ".incoming-x", "é", "a\u202eb", strings.Repeat("a", 64), strings.Repeat("a", 65), "/abs", "~root", "a:b",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, id string) {
		err := releaseIDConfig(id).Validate()
		if (err == nil) != validID(id) {
			t.Fatalf("Validate and validID disagree about %q", id)
		}
		if err != nil {
			return
		}
		if !safeReleaseID.MatchString(id) {
			t.Fatalf("accepted the release id %q", id)
		}
		if id == "." || id == ".." || strings.HasPrefix(id, "-") || strings.HasPrefix(id, incomingPref) {
			t.Fatalf("accepted the release id %q that names a special entry", id)
		}
		dir := "/srv/app/releases/" + id
		if path.Clean(dir) != dir || path.Base(dir) != id || path.Dir(dir) != "/srv/app/releases" {
			t.Fatalf("the release id %q does not stay one directory name", id)
		}
		line, cerr := Command("mkdir", "--", dir)
		if cerr != nil || strings.ContainsAny(line, "\n\r\t$`\\\"") {
			t.Fatalf("the release id %q cannot be quoted into one command: %q", id, line)
		}
		argv := HookArgv("true", "/srv/app", dir, id, "", time.Minute)
		if _, herr := Command(argv...); herr != nil {
			t.Fatalf("the release id %q cannot be passed to a hook: %v", id, herr)
		}
	})
}
