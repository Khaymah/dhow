package deployrelease

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"strings"
	"time"
)

const (
	dirName      = ".dhow"
	releasesName = "releases"
	sharedName   = "shared"
	currentName  = "current"
	incomingPref = ".incoming-"
)

type Engine struct {
	Config  Config
	Connect func(ctx context.Context) (Remote, error)
	Doer    Doer
	Report  Reporter
	Log     Logger
	Clock   Clock
}

type run struct {
	e          *Engine
	cfg        Config
	r          Remote
	base       string
	dhow       string
	releases   string
	current    string
	releaseDir string
	incoming   string
	manifest   string
	currentID  string
	previous   string
	known      string
	resumed    bool
	started    bool
	locked     bool
	totals     scan
}

func (e *Engine) Run(ctx context.Context) *Failure {
	cfg := e.Config
	d := &run{e: e, cfg: cfg, base: cfg.DeployPath}
	if e.Report == nil {
		e.Report = DiscardReporter{}
	}
	if e.Clock.Now == nil {
		e.Clock = RealClock()
	}
	if e.Log == nil {
		e.Log = slog.Default()
	}
	if err := cfg.Validate(); err != nil {
		return d.abort(fail(ExitInput, CauseOther, "the deployment settings are not valid", err))
	}
	d.dhow = d.base + "/" + dirName
	d.releases = d.base + "/" + releasesName
	d.current = d.base + "/" + currentName
	d.releaseDir = d.releases + "/" + cfg.ReleaseID
	d.incoming = d.releases + "/" + incomingPref + cfg.ReleaseID
	d.manifest = d.incoming + ".sha256"
	return d.execute(ctx)
}

func (d *run) execute(ctx context.Context) *Failure {
	totals, f := d.scanArchive()
	if f != nil {
		return d.abort(f)
	}
	d.totals = totals
	remote, err := d.e.Connect(ctx)
	if err != nil {
		return d.abort(connectFailure(err))
	}
	d.r = remote
	defer func() { _ = remote.Close() }()
	if f := d.preflight(ctx); f != nil {
		return d.abort(f)
	}
	defer d.unlock()
	d.emitStarted()
	if f := d.stageRelease(ctx); f != nil {
		d.cleanStage()
		return d.abort(f)
	}
	return d.cutOver(ctx)
}

func connectFailure(err error) *Failure {
	if errors.Is(err, ErrHostKeyMismatch) {
		return fail(ExitHostKeyMismatch, CauseOther, "host key mismatch", err)
	}
	return fail(ExitConnect, CauseOther, "cannot connect to the server", err)
}

func (d *run) emitStarted() {
	if d.started {
		return
	}
	d.started = true
	d.e.Report.Emit(Event{Event: EventStarted, ReleaseID: d.cfg.ReleaseID, PreviousReleaseID: d.known, Mode: ModeDeploy})
}

func (d *run) abort(f *Failure) *Failure {
	d.emitStarted()
	d.e.Report.Emit(Event{Event: EventFailed, HostID: d.cfg.HostID, ReleaseID: d.cfg.ReleaseID, Reason: f.Reason, Message: f.Text})
	d.e.Report.Emit(Event{Event: EventFailed, ReleaseID: d.cfg.ReleaseID, Reason: f.Reason, Message: f.Text})
	d.e.Log.Warn("the deployment failed", "reason", f.Reason, "error", f.Error())
	return f
}

func (d *run) cleanStage() {
	if d.r == nil {
		return
	}
	_ = d.r.RemoveAll(d.incoming)
	_ = d.r.Remove(d.manifest)
	if !d.resumed {
		_ = d.r.RemoveAll(d.releaseDir)
	}
}

func (d *run) now() time.Time { return d.e.Clock.Now() }

func releaseOfLink(base, link string) (string, bool) {
	rest, ok := strings.CutPrefix(link, releasesName+"/")
	if !ok {
		rest, ok = strings.CutPrefix(link, base+"/"+releasesName+"/")
	}
	if !ok || strings.Contains(rest, "/") || !validID(rest) {
		return "", false
	}
	return rest, true
}

func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }
