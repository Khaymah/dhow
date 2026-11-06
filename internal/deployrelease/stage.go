package deployrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/Khaymah/dhow/internal/archive"
)

func (d *run) stageRelease(ctx context.Context) *Failure {
	if d.resumed {
		d.e.Report.Emit(Event{Event: EventUploaded, HostID: d.cfg.HostID, ReleaseID: d.cfg.ReleaseID})
		return nil
	}
	if f := d.clearLeftovers(); f != nil {
		return f
	}
	if f := d.upload(ctx); f != nil {
		return f
	}
	if f := d.linkShared(); f != nil {
		return f
	}
	if f := d.verifyUpload(ctx); f != nil {
		return f
	}
	if err := d.r.Rename(d.incoming, d.releaseDir); err != nil {
		return fail(ExitFailed, CauseUpload, "cannot put the release in place", err)
	}
	d.e.Report.Emit(Event{Event: EventUploaded, HostID: d.cfg.HostID, ReleaseID: d.cfg.ReleaseID})
	if d.cfg.PreSwitch != "" {
		if f := d.hook(ctx, "pre-switch", d.cfg.PreSwitch, d.releaseDir, d.cfg.ReleaseID, d.previous); f != nil {
			return f
		}
	}
	return nil
}

func (d *run) clearLeftovers() *Failure {
	list, err := d.r.ReadDir(d.releases)
	if err != nil {
		return fail(ExitFailed, CauseUpload, "cannot list the releases", err)
	}
	for _, f := range list {
		stale := strings.HasPrefix(f.Name, incomingPref)
		if stale || f.Name == d.cfg.ReleaseID {
			if err := d.r.RemoveAll(d.releases + "/" + f.Name); err != nil {
				return fail(ExitFailed, CauseUpload, "cannot clear a leftover of an earlier deployment", err)
			}
		}
	}
	return nil
}

type uploader struct {
	d        *run
	ctx      context.Context
	manifest []string
	made     map[string]bool
}

func (d *run) upload(ctx context.Context) *Failure {
	if err := d.r.Mkdir(d.incoming, 0o755); err != nil {
		return fail(ExitFailed, CauseUpload, "cannot create the upload directory", err)
	}
	f, err := os.Open(d.cfg.ArchivePath)
	if err != nil {
		return fail(ExitInput, CauseOther, "the artifact cannot be opened", err)
	}
	defer func() { _ = f.Close() }()
	u := &uploader{d: d, ctx: ctx, made: map[string]bool{}}
	hash := sha256.New()
	if _, err := archive.Walk(io.TeeReader(f, hash), d.cfg.Limits, u.visit); err != nil {
		if ctx.Err() != nil {
			return fail(ExitFailed, CauseCanceled, "the deployment was canceled", ctx.Err())
		}
		return archiveFailure(err)
	}
	if _, err := io.Copy(hash, f); err != nil {
		return fail(ExitInput, CauseOther, "the artifact cannot be read", err)
	}
	if d.cfg.ArchiveDigest != "" && hex.EncodeToString(hash.Sum(nil)) != d.cfg.ArchiveDigest {
		return fail(ExitUnsafeArtifact, CauseOther, "the artifact changed while it was uploaded", ErrDigest)
	}
	d.hasFiles = len(u.manifest) > 0
	if !d.hasFiles {
		return nil
	}
	return u.writeManifest()
}

func (u *uploader) visit(e archive.Entry, body io.Reader) error {
	if err := u.ctx.Err(); err != nil {
		return err
	}
	name := u.d.incoming + "/" + e.Name
	r := u.d.r
	if parent := path.Dir(name); e.Kind != archive.KindDir && !u.made[parent] {
		if err := r.MkdirAll(parent, 0o755); err != nil {
			return err
		}
		u.made[parent] = true
	}
	switch e.Kind {
	case archive.KindDir:
		return r.MkdirAll(name, e.Mode)
	case archive.KindSymlink:
		return r.Symlink(e.Link, name)
	default:
		return u.file(name, e, body)
	}
}

func (u *uploader) file(name string, e archive.Entry, body io.Reader) error {
	w, err := u.d.r.Create(name, e.Mode)
	if err != nil {
		return err
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, sum), body)
	closeErr := w.Close()
	switch {
	case err != nil:
		return err
	case closeErr != nil:
		return closeErr
	case n != e.Size:
		return fmt.Errorf("%w: %s is cut short", archive.ErrUnsafe, e.Name)
	}
	u.manifest = append(u.manifest, hex.EncodeToString(sum.Sum(nil))+"  "+name)
	return nil
}

func (u *uploader) writeManifest() *Failure {
	w, err := u.d.r.Create(u.d.manifest, 0o600)
	if err != nil {
		return fail(ExitFailed, CauseUpload, "cannot write the checksum list", err)
	}
	_, err = io.WriteString(w, strings.Join(u.manifest, "\n")+"\n")
	closeErr := w.Close()
	if err != nil || closeErr != nil {
		return fail(ExitFailed, CauseUpload, "cannot write the checksum list", errors.Join(err, closeErr))
	}
	return nil
}

func (d *run) verifyUpload(ctx context.Context) *Failure {
	if !d.hasFiles {
		return nil
	}
	defer func() { _ = d.r.Remove(d.manifest) }()
	res, err := d.r.Exec(ctx, []string{"sha256sum", "-c", "--", d.manifest})
	switch {
	case err != nil:
		return fail(ExitFailed, CauseUpload, "cannot check the uploaded files", err)
	case res.ExitCode != 0:
		d.e.Log.Warn("the checksums of the uploaded files do not match", "output", tail(res.Output))
		return fail(ExitFailed, CauseUpload, "the uploaded files do not match their checksums", nil)
	}
	return nil
}

func (d *run) linkShared() *Failure {
	for _, n := range d.cfg.Shared {
		link := d.incoming + "/" + n
		if _, err := d.r.Lstat(link); err == nil {
			return fail(ExitFailed, CauseUpload, "the release has a file with the name of a shared path", ErrSharedConflict)
		} else if !isNotExist(err) {
			return fail(ExitFailed, CauseUpload, "cannot look at a shared path", err)
		}
		if err := d.r.Symlink("../../"+sharedName+"/"+n, link); err != nil {
			return fail(ExitFailed, CauseUpload, "cannot link a shared path", err)
		}
	}
	return nil
}

func tail(b []byte) string {
	const limit = 4 << 10
	if len(b) > limit {
		b = b[len(b)-limit:]
	}
	return string(b)
}
