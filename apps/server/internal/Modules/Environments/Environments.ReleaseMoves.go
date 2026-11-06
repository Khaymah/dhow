package environments

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"connectrpc.com/connect"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func (sc *reportScope) start(ctx context.Context, ev *runnerv1.DeployEvent) error {
	targetID := sc.tid
	if err := sc.checkPrevious(ctx, ev.GetPreviousReleaseId()); err != nil {
		return err
	}
	existing, err := sc.q.GetReleaseForUpdate(ctx, sqlc.GetReleaseForUpdateParams{
		TargetID: targetID, ReleaseID: sc.target.GetReleaseId(), OrganizationID: sc.lease.OrganizationId,
	})
	switch {
	case err == nil:
		return sc.adopt(ctx, existing, ev)
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	if err := sc.retireStaleRunning(ctx, targetID); err != nil {
		return err
	}
	kind, restored, err := sc.kindAndRestored(ctx, ev)
	if err != nil {
		return err
	}
	rel, err := sc.q.InsertRelease(ctx, sqlc.InsertReleaseParams{
		OrganizationID: sc.lease.OrganizationId, DeploymentID: *sc.lease.DeploymentId, RunJobID: *sc.lease.RunJobId, JobID: sc.lease.JobId,
		LeaseID: sc.lease.LeaseId, TargetID: targetID, StepIndex: sc.step, Kind: kind, ReleaseID: sc.target.GetReleaseId(),
		PreviousReleaseID: text(ev.GetPreviousReleaseId()), ArtifactDigest: sc.digest,
		BatchSize: max(sc.target.GetSettings().GetBatchSize(), 1), RestoredReleaseID: text(restored), HealthInputs: sc.healthInputs(), Now: sc.now,
	})
	if err != nil {
		return err
	}
	sc.rel = &rel
	sc.hosts = make(map[uuid.UUID]*sqlc.DeploymentReleaseHost, len(sc.target.GetHosts()))
	for i, hid := range sc.hids {
		if err := sc.q.InsertReleaseHost(ctx, sqlc.InsertReleaseHostParams{
			OrganizationID: rel.OrganizationId, ReleaseRowID: rel.Id, HostID: hid, ReleaseID: rel.ReleaseId, Position: int32(i + 1), Now: sc.now,
		}); err != nil {
			return err
		}
	}
	rows, err := sc.q.ListReleaseHostsForUpdate(ctx, sqlc.ListReleaseHostsForUpdateParams{ReleaseRowID: rel.Id, OrganizationID: rel.OrganizationId})
	if err != nil {
		return err
	}
	for i := range rows {
		sc.hosts[rows[i].HostId] = &rows[i]
	}
	if err := sc.q.MarkReleaseDeploymentRunning(ctx, sqlc.MarkReleaseDeploymentRunningParams{ID: rel.DeploymentId, OrganizationID: rel.OrganizationId, Now: sc.now}); err != nil {
		return err
	}
	return sc.audit(ctx, eventbus.DeploymentReleaseStarted, map[string]string{"Kind": kind})
}

func (sc *reportScope) adopt(ctx context.Context, existing sqlc.DeploymentRelease, ev *runnerv1.DeployEvent) error {
	if existing.LeaseId == sc.lease.LeaseId && existing.State == groupRunning {
		return sc.load(ctx)
	}
	if existing.JobId != sc.lease.JobId || existing.StepIndex != sc.step || existing.DeploymentId != *sc.lease.DeploymentId {
		return reject(connect.CodeFailedPrecondition, CodeReleaseConflict)
	}
	switch existing.State {
	case groupRunning:
		if err := sc.retireStaleRunning(ctx, sc.tid); err != nil {
			return err
		}
	case groupUnknown:
		if err := sc.retireStaleRunning(ctx, sc.tid); err != nil {
			return err
		}
	default:
		return reject(connect.CodeFailedPrecondition, CodeReleaseConflict)
	}
	if err := sc.q.AdoptRelease(ctx, sqlc.AdoptReleaseParams{
		LeaseID: sc.lease.LeaseId, DeploymentID: *sc.lease.DeploymentId, PreviousReleaseID: text(ev.GetPreviousReleaseId()),
		ArtifactDigest: sc.digest, Now: sc.now, ID: existing.Id, OrganizationID: existing.OrganizationId,
	}); err != nil {
		return err
	}
	if err := sc.q.ResetReleaseHosts(ctx, sqlc.ResetReleaseHostsParams{Now: sc.now, ReleaseRowID: existing.Id, OrganizationID: existing.OrganizationId}); err != nil {
		return err
	}
	if err := sc.q.MarkReleaseDeploymentRunning(ctx, sqlc.MarkReleaseDeploymentRunningParams{ID: *sc.lease.DeploymentId, OrganizationID: existing.OrganizationId, Now: sc.now}); err != nil {
		return err
	}
	if err := sc.load(ctx); err != nil {
		return err
	}
	return sc.audit(ctx, eventbus.DeploymentReleaseStarted, map[string]string{"Kind": existing.Kind})
}

func (sc *reportScope) retireStaleRunning(ctx context.Context, target uuid.UUID) error {
	old, err := sc.q.GetRunningReleaseOfTarget(ctx, sqlc.GetRunningReleaseOfTargetParams{TargetID: target, OrganizationID: sc.lease.OrganizationId})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	active, err := sc.q.IsLeaseActive(ctx, old.LeaseId)
	if err != nil {
		return err
	}
	if active {
		return reject(connect.CodeFailedPrecondition, CodeReleaseConflict)
	}
	return sc.s.markUnknown(ctx, sc.q, old, sc.now)
}

func (sc *reportScope) persist(ctx context.Context, h *sqlc.DeploymentReleaseHost) error {
	return sc.q.UpdateReleaseHost(ctx, sqlc.UpdateReleaseHostParams{
		State: h.State, Batch: h.Batch, PreviousReleaseID: h.PreviousReleaseId, Health: h.Health, RollbackReason: h.RollbackReason,
		Message: h.Message, SwitchedAt: h.SwitchedAt, RolledBackAt: h.RolledBackAt, FinishedAt: h.FinishedAt, Now: sc.now,
		ID: h.Id, OrganizationID: h.OrganizationId,
	})
}

func (sc *reportScope) moveHost(ctx context.Context, ev *runnerv1.DeployEvent) error {
	if err := sc.knownHost(ev.GetHostId(), false); err != nil {
		return err
	}
	if err := sc.checkPrevious(ctx, ev.GetPreviousReleaseId()); err != nil {
		return err
	}
	h := sc.hosts[sc.hostKey(ev.GetHostId())]
	if ev.GetBatch() > 0 {
		h.Batch = pgtype.Int4{Int32: ev.GetBatch(), Valid: true}
	}
	if ev.GetPreviousReleaseId() != "" {
		h.PreviousReleaseId = text(ev.GetPreviousReleaseId())
	}
	if ev.GetMessage() != "" {
		h.Message = text(redactMessage(cleanMessage(ev.GetMessage())))
	}
	if ev.GetKind() == runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_HEALTH {
		if !slices.Contains(healthStates, h.State) || ev.GetHealth() == nil {
			return reject(connect.CodeFailedPrecondition, CodeIllegalTransition)
		}
		h.Health = healthJSON(ev.GetHealth())
		return sc.persist(ctx, h)
	}
	move, ok := hostMoves[ev.GetKind()]
	if !ok {
		return errInvalidReport("Kind")
	}
	if h.State == move.to {
		return nil
	}
	if !slices.Contains(move.from, h.State) {
		return reject(connect.CodeFailedPrecondition, CodeIllegalTransition)
	}
	if ev.GetReason() != "" {
		h.RollbackReason = text(ev.GetReason())
	}
	h.State = move.to
	sc.stamp(h)
	if err := sc.persist(ctx, h); err != nil {
		return err
	}
	if move.to == hostFailed && sc.rel.FailedHostId == nil {
		id := h.HostId
		sc.rel.FailedHostId = &id
		if err := sc.q.SetReleaseFailedHost(ctx, sqlc.SetReleaseFailedHostParams{FailedHostID: &id, Now: sc.now, ID: sc.rel.Id, OrganizationID: sc.rel.OrganizationId}); err != nil {
			return err
		}
	}
	if move.to == hostSwitched {
		return sc.audit(ctx, eventbus.DeploymentReleaseSwitched, map[string]string{"HostId": h.HostId.String()})
	}
	return nil
}

func (sc *reportScope) stamp(h *sqlc.DeploymentReleaseHost) {
	now := sc.now
	switch h.State {
	case hostSwitched:
		h.SwitchedAt = &now
	case hostRolledBack:
		h.RolledBackAt = &now
		h.FinishedAt = &now
	}
	if hostFinal(h.State) && h.FinishedAt == nil {
		h.FinishedAt = &now
	}
}

func (sc *reportScope) finishGroup(ctx context.Context, ev *runnerv1.DeployEvent) error {
	ordered := make([]*sqlc.DeploymentReleaseHost, 0, len(sc.hosts))
	states := make([]string, 0, len(sc.hosts))
	for _, h := range sc.hosts {
		ordered = append(ordered, h)
		states = append(states, h.State)
	}
	verdict := groupEnd(ev.GetKind(), states)
	if verdict.rejected {
		return reject(connect.CodeFailedPrecondition, CodeIllegalTransition)
	}
	for _, h := range ordered {
		to := ""
		switch {
		case slices.Contains(verdict.skip, h.State):
			to = hostSkipped
		case slices.Contains(verdict.unknown, h.State):
			to = hostUnknown
		}
		if to == "" {
			continue
		}
		h.State = to
		sc.stamp(h)
		if err := sc.persist(ctx, h); err != nil {
			return err
		}
	}
	reason := ev.GetReason()
	if err := sc.q.FinishRelease(ctx, sqlc.FinishReleaseParams{
		State: verdict.state, FailedHostID: sc.rel.FailedHostId, RollbackReason: text(reason), Message: text(redactMessage(cleanMessage(ev.GetMessage()))),
		Now: &sc.now, ID: sc.rel.Id, OrganizationID: sc.rel.OrganizationId,
	}); err != nil {
		return err
	}
	sc.rel.State = verdict.state
	if verdict.state == groupSucceeded {
		if err := sc.settleOlderUnknown(ctx); err != nil {
			return err
		}
	}
	return sc.auditEnd(ctx, verdict.state, reason)
}

func (sc *reportScope) auditEnd(ctx context.Context, state, reason string) error {
	names := map[string]string{
		groupSucceeded: eventbus.DeploymentReleaseSucceeded, groupRolledBack: eventbus.DeploymentReleaseRolledBack,
		groupRollbackFailed: eventbus.DeploymentReleaseRollbackFailed,
	}
	name, ok := names[state]
	if !ok {
		return nil
	}
	data := map[string]string{}
	if reason != "" {
		data["Reason"] = reason
	}
	if state == groupRollbackFailed {
		data[eventbus.SeverityKey] = eventbus.SeverityHigh
	}
	return sc.audit(ctx, name, data)
}

func (sc *reportScope) kindAndRestored(ctx context.Context, ev *runnerv1.DeployEvent) (string, string, error) {
	kind := modeDeploy
	if sc.mode == jobspec.TargetModeRollback {
		kind = modeRollback
	}
	if ev.GetMode() != "" && ev.GetMode() != kind {
		return "", "", errInvalidReport("Mode")
	}
	if kind != modeRollback {
		return kind, "", nil
	}
	restored := sc.inputs[releaseInput]
	rel, err := sc.q.GetRelease(ctx, sqlc.GetReleaseParams{TargetID: sc.tid, ReleaseID: restored, OrganizationID: sc.lease.OrganizationId})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (rel.Kind != modeDeploy || rel.State != groupSucceeded)) {
		return "", "", reject(connect.CodeFailedPrecondition, CodeIllegalTransition)
	}
	return kind, restored, err
}

func (sc *reportScope) healthInputs() []byte {
	out := map[string]string{}
	for _, k := range healthInputNames {
		if v := sc.inputs[k]; v != "" {
			out[k] = v
		}
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > maxHealthInputs {
		return []byte("{}")
	}
	return raw
}
