package podman

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

var ErrIDRange = errors.New("podman unshare did not report the runner's sub-id range")

type idSlots struct {
	inner  int64
	host   int64
	count  int64
	inUse  map[int64]string
	byName map[string]int64
}

func parseSubidRange(raw string) (idRange, error) {
	ranges, err := parseIDMap(raw)
	if err != nil {
		return idRange{}, err
	}
	for _, r := range ranges {
		if r.inside == 1 && r.count >= userNSSize {
			return r, nil
		}
	}
	return idRange{}, ErrIDRange
}

func newSlots(r idRange) *idSlots {
	return &idSlots{inner: r.inside, host: r.outside, count: r.count / userNSSize, inUse: map[int64]string{}, byName: map[string]int64{}}
}

func (s *idSlots) take(owner string) (int64, error) {
	if k, ok := s.byName[owner]; ok {
		return k, nil
	}
	for k := range s.count {
		if _, busy := s.inUse[k]; !busy {
			s.inUse[k] = owner
			s.byName[owner] = k
			return k, nil
		}
	}
	return 0, sandbox.ErrNoIDRange
}

func (s *idSlots) release(owner string) {
	if k, ok := s.byName[owner]; ok {
		delete(s.inUse, k)
		delete(s.byName, owner)
	}
}

func (s *idSlots) innerStart(k int64) int64 { return s.inner + k*userNSSize }

func (s *idSlots) hostStart(k int64) int64 { return s.host + k*userNSSize }

func (d *Driver) detectSlots(ctx context.Context) error {
	out, err := d.runner().run(ctx, "unshare", "cat", "/proc/self/uid_map")
	if err != nil {
		return fmt.Errorf("read the runner's sub-id range: %w", err)
	}
	r, err := parseSubidRange(string(out))
	if err != nil {
		return err
	}
	d.mu.Lock()
	if d.slots == nil {
		d.slots = newSlots(r)
	}
	d.mu.Unlock()
	return nil
}

type workspace struct {
	d       *Driver
	owner   string
	dir     string
	slot    int64
	mounted bool
}

func (d *Driver) Workspace(ctx context.Context, spec sandbox.WorkspaceSpec) (sandbox.Workspace, error) {
	d.mu.Lock()
	missing := d.slots == nil
	d.mu.Unlock()
	if missing {
		if err := d.detectSlots(ctx); err != nil {
			return nil, fmt.Errorf("%w: %w", sandbox.ErrSandbox, err)
		}
	}
	d.mu.Lock()
	slot, err := d.slots.take(spec.AttemptID)
	inner := d.slots.innerStart(slot)
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	w := &workspace{d: d, owner: spec.AttemptID, dir: filepath.Join(spec.Dir, workspaceDirName), slot: slot}
	if err := w.create(ctx, spec, inner); err != nil {
		return nil, errors.Join(fmt.Errorf("%w: %w", sandbox.ErrSandbox, err), w.Destroy(ctx))
	}
	return w, nil
}

func (w *workspace) create(ctx context.Context, spec sandbox.WorkspaceSpec, inner int64) error {
	if err := config.EnsurePrivateDir(w.dir); err != nil {
		return err
	}
	id := strconv.FormatInt(inner, 10)
	r := w.d.runner()
	if spec.DiskMode == config.DiskModeTmpfs {
		opts := fmt.Sprintf("size=%d,nosuid,nodev,mode=0755,uid=%s,gid=%s", spec.DiskBytes, id, id)
		if _, err := r.run(ctx, "unshare", "mount", "-t", "tmpfs", "-o", opts, "tmpfs", w.dir); err != nil {
			return fmt.Errorf("mount the workspace tmpfs: %w", err)
		}
		w.mounted = true
		return nil
	}
	if _, err := r.run(ctx, "unshare", "chown", id+":"+id, "--", w.dir); err != nil {
		return fmt.Errorf("give the workspace to the attempt's user namespace: %w", err)
	}
	return nil
}

var ErrWorkspaceInUse = errors.New("a sandbox of the attempt is still alive, so its workspace and id range stay")

func (d *Driver) attemptAlive(owner string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	prefix := "dhow-" + owner
	for name := range d.live {
		if name == prefix || strings.HasPrefix(name, prefix+"-") {
			return true
		}
	}
	return false
}

func (w *workspace) Destroy(ctx context.Context) error {
	if w.d.attemptAlive(w.owner) {
		return ErrWorkspaceInUse
	}
	var errs []error
	if w.mounted {
		if _, err := w.d.runner().run(ctx, "unshare", "umount", "--", w.dir); err != nil {
			errs = append(errs, fmt.Errorf("unmount the workspace: %w", err))
		} else {
			w.mounted = false
		}
	}
	if len(errs) == 0 {
		if err := w.d.removeOwned(ctx, w.dir); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		w.d.mu.Lock()
		w.d.slots.release(w.owner)
		w.d.mu.Unlock()
	}
	return errors.Join(errs...)
}

func (d *Driver) slotOf(ws sandbox.Workspace) (*workspace, bool) {
	w, ok := ws.(*workspace)
	return w, ok && w.d == d
}

func (d *Driver) unmountLeftovers(ctx context.Context) {
	entries, err := os.ReadDir(d.attemptsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		dir := filepath.Join(d.attemptsDir, e.Name(), workspaceDirName)
		if _, err := os.Lstat(dir); err != nil {
			continue
		}
		if _, err := d.runner().run(ctx, "unshare", "umount", "--lazy", "--", dir); err != nil {
			d.log.Debug("no workspace mount to remove", "error", err.Error())
		}
	}
}
