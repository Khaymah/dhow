package installer

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	engine "github.com/Khaymah/dhow/internal/installer"
)

const (
	readyTimeout  = 3 * time.Minute
	readyInterval = time.Second
)

type Builder struct {
	Sys     System
	Det     Detection
	A       Answers
	M       Messages
	Version string
	Exe     string
	Sleep   func(time.Duration)
	PGDG    PGDGCheck

	restart bool
}

func (b *Builder) sleep(d time.Duration) {
	if b.Sleep != nil {
		b.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (b *Builder) Build() Plan {
	var p Plan
	p.Blockers = b.blockers()
	p.Notes = b.notes()
	if len(p.Blockers) > 0 {
		return p
	}
	p.Steps = append(p.Steps, b.packageSteps()...)
	if b.A.Mode == ModeCompose {
		p.Steps = append(p.Steps, b.composeSteps()...)
	} else {
		p.Steps = append(p.Steps, b.nativeSteps()...)
	}
	p.Steps = append(p.Steps, b.proxySteps()...)
	p.Steps = append(p.Steps, b.firewallSteps()...)
	return p
}

func MissingPackages(det Detection, a Answers) []string {
	var out []string
	if a.Mode == ModeNative && a.Database.Mode == DatabaseInstall && !det.Postgres.Installed18 {
		out = append(out, det.Distro.PostgresPackage())
	}
	switch a.Proxy.Kind {
	case ProxyCaddy:
		if !det.Caddy {
			out = append(out, "caddy")
		}
	case ProxyNginx:
		if !det.Nginx {
			out = append(out, "nginx")
		}
		if !det.Certbot {
			out = append(out, "certbot")
		}
	}
	return out
}

func (b *Builder) blockers() []string {
	var out []string
	det, a, m := b.Det, b.A, b.M
	if !det.Systemd {
		out = append(out, m.T(KeyBlockSystemd))
	}
	if a.Mode == ModeCompose && !det.Compose {
		out = append(out, m.T(KeyBlockCompose))
	}
	missing := MissingPackages(det, a)
	if len(missing) > 0 {
		list := strings.Join(missing, " ")
		switch {
		case !det.Distro.Supported:
			out = append(out, m.T(KeyBlockManual, det.Distro.PrettyName, list))
		case !det.PackagesInstallable():
		case !a.InstallPackages:
			out = append(out, m.T(KeyBlockDeclined, list))
		}
	}
	if det.PackagesInstallable() && a.InstallPackages {
		out = append(out, b.unavailable()...)
		out = append(out, b.pgdgBlockers()...)
	}
	if a.Mode == ModeNative && a.Database.Mode == DatabaseInstall && !det.CanInstallPostgres() {
		out = append(out, m.T(KeyBlockPostgresPort))
	}
	if port := listenPort(a.ListenAddr); port > 0 && det.BusyPort[port] && !det.Existing {
		out = append(out, m.T(KeyBlockListenPort, port))
	}
	if a.ManagedProxy() {
		out = append(out, b.proxyPortBlockers()...)
	}
	blockers, _ := b.resourceBlockers()
	return append(out, blockers...)
}

func (b *Builder) unavailable() []string {
	var out []string
	d := b.Det.Distro
	if b.A.Proxy.Kind == ProxyCaddy && !b.Det.Caddy && !d.CaddyInstallable() {
		out = append(out, b.M.T(KeyBlockNoPackage, "caddy", d.PrettyName))
	}
	if b.A.Proxy.Kind == ProxyNginx && (!b.Det.Nginx || !b.Det.Certbot) && !d.ExtraPackagesInstallable() {
		out = append(out, b.M.T(KeyBlockNoPackage, "nginx/certbot", d.PrettyName))
	}
	return out
}

func (b *Builder) proxyPortBlockers() []string {
	installed := (b.A.Proxy.Kind == ProxyCaddy && b.Det.Caddy) || (b.A.Proxy.Kind == ProxyNginx && b.Det.Nginx)
	if installed || b.Det.Existing {
		return nil
	}
	var out []string
	for _, port := range []int{80, 443} {
		if b.Det.BusyPort[port] {
			out = append(out, b.M.T(KeyBlockProxyPort, port))
		}
	}
	return out
}

func listenPort(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0
	}
	return n
}

func (b *Builder) notes() []string {
	_, out := b.resourceBlockers()
	if !b.Det.Distro.Supported {
		out = append(out, b.M.T(KeyNoteManualMode, b.Det.Distro.PrettyName))
	}
	out = append(out, b.pgdgNotes()...)
	if b.A.Mode == ModeCompose {
		out = append(out, b.M.T(KeyDockerGroupWarning))
	}
	switch b.A.Proxy.Kind {
	case ProxyExisting:
		out = append(out, b.M.T(KeyNoteExistingProxy, b.A.EffectivePublicURL())+"\n\n"+ExistingProxySnippets(b.A)+"\n"+b.M.T(KeyNoteNginxHTTP2))
		if b.A.Proxy.TrustedProxies == "" {
			out = append(out, b.M.T(KeyNoteNoTrustedProxies))
		}
	case ProxyNone:
		out = append(out, b.M.T(KeyNoteNoProxy, b.A.ListenAddr))
	}
	if !b.A.BreachedCheck() {
		out = append(out, b.M.T(KeyNoteBreachedOff))
	}
	return out
}

func (b *Builder) track(s Step) Step {
	apply := s.Apply
	s.Apply = func(ctx context.Context) error {
		b.restart = true
		return apply(ctx)
	}
	return s
}

func (b *Builder) ConfigStep(title string, files []fileSpec, after []Command, done func(context.Context) (bool, error)) Step {
	return engine.FilesStep(b.Sys, title, files, after, done)
}

func (b *Builder) filesMatch(files []fileSpec) (bool, error) { return engine.FilesMatch(b.Sys, files) }

func (b *Builder) readExisting(path string) []byte {
	data, err := b.Sys.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func (b *Builder) serviceActive(ctx context.Context, name string) bool {
	return succeeds(ctx, b.Sys, Cmd("systemctl", "is-active", "--quiet", name))
}

func (b *Builder) waitReady(ctx context.Context) error {
	target := "http://" + upstream(b.A) + "/api/v1/health/ready"
	deadline := b.Sys.Now().Add(readyTimeout)
	var last string
	for {
		res, err := b.Sys.HTTP(ctx, Probe{URL: target})
		if err == nil && res.Status == 200 {
			return nil
		}
		if err != nil {
			last = err.Error()
		} else {
			last = fmt.Sprintf("status %d", res.Status)
		}
		if !b.Sys.Now().Before(deadline) {
			return fmt.Errorf("%s: %s (%s)", target, b.M.T(KeyNotReady), last)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		b.sleep(readyInterval)
	}
}
