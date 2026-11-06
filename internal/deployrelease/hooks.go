package deployrelease

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const hookScript = `cd "$DHOW_RELEASE_PATH" && eval "$1"`

const killGrace = 5 * time.Second

func HookArgv(hook, base, releasePath, releaseID, previous string, limit time.Duration) []string {
	argv := []string{
		"env",
		"DHOW_DEPLOY_PATH=" + base,
		"DHOW_RELEASE_PATH=" + releasePath,
		"DHOW_RELEASE_ID=" + releaseID,
		"DHOW_PREVIOUS_RELEASE=" + previous,
	}
	if limit > 0 {
		argv = append(argv, "timeout", "-s", "KILL", strconv.Itoa(int(limit.Seconds())))
	}
	return append(argv, "sh", "-c", hookScript, "dhow-hook", hook)
}

func (d *run) remoteTimeout(ctx context.Context) bool {
	if d.timeoutKnown {
		return d.hasTimeout
	}
	d.timeoutKnown = true
	res, err := d.r.Exec(ctx, []string{"command", "-v", "timeout"})
	d.hasTimeout = err == nil && res.ExitCode == 0
	if !d.hasTimeout {
		d.e.Log.Warn("the server has no timeout command, so a hook is limited from this side only")
	}
	return d.hasTimeout
}

func (d *run) hook(ctx context.Context, name, command, releasePath, releaseID, previous string) *Failure {
	reason := CauseHook
	if name == "reload" {
		reason = CauseReload
	}
	var remoteLimit time.Duration
	if d.remoteTimeout(ctx) {
		remoteLimit = d.cfg.HookTimeout
	}
	hctx, cancel := context.WithTimeout(ctx, d.cfg.HookTimeout+killGrace)
	defer cancel()
	start := time.Now()
	res, err := d.r.Exec(hctx, HookArgv(command, d.base, releasePath, releaseID, previous, remoteLimit))
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
