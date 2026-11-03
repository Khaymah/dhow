package installer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type PGDGSupport int

const (
	PGDGNotChecked PGDGSupport = iota
	PGDGPublished
	PGDGMissing
	PGDGUnknown
)

const (
	pgdgProbeTimeout = 5 * time.Second
	pgdgListSpan     = 4
)

var ErrPGDGUnsupported = errors.New("PGDG_RELEASE_UNSUPPORTED")

type PGDGCheck struct {
	Support   PGDGSupport
	Release   string
	Supported []string
	Reason    string
}

func BaseArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return goarch
}

func PGDGRepomdURL(d Distro, major int, arch string) string {
	dir := "redhat/rhel-"
	if d.Fedora {
		dir = "fedora/fedora-"
	}
	return DnfRepoBase + dir + strconv.Itoa(major) + "-" + arch + "/repodata/repomd.xml"
}

func NeedsPGDG(det Detection, a Answers) bool {
	d := det.Distro
	return d.Family == FamilyDnf && det.PackagesInstallable() && a.InstallPackages &&
		slices.Contains(MissingPackages(det, a), d.PostgresPackage())
}

func CheckPGDG(ctx context.Context, sys System, det Detection, a Answers) PGDGCheck {
	if !NeedsPGDG(det, a) {
		return PGDGCheck{}
	}
	d := det.Distro
	arch := BaseArch(runtime.GOARCH)
	major := d.Major()
	first := probePGDG(ctx, sys, d, major, arch)
	c := PGDGCheck{Support: first.Support, Release: pgdgRelease(d, major), Reason: first.Reason}
	if c.Support != PGDGMissing || !d.Fedora {
		return c
	}
	for r := max(minFedora, major-pgdgListSpan); r < major; r++ {
		if probePGDG(ctx, sys, d, r, arch).Support == PGDGPublished {
			c.Supported = append(c.Supported, pgdgRelease(d, r))
		}
	}
	return c
}

type pgdgProbe struct {
	Support PGDGSupport
	Reason  string
}

func probePGDG(ctx context.Context, sys System, d Distro, major int, arch string) pgdgProbe {
	res, err := sys.HTTP(ctx, Probe{URL: PGDGRepomdURL(d, major, arch), Timeout: pgdgProbeTimeout})
	switch {
	case err != nil:
		return pgdgProbe{Support: PGDGUnknown, Reason: err.Error()}
	case res.Status == http.StatusOK:
		return pgdgProbe{Support: PGDGPublished}
	case res.Status == http.StatusNotFound:
		return pgdgProbe{Support: PGDGMissing}
	}
	return pgdgProbe{Support: PGDGUnknown, Reason: "status " + strconv.Itoa(res.Status)}
}

func pgdgRelease(d Distro, major int) string {
	if d.Fedora {
		return fmt.Sprintf("Fedora %d", major)
	}
	return d.PrettyName
}

func (b *Builder) pgdgBlockers() []string {
	if b.PGDG.Support != PGDGMissing {
		return nil
	}
	supported := b.M.T(KeyNone)
	if len(b.PGDG.Supported) > 0 {
		supported = strings.Join(b.PGDG.Supported, b.M.T(KeyListSeparator))
	}
	return []string{b.M.T(KeyBlockPGDGRelease, b.PGDG.Release, supported, b.M.T(KeyInstallerDocsURL))}
}

func (b *Builder) pgdgNotes() []string {
	if b.PGDG.Support != PGDGUnknown {
		return nil
	}
	return []string{b.M.T(KeyNotePGDGUnchecked, b.PGDG.Release, b.PGDG.Reason)}
}
