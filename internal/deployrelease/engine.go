package deployrelease

import (
	"context"
	"errors"
	"io/fs"
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

	LockRenew   time.Duration
	LockTimeout time.Duration
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
	hasFiles   bool
	present    bool
	confirmed  bool

	timeoutKnown bool
	hasTimeout   bool
	switched     bool
	marked       bool
	batch        int
	locked       bool
	stopRenew    func()
	totals       scan
}

func (e *Engine) Run(ctx context.Context) *Failure {
	f := &Fleet{
		Config: e.Config, Hosts: []Host{{ID: e.Config.HostID, Name: e.Config.HostName, Connect: e.Connect}}, BatchSize: 1,
		Doer: e.Doer, Report: e.Report, Log: e.Log, Clock: e.Clock, LockRenew: e.LockRenew,
	}
	return f.Run(ctx)
}

func newRun(e *Engine) *run {
	cfg := e.Config
	d := &run{e: e, cfg: cfg, base: cfg.DeployPath}
	d.dhow = d.base + "/" + dirName
	d.releases = d.base + "/" + releasesName
	d.current = d.base + "/" + currentName
	d.releaseDir = d.releases + "/" + cfg.ReleaseID
	d.incoming = d.releases + "/" + incomingPref + cfg.ReleaseID
	d.manifest = d.incoming + ".sha256"
	return d
}

func connectFailure(err error) *Failure {
	if errors.Is(err, ErrHostKeyMismatch) {
		return fail(ExitHostKeyMismatch, CauseOther, "host key mismatch", err)
	}
	return fail(ExitConnect, CauseOther, "cannot connect to the server", err)
}

func (d *run) cleanStage() {
	if d.r == nil {
		return
	}
	_ = d.r.RemoveAll(d.incoming)
	_ = d.r.Remove(d.manifest)
	if !d.resumed && !d.present {
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
