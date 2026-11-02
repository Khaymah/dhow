package installer

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pgdgExistingDB = "mode: native\ndomain: dhow.example.com\ndatabase: {mode: existing, url_file: /root/db}\nproxy: {kind: none}\ninstall_packages: true\n"

var errPGDGTimeout = errors.New("dial tcp: i/o timeout")

func fedoraHost(version string) func() *fakeSys {
	return func() *fakeSys {
		f := debianHost()
		f.put(OSReleasePath, "ID=fedora\nVERSION_ID="+version+"\nPRETTY_NAME=\"Fedora Linux "+version+" (Container Image)\"\n", 0o644)
		return f
	}
}

func rockyHost() *fakeSys {
	f := debianHost()
	f.put(OSReleasePath, rockyRelease, 0o644)
	return f
}

type pgdgServer struct {
	status map[string]int
	err    error
	probes []Probe
}

func (s *pgdgServer) serve(f *fakeSys) {
	f.httpFn = func(p Probe) (ProbeResult, error) {
		s.probes = append(s.probes, p)
		if s.err != nil {
			return ProbeResult{}, s.err
		}
		for release, code := range s.status {
			if strings.Contains(p.URL, "/"+release+"-") {
				return ProbeResult{Status: code}, nil
			}
		}
		return ProbeResult{Status: http.StatusNotFound}, nil
	}
}

func publishedFedora(releases ...string) map[string]int {
	out := map[string]int{}
	for _, r := range releases {
		out["fedora-"+r] = http.StatusOK
	}
	return out
}

func TestPGDGRepomdURL(t *testing.T) {
	fedora := ParseOSRelease([]byte("ID=fedora\nVERSION_ID=45\n"))
	rocky := ParseOSRelease([]byte(rockyRelease))
	assert.Equal(t, "https://download.postgresql.org/pub/repos/yum/18/fedora/fedora-45-x86_64/repodata/repomd.xml", PGDGRepomdURL(fedora, 45, BaseArch("amd64")))
	assert.Equal(t, "https://download.postgresql.org/pub/repos/yum/18/redhat/rhel-9-aarch64/repodata/repomd.xml", PGDGRepomdURL(rocky, 9, BaseArch("arm64")))
	assert.Equal(t, "ppc64le", BaseArch("ppc64le"))
}

func TestCheckPGDG(t *testing.T) {
	tests := []struct {
		name   string
		host   func() *fakeSys
		yaml   string
		status map[string]int
		err    error
		want   PGDGCheck
		probes int
	}{
		{"fedora 44 is published", fedoraHost("44"), ciAnswers, publishedFedora("44"), nil, PGDGCheck{Support: PGDGPublished, Release: "Fedora 44"}, 1},
		{"fedora 45 is missing and lists the published releases", fedoraHost("45"), ciAnswers, publishedFedora("42", "43", "44"), nil, PGDGCheck{Support: PGDGMissing, Release: "Fedora 45", Supported: []string{"Fedora 42", "Fedora 43", "Fedora 44"}}, 5},
		{"a network error is not missing", fedoraHost("45"), ciAnswers, nil, errPGDGTimeout, PGDGCheck{Support: PGDGUnknown, Release: "Fedora 45", Reason: errPGDGTimeout.Error()}, 1},
		{"a server error is not missing", fedoraHost("45"), ciAnswers, map[string]int{"fedora-45": http.StatusServiceUnavailable}, nil, PGDGCheck{Support: PGDGUnknown, Release: "Fedora 45", Reason: "status 503"}, 1},
		{"a redirect is not followed or missing", fedoraHost("45"), ciAnswers, map[string]int{"fedora-45": http.StatusFound}, nil, PGDGCheck{Support: PGDGUnknown, Release: "Fedora 45", Reason: "status 302"}, 1},
		{"rocky 9 is published", rockyHost, ciAnswers, map[string]int{"rhel-9": http.StatusOK}, nil, PGDGCheck{Support: PGDGPublished, Release: "Rocky Linux 9.4 (Blue Onyx)"}, 1},
		{"rocky 9 missing lists nothing", rockyHost, ciAnswers, nil, nil, PGDGCheck{Support: PGDGMissing, Release: "Rocky Linux 9.4 (Blue Onyx)"}, 1},
		{"debian uses apt and is not checked", debianHost, ciAnswers, nil, nil, PGDGCheck{}, 0},
		{"an existing database needs no PGDG", fedoraHost("45"), pgdgExistingDB, nil, nil, PGDGCheck{}, 0},
		{"declined packages are not checked", fedoraHost("45"), strings.Replace(ciAnswers, "install_packages: true", "install_packages: false", 1), nil, nil, PGDGCheck{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.host()
			srv := &pgdgServer{status: tt.status, err: tt.err}
			srv.serve(f)
			got := CheckPGDG(context.Background(), f, Detect(context.Background(), f), answers(t, tt.yaml))
			assert.Equal(t, tt.want, got)
			require.Len(t, srv.probes, tt.probes)
			for _, p := range srv.probes {
				assert.True(t, strings.HasPrefix(p.URL, DnfRepoBase), p.URL)
				assert.True(t, strings.HasSuffix(p.URL, "/repodata/repomd.xml"), p.URL)
				assert.Contains(t, []string{"", http.MethodGet}, p.Method)
				assert.Positive(t, p.Timeout)
				assert.LessOrEqual(t, p.Timeout, 10*time.Second)
			}
		})
	}
}

