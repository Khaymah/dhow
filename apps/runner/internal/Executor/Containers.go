package executor

import (
	"context"
	"errors"
	"strconv"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	"github.com/Khaymah/dhow/internal/gitsource"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type segment struct {
	kind   sandbox.Kind
	image  string
	steps  []int32
	egress []string
}

func segments(spec *runnerv1.JobSpec, checkoutImage string) []segment {
	var out []segment
	if co := spec.GetCheckout(); co != nil {
		out = append(out, segment{kind: sandbox.KindCheckout, image: checkoutImage, egress: checkoutEgress(co)})
	}
	for i, s := range spec.GetSteps() {
		idx := int32(i)
		if p := s.GetPlugin(); p != nil {
			out = append(out, segment{kind: sandbox.KindPlugin, image: p.GetImage(), steps: []int32{idx}, egress: spec.GetNetwork().GetEgress()})
			continue
		}
		if n := len(out); n > 0 && out[n-1].kind == sandbox.KindJob {
			out[n-1].steps = append(out[n-1].steps, idx)
			continue
		}
		out = append(out, segment{kind: sandbox.KindJob, image: spec.GetImage(), steps: []int32{idx}, egress: spec.GetNetwork().GetEgress()})
	}
	return out
}

func checkoutEgress(co *runnerv1.Checkout) []string {
	u, err := gitsource.Parse(co.GetRepositoryUrl())
	if err != nil {
		return nil
	}
	return []string{u.Host + ":" + strconv.Itoa(u.EffectivePort())}
}

func (a *attempt) openWorkspace(ctx context.Context) (result, bool) {
	spec := a.grant.GetSpec()
	ws, err := a.e.o.Driver.Workspace(ctx, sandbox.WorkspaceSpec{
		RunnerID: a.e.o.Identity.RunnerID, AttemptID: a.grant.GetAttemptId(), Dir: a.dir,
		DiskMode: a.e.o.Config.DiskMode, DiskBytes: spec.GetResources().GetDiskBytes(),
	})
	if err != nil {
		a.log.Error("could not create the workspace", "error", a.masked(err))
		a.pipe.Systemf(nil, "dhow: SANDBOX_FAILED: the workspace could not be created")
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	a.mu.Lock()
	a.workspace = ws
	a.mu.Unlock()
	return result{}, true
}

func (a *attempt) runSegments(ctx context.Context, segs []segment) result {
	spec := a.grant.GetSpec()
	jobCtx, cancel := jobContext(ctx, spec)
	defer cancel()
	for i, seg := range segs {
		if i > 0 {
			a.retire(ctx)
			if a.hadSurvivors() {
				a.pipe.Systemf(nil, "dhow: SANDBOX_FAILED: processes of the previous sandbox survived, so the next one does not start")
				return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0)
			}
			if r, stop := a.interrupted(jobCtx); stop {
				return r
			}
			if r, ok := a.prepare(ctx, i, seg); !ok {
				return r
			}
			if err := a.currentBox().Start(ctx); err != nil {
				a.log.Error("could not start the sandbox", "error", a.masked(err))
				return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0)
			}
		}
		if r, ok := a.runSegment(ctx, jobCtx, seg); !ok {
			return r
		}
	}
	if a.fenced() {
		return a.lostResult()
	}
	return succeeded()
}

func (a *attempt) runSegment(ctx, jobCtx context.Context, seg segment) (result, bool) {
	switch seg.kind {
	case sandbox.KindCheckout:
		return a.runCheckout(ctx, jobCtx)
	case sandbox.KindPlugin:
		return a.runPlugin(ctx, jobCtx, seg.steps[0])
	default:
		for _, idx := range seg.steps {
			if r, stop := a.interrupted(jobCtx); stop {
				return r, false
			}
			step := a.grant.GetSpec().GetSteps()[idx]
			if r, ok := a.beginStep(ctx, idx, step.GetMutating()); !ok {
				return r, false
			}
			if r, ok := a.runStep(ctx, jobCtx, idx, step, execPlan{}); !ok {
				return r, false
			}
		}
		return result{}, true
	}
}

func (a *attempt) retire(ctx context.Context) {
	a.destroy(ctx)
	a.mu.Lock()
	a.box = nil
	a.mu.Unlock()
}

func (a *attempt) hadSurvivors() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.survivors
}

func (a *attempt) closeWorkspace(ctx context.Context) {
	a.mu.Lock()
	ws := a.workspace
	a.workspace = nil
	a.mu.Unlock()
	if ws == nil {
		return
	}
	dctx, cancel := context.WithTimeout(ctx, teardownTimeout)
	defer cancel()
	if err := ws.Destroy(dctx); err != nil {
		a.log.Error("could not remove the workspace", "error", a.masked(err))
		a.noteSurvivors(errors.Join(sandbox.ErrSurvivors, err))
	}
}
