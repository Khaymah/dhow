package environments

import (
	"context"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	targetCheck     = "deploy_target"
	repairedMessage = "the state was written from a check of the target"
	checkOK         = "ok"
	checkDiverged   = "diverged"
	maxFreeBytes    = int64(1) << 62
)

var checkReasons = []string{
	checkOK, "host_key_mismatch", "unreachable", "auth_failed", "path_missing", "current_not_symlink", "state_corrupt", "disk_low", "locked",
	checkDiverged,
}

func validCheck(ev *runnerv1.DeployEvent) *rejection {
	switch {
	case ev.GetHostId() == "" || len(ev.GetHostId()) > 64:
		return errInvalidReport("HostId")
	case !slices.Contains(checkReasons, ev.GetReason()):
		return errInvalidReport("Reason")
	case ev.GetCurrentReleaseId() != "" && !releaseIDShape.MatchString(ev.GetCurrentReleaseId()):
		return errInvalidReport("CurrentReleaseId")
	case ev.GetVerifiedReleaseId() != "" && !releaseIDShape.MatchString(ev.GetVerifiedReleaseId()):
		return errInvalidReport("VerifiedReleaseId")
	case ev.GetFreeBytes() < 0 || ev.GetFreeBytes() > maxFreeBytes:
		return errInvalidReport("FreeBytes")
	case ev.GetReleaseId() != "" || ev.GetPreviousReleaseId() != "" || ev.GetBatch() != 0 || ev.GetMode() != "" || ev.GetHealth() != nil:
		return errInvalidReport("Kind")
	case len(ev.GetMessage()) > 4*MaxReportMessage:
		return errInvalidReport("Message")
	}
	return nil
}

func (sc *reportScope) checked(ctx context.Context, ev *runnerv1.DeployEvent) error {
	if sc.mode != jobspec.TargetModeCheck {
		return reject(connect.CodeFailedPrecondition, CodeIllegalTransition)
	}
	hid := sc.hostKey(ev.GetHostId())
	if !slices.Contains(sc.hids, hid) {
		return reject(connect.CodePermissionDenied, CodeUnknownHost)
	}
	id, err := sc.q.UpsertHostCheck(ctx, sqlc.UpsertHostCheckParams{
		OrganizationID: sc.lease.OrganizationId, TargetID: sc.tid, HostID: hid, DeploymentID: *sc.lease.DeploymentId, RunJobID: *sc.lease.RunJobId,
		Reason: ev.GetReason(), Message: text(redactMessage(cleanMessage(ev.GetMessage()))), CurrentReleaseID: text(ev.GetCurrentReleaseId()),
		VerifiedReleaseID: text(ev.GetVerifiedReleaseId()), FreeBytes: pgtype.Int8{Int64: ev.GetFreeBytes(), Valid: true}, Locked: ev.GetLocked(),
		Now: sc.now,
	})
	if err != nil {
		return err
	}
	if err := sc.q.SupersedeOlderHostChecks(ctx, sqlc.SupersedeOlderHostChecksParams{
		TargetID: sc.tid, HostID: hid, OrganizationID: sc.lease.OrganizationId, ID: id, Now: sc.now,
	}); err != nil {
		return err
	}
	repaired, err := sc.repair(ctx, hid, ev)
	if err != nil {
		return err
	}
	if repaired {
		if err := sc.q.MarkHostCheckRepaired(ctx, sqlc.MarkHostCheckRepairedParams{ID: id, OrganizationID: sc.lease.OrganizationId, Now: sc.now}); err != nil {
			return err
		}
	}
	return sc.s.bus.Publish(ctx, eventbus.Event{Name: eventbus.DeploymentTargetChecked, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{sc.lease.OrganizationId}, TargetType: targetCheck, TargetID: sc.tid.String(),
		Data: map[string]string{"HostId": hid.String(), "Reason": ev.GetReason(), "DeploymentId": sc.lease.DeploymentId.String()}, Tx: sc.q,
	}})
}

func (sc *reportScope) repair(ctx context.Context, host uuid.UUID, ev *runnerv1.DeployEvent) (bool, error) {
	if ev.GetReason() != checkOK {
		return false, nil
	}
	return sc.writeTruth(ctx, host, ev.GetCurrentReleaseId(), ev.GetVerifiedReleaseId())
}

func (sc *reportScope) writeTruth(ctx context.Context, host uuid.UUID, current, verified string) (bool, error) {
	if current == "" || current != verified {
		return false, nil
	}
	rows, err := sc.q.LockUnknownReleaseHosts(ctx, sqlc.LockUnknownReleaseHostsParams{HostID: host, OrganizationID: sc.lease.OrganizationId, TargetID: sc.tid})
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		to := hostRolledBack
		var rolledBack *time.Time
		if row.ReleaseId == current {
			to = hostVerified
		} else {
			rolledBack = &sc.now
		}
		if err := sc.q.RepairReleaseHost(ctx, sqlc.RepairReleaseHostParams{
			State: to, Message: text(repairedMessage), RolledBackAt: rolledBack, Now: &sc.now, ID: row.Id, OrganizationID: sc.lease.OrganizationId,
		}); err != nil {
			return false, err
		}
		if row.GroupState == groupUnknown {
			if err := sc.settleRepaired(ctx, row.ReleaseRowId); err != nil {
				return false, err
			}
		}
	}
	return len(rows) > 0, nil
}

func (sc *reportScope) settleRepaired(ctx context.Context, release uuid.UUID) error {
	states, err := sc.q.ListReleaseHostStates(ctx, sqlc.ListReleaseHostStatesParams{ReleaseRowID: release, OrganizationID: sc.lease.OrganizationId})
	if err != nil {
		return err
	}
	if slices.Contains(states, hostUnknown) {
		return nil
	}
	verified := countStates(states, hostVerified)
	to := ""
	switch {
	case verified == len(states):
		to = groupSucceeded
	case verified == 0:
		to = groupRolledBack
	default:
		return nil
	}
	if err := sc.q.RepairRelease(ctx, sqlc.RepairReleaseParams{State: to, Message: text(repairedMessage), Now: sc.now, ID: release, OrganizationID: sc.lease.OrganizationId}); err != nil {
		return err
	}
	return sc.s.bus.Publish(ctx, eventbus.Event{Name: eventbus.DeploymentReleaseRepaired, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{sc.lease.OrganizationId}, TargetType: targetRelease, TargetID: release.String(),
		Data: map[string]string{"TargetId": sc.tid.String(), "State": to}, Tx: sc.q,
	}})
}

func (sc *reportScope) settleOlderUnknown(ctx context.Context) error {
	for hid := range sc.hosts {
		if _, err := sc.writeTruth(ctx, hid, sc.rel.ReleaseId, sc.rel.ReleaseId); err != nil {
			return err
		}
	}
	return nil
}