func TestInstallChecksPGDGBeforeChangingTheHost(t *testing.T) {
	tests := []struct {
		name    string
		version string
		lang    string
		status  map[string]int
		err     error
		blocked bool
		want    []string
	}{
		{
			name: "unsupported release stops in English", version: "45", lang: LangEn, status: publishedFedora("42", "43", "44"), blocked: true,
			want: []string{
				"PostgreSQL's package repository (PGDG) does not support Fedora 45 yet, so the installer cannot install PostgreSQL 18 here.",
				"Nothing was changed on this host. Supported releases: Fedora 42, Fedora 43, Fedora 44.",
				"Install on a supported release, or use the Docker Compose install (mode: compose).",
				"https://github.com/Khaymah/dhow/blob/main/docs/en/install/installer.md#supported-systems",
			},
		},
		{
			name: "unsupported release stops in Arabic", version: "45", lang: LangAr, status: publishedFedora("42", "43", "44"), blocked: true,
			want: []string{
				"مستودع PostgreSQL (PGDG) لا يدعم Fedora 45 بعد، لذلك لا يستطيع installer تثبيت PostgreSQL 18 هنا.",
				"لم نغيّر شيئًا في هذا السيرفر. الإصدارات المدعومة: Fedora 42، Fedora 43، Fedora 44.",
				"ثبّت داو على إصدار مدعوم، أو استخدم التثبيت بـ Docker Compose (mode: compose).",
				"https://github.com/Khaymah/dhow/blob/main/docs/install/installer.md#الأنظمة-المدعومة",
			},
		},
		{
			name: "supported release continues", version: "44", lang: LangEn, status: publishedFedora("44"),
			want: []string{DnfRepoPath, DnfExcludePath, "postgresql18-server"},
		},
		{
			name: "network error continues with a note in English", version: "45", lang: LangEn, err: errPGDGTimeout,
			want: []string{"Could not check whether PostgreSQL's package repository (PGDG) supports Fedora 45 (dial tcp: i/o timeout).", DnfRepoPath, "postgresql18-server"},
		},
		{
			name: "network error continues with a note in Arabic", version: "45", lang: LangAr, err: errPGDGTimeout,
			want: []string{"لم نتمكن من التحقق من أن مستودع PostgreSQL (PGDG) يدعم Fedora 45 (dial tcp: i/o timeout).", DnfRepoPath, "postgresql18-server"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := fedoraHost(tt.version)()
			(&pgdgServer{status: tt.status, err: tt.err}).serve(f)
			f.put("/root/answers.yaml", ciAnswers, 0o600)
			before := slices.Sorted(maps.Keys(f.files))
			app, out := newApp(f, "")
			err := app.Install(context.Background(), InstallOptions{Answers: "/root/answers.yaml", Yes: true, DryRun: !tt.blocked, Lang: tt.lang})
			for _, w := range tt.want {
				assert.Contains(t, out.String(), w)
			}
			assert.Equal(t, before, slices.Sorted(maps.Keys(f.files)), "the installer changed files")
			for _, c := range f.commands {
				assert.False(t, strings.HasPrefix(c, "rpm --import") || strings.HasPrefix(c, "dnf "), c)
			}
			if !tt.blocked {
				require.NoError(t, err, out.String())
				return
			}
			require.ErrorIs(t, err, ErrBlocked)
			require.ErrorIs(t, err, ErrPGDGUnsupported)
			assert.Contains(t, err.Error(), "PGDG_RELEASE_UNSUPPORTED")
		})
	}
}
