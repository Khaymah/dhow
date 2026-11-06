package deployrelease

import (
	"context"
	"errors"
)

type Release struct {
	DeployPath string
	Path       string
	ID         string
	Previous   string
}

type Activator interface {
	Prepare(ctx context.Context, r Remote, rel Release) error
	Activate(ctx context.Context, r Remote, rel Release) error
}

type Inspector interface {
	Inspect(ctx context.Context, cfg Config) error
}

func (g *group) inspect(ctx context.Context) *Failure {
	in, ok := g.f.Config.Activator.(Inspector)
	if !ok {
		return nil
	}
	if err := in.Inspect(ctx, g.f.Config); err != nil {
		var f *Failure
		if errors.As(err, &f) {
			return f
		}
		return fail(ExitFailed, CauseOther, "the release was refused before any server was touched", err)
	}
	return nil
}

func (d *run) release(path, id, previous string) Release {
	return Release{DeployPath: d.base, Path: path, ID: id, Previous: previous}
}

func (d *run) activatorFailure(ctx context.Context, err error, reason, text string) *Failure {
	var f *Failure
	switch {
	case errors.As(err, &f):
		return f
	case ctx.Err() != nil:
		return fail(ExitFailed, CauseCanceled, "the deployment was canceled", ctx.Err())
	}
	return fail(ExitFailed, reason, text, err)
}

func (d *run) prepareRelease(ctx context.Context) *Failure {
	a := d.cfg.Activator
	if a == nil {
		return nil
	}
	if err := a.Prepare(ctx, d.r, d.release(d.releaseDir, d.cfg.ReleaseID, d.previous)); err != nil {
		return d.activatorFailure(ctx, err, CauseHook, "the release could not be prepared on the server")
	}
	return nil
}

func (d *run) activateRelease(ctx context.Context, path, id, previous string) *Failure {
	a := d.cfg.Activator
	if a == nil {
		return nil
	}
	if err := a.Activate(ctx, d.r, d.release(path, id, previous)); err != nil {
		return d.activatorFailure(ctx, err, CauseReload, "the release did not start on the server")
	}
	return nil
}
