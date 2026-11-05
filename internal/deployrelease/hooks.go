package deployrelease

import (
	"context"
	"fmt"
	"time"
)

const hookScript = `cd "$DHOW_RELEASE_PATH" && eval "$1"`

func HookArgv(hook, base, releasePath, releaseID, previous string) []string {
	return []string{
		"env",
		"DHOW_DEPLOY_PATH=" + base,
		"DHOW_RELEASE_PATH=" + releasePath,
		"DHOW_RELEASE_ID=" + releaseID,
		"DHOW_PREVIOUS_RELEASE=" + previous,
		"sh", "-c", hookScript, "dhow-hook", hook,
	}
}

func (d *run) hook(ctx context.Context, name, command, releasePath, releaseID, previous string) *Failure {
	reason := CauseHook
	if name == "reload" {
		reason = CauseReload
	}
	hctx, cancel := context.WithTimeout(ctx, d.cfg.HookTimeout)
	defer cancel()
	start := time.Now()
	res, err := d.r.Exec(hctx, HookArgv(command, d.base, releasePath, releaseID, previous))
	if err != nil {
		if ctx.Err() != nil {
			return fail(ExitFailed, CauseCanceled, "the deployment was canceled", ctx.Err())
		}
		return fail(ExitFailed, reason, fmt.Sprintf("the %s command could not finish", name), err)
	}
	d.e.Log.Info("a deployment command finished", "command", name, "exit", res.ExitCode, "seconds", int(time.Since(start).Seconds()), "output", tail(res.Output))
	if res.ExitCode != 0 {
		return fail(ExitFailed, reason, fmt.Sprintf("the %s command failed", name), nil)
	}
	return nil
}
