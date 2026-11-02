package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	runevents "github.com/Khaymah/dhow/apps/server/internal/Core/RunEvents"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Publisher interface {
	Publish(ctx context.Context, event eventbus.Event) error
}

type Listener func(ctx context.Context) (*pgx.Conn, error)

type Options struct {
	DB              DB
	Keys            *runnerauth.Keys
	Secrets         SecretResolver
	Bus             Publisher
	Listen          Listener
	MaxPolls        int
	Reads           sqlc.DBTX
	ReadWait        time.Duration
	Logs            config.Logs
	LogSettings     func() config.LogSettings
	StreamWriteWait time.Duration
	Log             *slog.Logger
	ManualJobs      bool
}

type Service struct {
	db       DB
	keys     *runnerauth.Keys
	secrets  SecretResolver
	bus      Publisher
	log      *slog.Logger
	hub      *hub
	maxPolls int
	logs     config.Logs
	limits   func() config.LogSettings
	streams  *streamLimiter
	wait     time.Duration
	reads    sqlc.DBTX
	readWait time.Duration
	readRate *rateLimiter
	openRate *rateLimiter
	manual   bool
	runs     runevents.Sink

	pollMu  sync.Mutex
	polls   int
	pollers map[uuid.UUID]struct{}
	buckets map[uuid.UUID]claimBucket

	stop     chan struct{}
	stopOnce sync.Once
}

func NewService(o Options) *Service {
	secrets := o.Secrets
	if secrets == nil {
		secrets = NoSecrets{}
	}
	maxPolls := o.MaxPolls
	if maxPolls <= 0 {
		maxPolls = config.RunnerDefaultMaxPolls
	}
	logs := logsOrDefault(o.Logs)
	wait := o.StreamWriteWait
	if wait <= 0 {
		wait = config.LogStreamWriteWait
	}
	readWait := o.ReadWait
	if readWait <= 0 {
		readWait = config.LogReadWait
	}
	limits := o.LogSettings
	if limits == nil {
		limits = config.DefaultLogSettings
	}
	var reads sqlc.DBTX = o.DB
	if o.Reads != nil {
		reads = o.Reads
	}
	return &Service{
		db: o.DB, keys: o.Keys, secrets: secrets, bus: o.Bus, log: o.Log, maxPolls: maxPolls, logs: logs, limits: limits, wait: wait,
		reads: reads, readWait: readWait, streams: newStreamLimiter(logs),
		readRate: newRateLimiter(config.LogReadBurst, config.LogReadRefill),
		openRate: newRateLimiter(config.LogStreamOpenBurst, config.LogStreamOpenRefill),
		hub:      newHub(o.Listen, o.Log), pollers: map[uuid.UUID]struct{}{}, buckets: map[uuid.UUID]claimBucket{},
		stop: make(chan struct{}), manual: o.ManualJobs,
	}
}

func logsOrDefault(l config.Logs) config.Logs {
	d := config.DefaultLogs()
	if l.MaxStreams <= 0 {
		l.MaxStreams = d.MaxStreams
	}
	if l.ReadConns <= 0 {
		l.ReadConns = d.ReadConns
	}
	return l
}

func (s *Service) readQueries() *sqlc.Queries { return sqlc.New(s.reads) }

func (s *Service) inTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	return dbtx.Run(ctx, s.db, fn)
}

func (s *Service) UseRunEvents(sink runevents.Sink) { s.runs = sink }

func (s *Service) jobFinished(ctx context.Context, q *sqlc.Queries, job sqlc.Job) error {
	if s.runs == nil || job.OriginKind != jobqueue.OriginPipeline || job.OriginId == nil {
		return nil
	}
	return s.runs.JobFinished(ctx, q, runevents.Finished{
		OrganizationID: job.OrganizationId, JobID: job.Id, OriginID: *job.OriginId, State: job.State, FailureReason: job.FailureReason,
	})
}

func (s *Service) Drain() { s.stopOnce.Do(func() { close(s.stop) }) }

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

var specJSON = protojson.MarshalOptions{UseProtoNames: true}

func encodeSpec(spec *runnerv1.JobSpec) ([]byte, error) {
	raw, err := specJSON.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("encode job spec: %w", err)
	}
	return raw, nil
}

func decodeSpec(raw []byte) (*runnerv1.JobSpec, error) {
	var spec runnerv1.JobSpec
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("decode stored job spec: %w", err)
	}
	return &spec, nil
}

func jobOf(j sqlc.Job) models.Job {
	return listedJob(sqlc.ListJobsRow{
		Id: j.Id, CreatedAt: j.CreatedAt, OrganizationId: j.OrganizationId, CreatedByUserId: j.CreatedByUserId, Name: j.Name,
		SpecDigest: j.SpecDigest, Image: j.Image, Labels: j.Labels, Isolation: j.Isolation, Priority: j.Priority, State: j.State,
		FailureReason: j.FailureReason, QueuedAt: j.QueuedAt, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, Deadline: j.Deadline,
		QueueDeadline: j.QueueDeadline, CancelRequestedAt: j.CancelRequestedAt, AttemptCount: j.AttemptCount,
		MaxInfraRetries: j.MaxInfraRetries,
	})
}

func listedJob(j sqlc.ListJobsRow) models.Job {
	return models.Job{
		ID: j.Id, OrganizationID: j.OrganizationId, Name: j.Name, State: j.State, FailureReason: j.FailureReason,
		Labels: j.Labels, Isolation: j.Isolation, Priority: int(j.Priority), Image: j.Image, SpecDigest: j.SpecDigest,
		AttemptCount: int(j.AttemptCount), MaxInfraRetries: int(j.MaxInfraRetries), QueuedAt: j.QueuedAt,
		StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, Deadline: j.Deadline, QueueDeadline: j.QueueDeadline,
		CancelRequestedAt: j.CancelRequestedAt, CreatedAt: j.CreatedAt, CreatedByUserID: j.CreatedByUserId,
	}
}

func intOf(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func attemptOf(a sqlc.JobAttempt, steps []sqlc.AttemptStep) models.Attempt {
	out := models.Attempt{
		ID: a.Id, Number: int(a.Number), RunnerID: a.RunnerId, FencingToken: a.FencingToken, State: a.State,
		FailureReason: a.FailureReason, ExitCode: intOf(a.ExitCode), LeasedAt: a.LeasedAt, StartedAt: a.StartedAt,
		FinishedAt: a.FinishedAt, MutationStartedAt: a.MutationStartedAt, Steps: []models.Step{},
	}
	for _, s := range steps {
		if s.AttemptId != a.Id {
			continue
		}
		out.Steps = append(out.Steps, models.Step{
			Index: int(s.Index), Name: s.Name, Mutating: s.Mutating, State: s.State, ExitCode: intOf(s.ExitCode),
			FailureReason: s.FailureReason, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
		})
	}
	return out
}

func isolationOf(spec *runnerv1.JobSpec) string { return jobspec.IsolationName(spec.GetIsolation()) }
