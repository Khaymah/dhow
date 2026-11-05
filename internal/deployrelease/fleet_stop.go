package deployrelease

import (
	"context"
	"errors"
	"sort"
	"sync"
)

const (
	messageRolledBack    = "rolled back to the previous release"
	messageRollbackFails = "the rollback failed"
	messageNoPrevious    = "the first release failed and there is no earlier release to go back to"
	messageNoTime        = "the rollback ran out of time"
)

func (g *group) assign(cause *Failure, culprit *member) {
	for _, m := range g.members {
		switch {
		case m == culprit:
			m.err = cause
		case culprit == nil && !m.staging && m.d.r == nil:
			m.err = cause
		}
	}
}

func (g *group) switched() []*member {
	var out []*member
	for _, m := range g.members {
		if m.d.switched {
			out = append(out, m)
		}
	}
	return out
}

func (g *group) settleUnswitched() {
	for _, m := range g.members {
		if m.d.switched {
			continue
		}
		if m.staging {
			m.d.cleanStage()
		}
		if m.err != nil && !m.collateral {
			g.rep.Emit(Event{Event: EventFailed, HostID: m.d.cfg.HostID, ReleaseID: g.releaseID(), Reason: m.err.Reason, Message: m.err.Text})
			continue
		}
		g.rep.Emit(Event{Event: EventSkipped, HostID: m.d.cfg.HostID, ReleaseID: g.releaseID()})
	}
}

func (g *group) stop(ctx context.Context, cause *Failure, culprit *member) *Failure {
	g.assign(cause, culprit)
	g.emitStarted()
	switched := g.switched()
	g.settleUnswitched()
	if len(switched) > 0 {
		return g.rollBackGroup(ctx, cause, switched)
	}
	g.rep.Emit(Event{Event: EventFailed, ReleaseID: g.releaseID(), Reason: cause.Reason, Message: cause.Text})
	g.log.Warn("the deployment failed", "reason", cause.Reason, "error", cause.Error())
	return cause
}

func (g *group) rollBackGroup(ctx context.Context, cause *Failure, switched []*member) *Failure {
	g.log.Warn("the new release failed after the switch", "reason", cause.Reason, "error", cause.Error())
	var back, stuck []*member
	for _, m := range switched {
		if m.d.previous == "" {
			stuck = append(stuck, m)
		} else {
			back = append(back, m)
		}
	}
	if len(back) == 0 {
		return g.keepFirstRelease(cause, stuck)
	}
	for _, m := range stuck {
		g.rep.Emit(Event{Event: EventRollbackFailed, HostID: m.d.cfg.HostID, ReleaseID: g.releaseID(), Reason: cause.Reason, Message: messageNoPrevious})
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), RollbackBudget)
	defer cancel()
	errs := g.revert(rctx, back, cause)
	if len(errs) == 0 && len(stuck) == 0 {
		g.rep.Emit(Event{Event: EventRolledBack, ReleaseID: g.releaseID(), PreviousReleaseID: g.previous(), Reason: cause.Reason, Message: messageRolledBack})
		return fail(ExitRolledBack, cause.Reason, messageRolledBack, cause)
	}
	g.rep.Emit(Event{Event: EventRollbackFailed, ReleaseID: g.releaseID(), Reason: cause.Reason, Message: messageRollbackFails})
	return fail(ExitRollbackFailed, cause.Reason, messageRollbackFails, errors.Join(append([]error{cause}, errs...)...))
}

func (g *group) revert(ctx context.Context, back []*member, cause *Failure) []error {
	byBatch := map[int][]*member{}
	var order []int
	for _, m := range back {
		if _, ok := byBatch[m.d.batch]; !ok {
			order = append(order, m.d.batch)
		}
		byBatch[m.d.batch] = append(byBatch[m.d.batch], m)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(order)))
	var errs []error
	for _, b := range order {
		ms := byBatch[b]
		results := make([]error, len(ms))
		if err := ctx.Err(); err != nil {
			for i := range results {
				results[i] = errors.New(messageNoTime)
			}
		} else {
			var wg sync.WaitGroup
			for i, m := range ms {
				wg.Go(func() { results[i] = m.d.rollBack(ctx) })
			}
			wg.Wait()
		}
		for i, m := range ms {
			g.reportRollback(m, cause, results[i])
			if results[i] != nil {
				errs = append(errs, results[i])
			}
		}
	}
	return errs
}

func (g *group) reportRollback(m *member, cause *Failure, err error) {
	if err != nil {
		g.log.Warn("the rollback of a server failed", "host", m.d.cfg.HostName)
		g.rep.Emit(Event{Event: EventRollbackFailed, HostID: m.d.cfg.HostID, ReleaseID: g.releaseID(), Reason: cause.Reason, Message: messageRollbackFails})
		return
	}
	g.rep.Emit(Event{Event: EventRolledBack, HostID: m.d.cfg.HostID, ReleaseID: g.releaseID(), PreviousReleaseID: m.d.known, Reason: cause.Reason, Message: messageRolledBack})
}

func (g *group) keepFirstRelease(cause *Failure, stuck []*member) *Failure {
	text := "the first release failed and there is no earlier release to go back to, so current is left as it is"
	if g.f.Config.FirstFailure == FirstFailureFail {
		text = messageNoPrevious
	}
	for _, m := range stuck {
		g.rep.Emit(Event{Event: EventFailed, HostID: m.d.cfg.HostID, ReleaseID: g.releaseID(), Reason: cause.Reason, Message: text})
	}
	g.rep.Emit(Event{Event: EventFailed, ReleaseID: g.releaseID(), Reason: cause.Reason, Message: text})
	exit := ExitFailed
	if cause.Reason == CauseHealth {
		exit = ExitHealthFailed
	}
	return fail(exit, cause.Reason, text, cause)
}
