package environments

import (
	"connectrpc.com/connect"
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const targetRelease = "deployment_release"

type reportScope struct {
	s      *Service
	q      *sqlc.Queries
	now    time.Time
	lease  sqlc.LockReleaseLeaseRow
	target *runnerv1.StepTarget
	step   int32
	digest []byte
	hosts  map[uuid.UUID]*sqlc.DeploymentReleaseHost
	rel    *sqlc.DeploymentRelease
}

func (s *Service) ReportRelease(ctx context.Context, lease *runnerv1.LeaseRef, step int32, events []*runnerv1.DeployEvent) (int32, error) {
	id, err := uuid.Parse(lease.GetLeaseId())
	if err != nil || lease.GetFencingToken() <= 0 {
		return 0, reject(connect.CodeFailedPrecondition, CodeReleaseLeaseLost)
	}
	if len(events) == 0 || len(events) > MaxReportEvents {
		return 0, errInvalidReport("Events")
	}
	for _, ev := range events {
		if r := validEvent(ev); r != nil {
			return 0, r
		}
	}
	p, ok := runnerauth.PrincipalFrom(ctx)
	if !ok {
		return 0, reject(connect.CodeUnauthenticated, CodeReleaseUnauth)
	}
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		row, err := q.LockReleaseLease(ctx, sqlc.LockReleaseLeaseParams{LeaseID: id, FencingToken: lease.GetFencingToken(), RunnerID: p.RunnerID})
		if errors.Is(err, pgx.ErrNoRows) {
			return reject(connect.CodeFailedPrecondition, CodeReleaseLeaseLost)
		}
		if err != nil {
			return err
		}
		sc, err := s.reportScope(q, row, step)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if err := sc.apply(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return int32(len(events)), nil //nolint:gosec // At most MaxReportEvents.
}

func (s *Service) reportScope(q *sqlc.Queries, row sqlc.LockReleaseLeaseRow, step int32) (*reportScope, error) {
	var spec runnerv1.JobSpec
	if err := (protojson.UnmarshalOptions{}).Unmarshal(row.Spec, &spec); err != nil {
		return nil, err
	}
	sc := &reportScope{s: s, q: q, now: s.now(), lease: row, step: step}
	for _, t := range spec.GetTargets() {
		if t.GetStepIndex() == step {
			sc.target = t
		}
	}
	if sc.target == nil || row.RunJobId == nil || row.DeploymentId == nil {
		return nil, reject(connect.CodePermissionDenied, CodeNotTargetStep)
	}
	if name := spec.GetSteps()[step].GetPlugin().GetInputs()[jobspec.TargetArtifactInput]; name != "" {
		for _, in := range spec.GetInputs() {
			if raw, err := hex.DecodeString(in.GetDigest()); err == nil && in.GetName() == name && len(raw) == 32 {
				sc.digest = raw
			}
		}
	}
	return sc, nil
}

func (sc *reportScope) apply(ctx context.Context, ev *runnerv1.DeployEvent) error {
	kind := ev.GetKind()
	if rid := ev.GetReleaseId(); rid != "" && rid != sc.target.GetReleaseId() {
		return errInvalidReport("ReleaseId")
	}
	if kind == runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_RELEASE_STARTED {
		return sc.start(ctx, ev)
	}
	if err := sc.load(ctx); err != nil {
		return err
	}
	if sc.rel.LeaseId != sc.lease.LeaseId {
		return reject(connect.CodePermissionDenied, CodeLeaseMismatch)
	}
	if kind == runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_PRUNED {
		return sc.knownHost(ev.GetHostId(), true)
	}
	if sc.rel.State != groupRunning {
		return reject(connect.CodeFailedPrecondition, CodeReleaseFinished)
	}
	if kind == runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_BATCH_PASSED {
		return sc.batchPassed(ev)
	}
	if ev.GetHostId() == "" {
		return sc.finishGroup(ctx, ev)
	}
	return sc.moveHost(ctx, ev)
}

func (sc *reportScope) load(ctx context.Context) error {
	if sc.rel != nil {
		return nil
	}
	rel, err := sc.q.GetReleaseForUpdate(ctx, sqlc.GetReleaseForUpdateParams{
		TargetID: uuid.MustParse(sc.target.GetTargetId()), ReleaseID: sc.target.GetReleaseId(), OrganizationID: sc.lease.OrganizationId,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return reject(connect.CodeFailedPrecondition, CodeReleaseNotStarted)
	}
	if err != nil {
		return err
	}
	rows, err := sc.q.ListReleaseHostsForUpdate(ctx, sqlc.ListReleaseHostsForUpdateParams{ReleaseRowID: rel.Id, OrganizationID: rel.OrganizationId})
	if err != nil {
		return err
	}
	sc.rel = &rel
	sc.hosts = make(map[uuid.UUID]*sqlc.DeploymentReleaseHost, len(rows))
	for i := range rows {
		sc.hosts[rows[i].HostId] = &rows[i]
	}
	return nil
}

func (sc *reportScope) knownHost(raw string, optional bool) error {
	if raw == "" && optional {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return reject(connect.CodePermissionDenied, CodeUnknownHost)
	}
	if _, ok := sc.hosts[id]; !ok {
		return reject(connect.CodePermissionDenied, CodeUnknownHost)
	}
	return nil
}

func (sc *reportScope) batchPassed(ev *runnerv1.DeployEvent) error {
	if ev.GetBatch() < 1 || int(ev.GetBatch()) > len(sc.hosts) {
		return errInvalidReport("Batch")
	}
	return nil
}

func (sc *reportScope) audit(ctx context.Context, name string, data map[string]string) error {
	data["TargetId"], data["ReleaseId"] = sc.target.GetTargetId(), sc.target.GetReleaseId()
	data["DeploymentId"], data["Step"] = sc.lease.DeploymentId.String(), strconv.Itoa(int(sc.step))
	return sc.s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{sc.lease.OrganizationId}, TargetType: targetRelease, TargetID: sc.rel.Id.String(), Data: data, Tx: sc.q,
	}})
}

func text(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }
