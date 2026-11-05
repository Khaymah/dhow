package deployrelease

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

const (
	historyName     = "history"
	maxHistoryBytes = 64 << 10
	maxHistoryLines = 500
)

func (d *run) switchOver(batch int) *Failure {
	if !d.resumed {
		d.writeState("switching", d.cfg.ReleaseID, d.previous)
		if err := d.switchTo(d.cfg.ReleaseID); err != nil {
			return fail(ExitFailed, CauseSwitch, "cannot switch to the new release", err)
		}
	}
	d.switched = true
	d.batch = batch
	d.e.Report.Emit(Event{Event: EventSwitched, HostID: d.cfg.HostID, ReleaseID: d.cfg.ReleaseID, PreviousReleaseID: d.known, Batch: batch})
	return nil
}

func (d *run) switchTo(id string) error {
	tmp := d.base + "/" + currentName + ".tmp-" + id
	if _, err := d.r.Lstat(tmp); err == nil {
		if err := d.r.Remove(tmp); err != nil {
			return err
		}
	}
	if err := d.r.Symlink(releasesName+"/"+id, tmp); err != nil {
		return err
	}
	if err := d.r.Rename(tmp, d.current); err != nil {
		_ = d.r.Remove(tmp)
		return err
	}
	return nil
}

func (d *run) activate(ctx context.Context) *Failure {
	if d.cfg.ReloadCommand != "" {
		if f := d.hook(ctx, "reload", d.cfg.ReloadCommand, d.releaseDir, d.cfg.ReleaseID, d.previous); f != nil {
			return f
		}
	}
	if d.cfg.PostSwitch != "" {
		return d.hook(ctx, "post-switch", d.cfg.PostSwitch, d.releaseDir, d.cfg.ReleaseID, d.previous)
	}
	return nil
}

func (d *run) checkHealth(ctx context.Context) *Failure {
	if !d.cfg.Health.Enabled() {
		return nil
	}
	res, err := CheckHealth(ctx, d.e.Doer, d.cfg.HostName, d.cfg.Health, d.e.Clock)
	return d.healthVerdict(ctx, res, err)
}

func (d *run) healthVerdict(ctx context.Context, res HealthResult, err error) *Failure {
	if err != nil && ctx.Err() != nil {
		return fail(ExitFailed, CauseCanceled, "the deployment was canceled", err)
	}
	d.e.Report.Emit(Event{Event: EventHealth, HostID: d.cfg.HostID, ReleaseID: d.cfg.ReleaseID, Health: res.Report()})
	if err != nil {
		return fail(ExitHealthFailed, CauseHealth, "the health check could not run", err)
	}
	if !res.Passed {
		return fail(ExitHealthFailed, CauseHealth, "the health check failed", nil)
	}
	return nil
}

func (d *run) markVerified() error {
	if err := WriteFile(d.r, d.dhow+"/"+verifiedName, []byte(d.cfg.ReleaseID+"\n"), 0o644); err != nil {
		return err
	}
	d.marked = true
	return nil
}

func (d *run) completeRelease() {
	id := d.cfg.ReleaseID
	d.appendHistory(id)
	d.writeState("idle", id, d.previous)
	d.e.Report.Emit(Event{Event: EventVerified, HostID: d.cfg.HostID, ReleaseID: id})
}

func (d *run) pruneOld() {
	removed := d.prune()
	d.e.Report.Emit(Event{Event: EventPruned, HostID: d.cfg.HostID, ReleaseID: d.cfg.ReleaseID, Message: fmt.Sprintf("%d old releases removed", removed)})
}

func (d *run) rollBack(ctx context.Context) error {
	if !d.releaseExists(d.previous) {
		return errors.New("the previous release is no longer on the server")
	}
	d.writeState("rolling_back", d.previous, d.cfg.ReleaseID)
	if err := d.switchTo(d.previous); err != nil {
		return err
	}
	if d.cfg.ReloadCommand != "" {
		if f := d.hook(ctx, "reload", d.cfg.ReloadCommand, d.releases+"/"+d.previous, d.previous, d.cfg.ReleaseID); f != nil {
			return f
		}
	}
	if d.marked {
		if err := WriteFile(d.r, d.dhow+"/"+verifiedName, []byte(d.previous+"\n"), 0o644); err != nil {
			return err
		}
		d.marked = false
	}
	d.writeState("idle", d.previous, d.cfg.ReleaseID)
	return nil
}

func (d *run) history() []string {
	raw, err := d.r.ReadFile(d.dhow+"/"+historyName, maxHistoryBytes)
	if err != nil {
		return nil
	}
	var ids []string
	for _, line := range strings.Split(string(raw), "\n") {
		if id := strings.TrimSpace(line); validID(id) {
			ids = append(ids, id)
		}
	}
	return ids
}

func (d *run) appendHistory(id string) {
	ids := slices.DeleteFunc(d.history(), func(v string) bool { return v == id })
	ids = append(ids, id)
	if len(ids) > maxHistoryLines {
		ids = ids[len(ids)-maxHistoryLines:]
	}
	if err := WriteFile(d.r, d.dhow+"/"+historyName, []byte(strings.Join(ids, "\n")+"\n"), 0o644); err != nil {
		d.e.Log.Warn("the release history could not be written")
	}
}

func (d *run) prune() int {
	list, err := d.r.ReadDir(d.releases)
	if err != nil {
		d.e.Log.Warn("the releases could not be listed, so none is pruned")
		return 0
	}
	rank := map[string]int{}
	for i, id := range d.history() {
		rank[id] = i + 1
	}
	protected := map[string]bool{d.cfg.ReleaseID: true, d.previous: true, d.currentID: true}
	var old []FileInfo
	kept := 0
	for _, f := range list {
		if !f.IsDir() || !validID(f.Name) {
			continue
		}
		if protected[f.Name] {
			kept++
			continue
		}
		old = append(old, f)
	}
	sort.Slice(old, func(i, j int) bool {
		if ri, rj := rank[old[i].Name], rank[old[j].Name]; ri != rj {
			return ri > rj
		}
		if !old[i].ModTime.Equal(old[j].ModTime) {
			return old[i].ModTime.After(old[j].ModTime)
		}
		return old[i].Name > old[j].Name
	})
	removed := 0
	for i, f := range old {
		if kept+i < d.cfg.Keep {
			continue
		}
		if err := d.r.RemoveAll(d.releases + "/" + f.Name); err != nil {
			d.e.Log.Warn("a release could not be removed", "release", f.Name)
			continue
		}
		removed++
	}
	return removed
}
