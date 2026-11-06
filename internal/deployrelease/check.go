package deployrelease

import (
	"context"
	"errors"
	"slices"
)

const (
	CheckMinFree = 64 << 20

	CheckOK             = "ok"
	CheckHostKey        = "host_key_mismatch"
	CheckUnreachable    = "unreachable"
	CheckAuthFailed     = "auth_failed"
	CheckPathMissing    = "path_missing"
	CheckCurrentNotLink = "current_not_symlink"
	CheckStateCorrupt   = "state_corrupt"
	CheckDiskLow        = "disk_low"
	CheckLocked         = "locked"
	CheckDiverged       = "diverged"
)

var checkMessages = map[string]string{
	CheckOK:             "the server is ready for a deployment",
	CheckHostKey:        "the host key of the server is not the pinned one",
	CheckUnreachable:    "the server cannot be reached",
	CheckAuthFailed:     "the server did not accept the key of the target",
	CheckPathMissing:    "the deploy path is not a directory on the server",
	CheckCurrentNotLink: "current is not a symlink to a release",
	CheckStateCorrupt:   "a state file of Dhow on the server is damaged",
	CheckDiskLow:        "the server has too little free disk space",
	CheckLocked:         "another deployment holds the lock of the target",
	CheckDiverged:       "the server is not on the last release that passed its health check",
}

var blocking = []string{CheckHostKey, CheckUnreachable, CheckAuthFailed, CheckPathMissing, CheckCurrentNotLink, CheckStateCorrupt, CheckDiskLow, CheckLocked}

type Checker struct {
	DeployPath string
	Hosts      []Host
	MinFree    uint64
	Report     Reporter
	Log        Logger
	Clock      Clock
}

type hostFacts struct {
	reason   string
	current  string
	verified string
	free     uint64
	locked   bool
}

func (c *Checker) Run(ctx context.Context) *Failure {
	bad := 0
	for _, h := range c.Hosts {
		facts := c.checkHost(ctx, h)
		if slices.Contains(blocking, facts.reason) {
			bad++
		}
		c.Report.Emit(Event{
			Event: EventChecked, HostID: h.ID, Reason: facts.reason, Message: checkMessages[facts.reason],
			CurrentReleaseID: facts.current, VerifiedReleaseID: facts.verified, FreeBytes: int64(min(facts.free, 1<<62)), Locked: facts.locked,
		})
		c.Log.Info("server checked", "server", h.Name, "result", facts.reason)
	}
	if bad > 0 {
		return fail(ExitCheckFailed, CauseOther, "at least one server of the target is not ready", nil)
	}
	return nil
}

func (c *Checker) checkHost(ctx context.Context, h Host) hostFacts {
	r, err := h.Connect(ctx)
	if err != nil {
		return hostFacts{reason: connectReason(err)}
	}
	defer func() { _ = r.Close() }()
	return c.inspect(r)
}

func connectReason(err error) string {
	switch {
	case errors.Is(err, ErrHostKeyMismatch):
		return CheckHostKey
	case errors.Is(err, ErrAuthFailed):
		return CheckAuthFailed
	default:
		return CheckUnreachable
	}
}

func (c *Checker) inspect(r Remote) hostFacts {
	base := c.DeployPath
	if info, err := r.Lstat(base); err != nil || !info.IsDir() {
		return hostFacts{reason: CheckPathMissing}
	}
	facts := hostFacts{}
	d := &run{r: r, base: base, dhow: base + "/" + dirName, current: base + "/" + currentName, e: &Engine{Clock: c.Clock, Log: c.Log}}
	if f := d.inspectCurrent(); f != nil {
		return hostFacts{reason: CheckCurrentNotLink}
	}
	facts.current = d.currentID
	dh, err := readDhow(r, d.dhow)
	if err != nil {
		return hostFacts{reason: CheckStateCorrupt, current: facts.current}
	}
	facts.verified = dh.verified
	facts.free, _ = r.FreeBytes(base)
	facts.locked = d.lockHeld()
	switch {
	case facts.free < c.minFree():
		facts.reason = CheckDiskLow
	case facts.locked:
		facts.reason = CheckLocked
	case (dh.verified != "" && dh.verified != facts.current) || (dh.hasState && dh.state.State != "idle"):
		facts.reason = CheckDiverged
	default:
		facts.reason = CheckOK
	}
	return facts
}

func (c *Checker) minFree() uint64 {
	if c.MinFree > 0 {
		return c.MinFree
	}
	return CheckMinFree
}

func (d *run) lockHeld() bool {
	dir := d.dhow + "/" + lockName
	if _, err := d.r.Lstat(dir); err != nil {
		return false
	}
	return !d.lockExpired(dir)
}
