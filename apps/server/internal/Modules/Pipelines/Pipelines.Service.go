package pipelines

import (
	"context"
	"encoding/hex"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	commitstatus "github.com/Khaymah/dhow/apps/server/internal/Core/CommitStatus"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
)

const targetRun = "run"

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Options struct {
	DB          DB
	Bus         Publisher
	Log         *slog.Logger
	Now         func() time.Time
	Queue       jobqueue.Queue
	Statuses    commitstatus.Recorder
	Definitions Definitions
	Images      ImageResolver
	Checkout    CheckoutSeam
	PublicURL   string
}

type Service struct {
	db          DB
	bus         Publisher
	log         *slog.Logger
	now         func() time.Time
	queue       jobqueue.Queue
	statuses    commitstatus.Recorder
	definitions Definitions
	images      ImageResolver
	checkout    CheckoutSeam
	publicURL   string
}

var (
	_ triggers.Sink  = (*Service)(nil)
	_ runevents.Sink = (*Service)(nil)
)

func NewService(o Options) *Service {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	definitions := o.Definitions
	if definitions == nil {
		definitions = ProviderDefinitions{}
	}
	checkout := o.Checkout
	if checkout == nil {
		checkout = CheckoutPending{}
	}
	return &Service{
		db: o.DB, bus: o.Bus, log: o.Log, now: now, queue: o.Queue, statuses: o.Statuses, definitions: definitions,
		images: o.Images, checkout: checkout, publicURL: strings.TrimSuffix(o.PublicURL, "/"),
	}
}

func (s *Service) UseStatuses(r commitstatus.Recorder) { s.statuses = r }

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func (s *Service) inTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	return dbtx.Run(ctx, s.db, fn)
}

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func text(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func int4(v int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(v), Valid: v > 0} //nolint:gosec // A pull request number from a provider, checked positive.
}

func userRef(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (s *Service) runURL(run sqlc.PipelineRun) string {
	if s.publicURL == "" {
		return ""
	}
	return s.publicURL + "/projects/" + run.ProjectId.String() + "/runs/" + strconv.FormatInt(run.Number, 10)
}

type auditEntry struct {
	name   string
	run    sqlc.PipelineRun
	actor  *uuid.UUID
	ip     string
	reason string
}

func (s *Service) audit(ctx context.Context, e auditEntry) {
	if s.bus == nil {
		return
	}
	data := map[string]string{
		"Number": strconv.FormatInt(e.run.Number, 10), "Trigger": e.run.Trigger, "Ref": e.run.Ref, "State": e.run.State,
		"ProjectId": e.run.ProjectId.String(),
	}
	if e.run.Sha.Valid {
		data["Sha"] = e.run.Sha.String
	}
	if len(e.run.DefinitionDigest) > 0 {
		data["SpecDigest"] = hex.EncodeToString(e.run.DefinitionDigest)
	}
	if e.reason != "" {
		data["Reason"] = e.reason
	}
	err := s.bus.Publish(ctx, eventbus.Event{Name: e.name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{e.run.OrganizationId}, ActorUserID: e.actor, TargetType: targetRun,
		TargetID: e.run.Id.String(), IP: e.ip, Data: data,
	}})
	if err != nil && s.log != nil {
		s.log.ErrorContext(ctx, "a run audit event was not recorded", "event", e.name, "run_id", e.run.Id.String(), "error", err)
	}
}

func (s *Service) auditByUser(ctx context.Context, name string, run sqlc.PipelineRun, p auth.Principal) {
	actor := p.UserID
	s.audit(ctx, auditEntry{name: name, run: run, actor: &actor, ip: auth.RequestMetaFrom(ctx).IPString()})
}

func (s *Service) recordStatus(ctx context.Context, q *sqlc.Queries, run sqlc.PipelineRun, state string) error {
	if s.statuses == nil || run.RepositoryId == nil || !run.Sha.Valid || !run.Provider.Valid || run.Provider.String == "generic" {
		return nil
	}
	return s.statuses.Record(ctx, q, commitstatus.Desired{
		OrganizationID: run.OrganizationId, RepositoryID: *run.RepositoryId, RunID: run.Id, Sha: run.Sha.String, State: state,
		TargetURL: s.runURL(run),
	})
}

func runOf(r sqlc.PipelineRun) models.Run {
	out := models.Run{
		ID: r.Id, ProjectID: r.ProjectId, Number: r.Number, Trigger: r.Trigger, Ref: r.Ref, Sha: r.Sha.String, BaseRef: r.BaseRef.String,
		BaseSha: r.BaseSha.String, IsFork: r.IsFork, PrNumber: int(r.PrNumber.Int32), Actor: r.Actor.String, State: r.State,
		FailureReason: r.FailureReason, DefinitionPath: r.DefinitionPath, DefinitionSha: r.DefinitionSha.String,
		ConcurrencyGroup: r.ConcurrencyGroup.String, TriggeredByUserID: r.TriggeredByUserId, ApprovedByUserID: r.ApprovedByUserId,
		ApprovedAt: r.ApprovedAt, CancelRequestedAt: r.CancelRequestedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if len(r.DefinitionDigest) > 0 {
		out.DefinitionDigest = hex.EncodeToString(r.DefinitionDigest)
	}
	return out
}
