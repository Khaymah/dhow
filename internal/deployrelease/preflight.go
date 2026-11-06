package deployrelease

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"time"
)

const (
	verifiedName = "verified"
	stateName    = "state.json"
	lockName     = "lock"
	ownerName    = "owner"
	maxSmallFile = 4 << 10
)

type lockOwner struct {
	Release   string    `json:"Release"`
	ExpiresAt time.Time `json:"ExpiresAt"`
}

type stateFile struct {
	State    string    `json:"State"`
	Release  string    `json:"Release"`
	Previous string    `json:"Previous"`
	At       time.Time `json:"At"`
}

func (d *run) preflight(ctx context.Context) *Failure {
	if err := ctx.Err(); err != nil {
		return fail(ExitFailed, CauseCanceled, "the deployment was canceled", err)
	}
	info, err := d.r.Lstat(d.base)
	if err != nil || !info.IsDir() {
		return fail(ExitFailed, CauseOther, "the deploy path is not a directory on the server", err)
	}
	free, err := d.r.FreeBytes(d.base)
	if err != nil {
		return fail(ExitFailed, CauseOther, "cannot read the free space of the server", err)
	}
	if need := uint64(max(d.totals.Bytes, 0)) * 2; free < need {
		return fail(ExitFailed, CauseOther, "not enough disk space on the server", ErrNoSpace)
	}
	if f := d.inspectCurrent(); f != nil {
		return f
	}
	for _, dir := range []string{d.dhow, d.releases, d.base + "/" + sharedName} {
		if err := d.r.MkdirAll(dir, 0o755); err != nil {
			return fail(ExitFailed, CauseOther, "cannot create the release directories", err)
		}
	}
	if f := d.lock(); f != nil {
		return f
	}
	return d.choosePrevious()
}

func (d *run) inspectCurrent() *Failure {
	info, err := d.r.Lstat(d.current)
	switch {
	case isNotExist(err):
		return nil
	case err != nil:
		return fail(ExitFailed, CauseOther, "cannot look at current", err)
	case !info.IsSymlink():
		return fail(ExitCurrentNotLink, CauseOther, "current is not a symlink", ErrCurrentNotLink)
	}
	link, err := d.r.Readlink(d.current)
	if err != nil {
		return fail(ExitFailed, CauseOther, "cannot read current", err)
	}
	id, ok := releaseOfLink(d.base, link)
	if !ok {
		return fail(ExitCurrentNotLink, CauseOther, "current points outside the releases directory", ErrCurrentNotLink)
	}
	d.currentID = id
	return nil
}

func (d *run) readSmall(name string) (string, bool) {
	raw, err := d.r.ReadFile(name, maxSmallFile)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(raw)), true
}

func (d *run) releaseExists(id string) bool {
	info, err := d.r.Lstat(d.releases + "/" + id)
	return err == nil && info.IsDir()
}

func (d *run) choosePrevious() *Failure {
	verified, _ := d.readSmall(d.dhow + "/" + verifiedName)
	if !validID(verified) || !d.releaseExists(verified) {
		verified = ""
	}
	candidate := d.currentID
	if verified != "" {
		if verified != d.currentID {
			d.e.Log.Warn("current is not the last release that passed its health check, so the rollback goes to the one that did")
		}
		candidate = verified
	}
	if d.currentID == d.cfg.ReleaseID {
		d.resumed = true
		candidate = ""
		if verified != d.cfg.ReleaseID {
			candidate = verified
		}
	} else if candidate == d.cfg.ReleaseID {
		candidate = d.currentID
	}
	d.previous = candidate
	if candidate != "" && candidate == verified {
		d.known = candidate
	}
	return nil
}

func (d *run) lock() *Failure {
	dir := d.dhow + "/" + lockName
	for range 2 {
		err := d.r.Mkdir(dir, 0o700)
		if err == nil {
			d.locked = true
			if f := d.writeOwner(dir); f != nil {
				return f
			}
			d.keepLock()
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return fail(ExitFailed, CauseOther, "cannot take the lock of the target", err)
		}
		if !d.lockExpired(dir) {
			return fail(ExitTargetLocked, CauseOther, "another deployment holds the lock of the target", ErrTargetLocked)
		}
		d.e.Log.Warn("the lock of the target had expired, so this deployment takes it")
		if err := d.r.RemoveAll(dir); err != nil {
			return fail(ExitFailed, CauseOther, "cannot clear the expired lock", err)
		}
	}
	return fail(ExitTargetLocked, CauseOther, "another deployment holds the lock of the target", ErrTargetLocked)
}

func (d *run) writeOwner(dir string) *Failure {
	if err := d.putOwner(dir); err != nil {
		d.locked = false
		_ = d.r.RemoveAll(dir)
		return fail(ExitFailed, CauseOther, "cannot write the owner of the lock", err)
	}
	return nil
}

func (d *run) putOwner(dir string) error { return d.putOwnerOn(d.r, dir) }

func (d *run) putOwnerOn(r Remote, dir string) error {
	raw, err := json.Marshal(lockOwner{Release: d.cfg.ReleaseID, ExpiresAt: d.now().Add(LockTTL).UTC()})
	if err != nil {
		return err
	}
	return WriteFile(r, dir+"/"+ownerName, raw, 0o600)
}

func (d *run) ownsLockOn(r Remote, dir string) bool {
	raw, err := r.ReadFile(dir+"/"+ownerName, maxSmallFile)
	if err != nil {
		return false
	}
	var o lockOwner
	return json.Unmarshal(raw, &o) == nil && o.Release == d.cfg.ReleaseID
}

func (d *run) keepLock() {
	every := d.e.LockRenew
	if every <= 0 {
		every = LockTTL / 3
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	base := d.r
	limit := d.lockTimeout()
	d.stopRenew = func() {
		close(stop)
		<-done
	}
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		dir := d.dhow + "/" + lockName
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if !d.renewLock(base, dir, limit) {
					return
				}
			}
		}
	}()
}

func (d *run) lockTimeout() time.Duration {
	if d.e.LockTimeout > 0 {
		return d.e.LockTimeout
	}
	return DefaultLockTimeout
}

func (d *run) renewLock(base Remote, dir string, limit time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	r := boundRemote{Remote: base, ctx: ctx}
	if !d.ownsLockOn(r, dir) {
		d.e.Log.Warn("the lock of the target is no longer ours, or could not be read, so it is not renewed")
		return ctx.Err() != nil
	}
	if err := d.putOwnerOn(r, dir); err != nil {
		d.e.Log.Warn("the lock of the target could not be renewed")
	}
	return true
}

func (d *run) lockExpired(dir string) bool {
	raw, err := d.r.ReadFile(dir+"/"+ownerName, maxSmallFile)
	if err == nil {
		var o lockOwner
		return json.Unmarshal(raw, &o) == nil && d.now().After(o.ExpiresAt)
	}
	info, statErr := d.r.Lstat(dir)
	return statErr == nil && d.now().Sub(info.ModTime) > LockTTL
}

func (d *run) unlock() {
	if !d.locked {
		return
	}
	d.locked = false
	if d.stopRenew != nil {
		d.stopRenew()
		d.stopRenew = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.lockTimeout())
	defer cancel()
	r := boundRemote{Remote: d.r, ctx: ctx}
	dir := d.dhow + "/" + lockName
	if !d.ownsLockOn(r, dir) {
		d.e.Log.Warn("the lock of the target belongs to another deployment now, or could not be read, so it is left alone")
		return
	}
	if err := r.RemoveAll(dir); err != nil {
		d.e.Log.Warn("the lock of the target could not be removed")
	}
}

func (d *run) writeState(state, release, previous string) {
	raw, err := json.Marshal(stateFile{State: state, Release: release, Previous: previous, At: d.now().UTC()})
	if err == nil {
		err = WriteFile(d.r, d.dhow+"/"+stateName, raw, 0o644)
	}
	if err != nil {
		d.e.Log.Warn("the state file of the target could not be written")
	}
}
