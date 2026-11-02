package jobs

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	messageInvalidValue = "errors.validationFailed"
	targetJob           = "job"
)

func principalOf(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func invalid(field string) apperrors.IErrorDetail {
	return apperrors.IErrorDetail{Field: field, Message: messageInvalidValue}
}

func specViolations(err error) error {
	var verr *jobspec.Error
	if !errors.As(err, &verr) {
		return apperrors.ValidationFailed(invalid("Spec"))
	}
	details := make([]apperrors.IErrorDetail, 0, len(verr.Violations))
	for _, v := range verr.Violations {
		details = append(details, invalid("Spec."+v.Field))
	}
	return apperrors.ValidationFailed(details...)
}

func boundedOr(v *int, def, lowest, highest int) (int, bool) {
	if v == nil {
		return def, true
	}
	return *v, *v >= lowest && *v <= highest
}

func (s *Service) Create(ctx context.Context, org uuid.UUID, in models.CreateInput) (models.Detail, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Detail{}, err
	}
	if !s.manual {
		return models.Detail{}, apperrors.NotFound()
	}
	var details []apperrors.IErrorDetail
	if !auth.ValidDisplayName(in.Name, config.JobNameMaxLength) {
		details = append(details, invalid("Name"))
	}
	priority, ok := boundedOr(in.Priority, 0, 0, config.JobPriorityMax)
	if !ok {
		details = append(details, invalid("Priority"))
	}
	retries, ok := boundedOr(in.MaxInfraRetries, config.JobDefaultInfraRetries, 0, config.JobMaxInfraRetries)
	if !ok {
		details = append(details, invalid("MaxInfraRetries"))
	}
	if len(details) > 0 {
		return models.Detail{}, apperrors.ValidationFailed(details...)
	}
	spec, err := jobspec.Prepare(in.Spec)
	if err != nil {
		return models.Detail{}, specViolations(err)
	}
	if names := jobspec.SecretNames(spec); len(names) > 0 {
		if err := s.secrets.Accepts(ctx, org, names); err != nil {
			if errors.Is(err, ErrSecretsUnavailable) {
				return models.Detail{}, apperrors.SecretsUnavailable()
			}
			return models.Detail{}, err
		}
	}
	return s.insert(ctx, p, org, in.Name, spec, priority, retries)
}

func displayImage(spec *runnerv1.JobSpec) string {
	if spec.GetImage() != "" {
		return spec.GetImage()
	}
	for _, s := range spec.GetSteps() {
		if p := s.GetPlugin(); p != nil {
			return p.GetImage()
		}
	}
	return ""
}

type newJob struct {
	org      uuid.UUID
	user     *uuid.UUID
	name     string
	spec     *runnerv1.JobSpec
	priority int
	retries  int
	origin   string
	originID *uuid.UUID
	features []string
}

func insertJob(ctx context.Context, q *sqlc.Queries, n newJob) (sqlc.Job, []byte, error) {
	raw, err := encodeSpec(n.spec)
	if err != nil {
		return sqlc.Job{}, nil, err
	}
	digest, err := jobspec.Digest(n.spec)
	if err != nil {
		return sqlc.Job{}, nil, err
	}
	if err := q.LockOrganizationQueue(ctx, n.org.String()); err != nil {
		return sqlc.Job{}, nil, err
	}
	needsFeatures := len(n.features) > 0
	queued, err := q.CountQueuedJobs(ctx, sqlc.CountQueuedJobsParams{OrganizationID: n.org, NeedsFeatures: needsFeatures})
	if err != nil {
		return sqlc.Job{}, nil, err
	}
	limit := int64(config.JobMaxQueuedPerOrg)
	if needsFeatures {
		limit = config.JobMaxQueuedWithFeatures
	}
	if queued >= limit {
		return sqlc.Job{}, nil, apperrors.JobQueueFull()
	}
	row, err := q.InsertJob(ctx, sqlc.InsertJobParams{
		OrganizationID: n.org, CreatedByUserID: n.user, Name: n.name, Spec: raw, SpecDigest: digest, Image: displayImage(n.spec),
		Labels: n.spec.GetLabels(), Isolation: isolationOf(n.spec), Priority: int32(n.priority), //nolint:gosec // Bounded to 0..100 by the callers.
		MaxInfraRetries: int32(n.retries), QueueTimeoutSeconds: config.JobQueueTimeout.Seconds(), //nolint:gosec // Bounded to 0..5 by the callers.
		OriginKind: n.origin, OriginID: n.originID, RequiredFeatures: nonNil(n.features),
	})
	if err != nil {
		return sqlc.Job{}, nil, err
	}
	if err := q.NotifyJobsQueued(ctx, n.org.String()); err != nil {
		return sqlc.Job{}, nil, err
	}
	return row, digest, nil
}

func (s *Service) insert(ctx context.Context, p auth.Principal, org uuid.UUID, name string, spec *runnerv1.JobSpec, priority, retries int) (models.Detail, error) {
	var row sqlc.Job
	var digest []byte
	wanted, ok := features(jobspec.Features(spec))
	if !ok {
		return models.Detail{}, apperrors.ValidationFailed(invalid("RequiredFeatures"))
	}
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		var err error
		row, digest, err = insertJob(ctx, q, newJob{
			org: org, user: &p.UserID, name: name, spec: spec, priority: priority, retries: retries, origin: jobqueue.OriginManual,
			features: wanted,
		})
		return err
	})
	if err != nil {
		return models.Detail{}, err
	}
	if err := s.publishByUser(ctx, eventbus.JobCreated, p, org, row.Id, map[string]string{
		"Name": row.Name, "Image": spec.GetImage(), "SpecDigest": hex.EncodeToString(digest),
		"Priority": strconv.Itoa(priority), "Steps": strconv.Itoa(len(spec.GetSteps())),
	}); err != nil {
		return models.Detail{}, err
	}
	return models.Detail{Job: jobOf(row), Spec: spec, Attempts: []models.Attempt{}}, nil
}

func (s *Service) ProjectOfJob(ctx context.Context, org, job uuid.UUID) (uuid.UUID, bool, error) {
	project, err := s.queries().GetJobProject(ctx, sqlc.GetJobProjectParams{ID: job, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return project, true, nil
}

func (s *Service) List(ctx context.Context, org uuid.UUID, states []string, page, limit int) ([]models.Job, int64, error) {
	q := s.queries()
	filter := nonNil(states)
	total, err := q.CountJobs(ctx, sqlc.CountJobsParams{OrganizationID: org, States: filter})
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListJobs(ctx, sqlc.ListJobsParams{
		OrganizationID: org, States: filter, PageLimit: int32(limit), PageOffset: int32((page - 1) * limit), //nolint:gosec // limit and page are bounded by response.PageParams.
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.Job, 0, len(rows))
	matching := map[string]bool{}
	for _, r := range rows {
		job := listedJob(r)
		if err := markUnmatched(ctx, q, &job, matching); err != nil {
			return nil, 0, err
		}
		out = append(out, job)
	}
	return out, total, nil
}

func markUnmatched(ctx context.Context, q *sqlc.Queries, job *models.Job, seen map[string]bool) error {
	if job.State != models.JobQueued {
		return nil
	}
	key := job.Isolation + "\x00" + strings.Join(job.Labels, "\x00")
	found, ok := seen[key]
	if !ok {
		n, err := q.CountMatchingRunners(ctx, sqlc.CountMatchingRunnersParams{
			OrganizationID: job.OrganizationID, Labels: nonNil(job.Labels), Isolation: job.Isolation,
		})
		if err != nil {
			return err
		}
		found = n > 0
		seen[key] = found
	}
	if !found {
		job.QueuedReason = models.ReasonNoMatchingRunner
	}
	return nil
}

func (s *Service) Get(ctx context.Context, org, id uuid.UUID) (models.Detail, error) {
	q := s.queries()
	row, err := q.GetJob(ctx, sqlc.GetJobParams{ID: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Detail{}, apperrors.NotFound()
	}
	if err != nil {
		return models.Detail{}, err
	}
	spec, err := decodeSpec(row.Spec)
	if err != nil {
		return models.Detail{}, err
	}
	attempts, err := q.ListJobAttempts(ctx, sqlc.ListJobAttemptsParams{JobID: id, OrganizationID: org})
	if err != nil {
		return models.Detail{}, err
	}
	steps, err := q.ListJobSteps(ctx, sqlc.ListJobStepsParams{JobID: id, OrganizationID: org})
	if err != nil {
		return models.Detail{}, err
	}
	job := jobOf(row)
	if err := markUnmatched(ctx, q, &job, map[string]bool{}); err != nil {
		return models.Detail{}, err
	}
	out := models.Detail{Job: job, Spec: spec, Attempts: make([]models.Attempt, 0, len(attempts))}
	for _, a := range attempts {
		out.Attempts = append(out.Attempts, attemptOf(a, steps))
	}
	return out, nil
}

func (s *Service) Cancel(ctx context.Context, org, id uuid.UUID) (models.Job, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Job{}, err
	}
	var out canceled
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		var err error
		out, err = s.cancelJob(ctx, q, org, id, p.UserID)
		if err == nil && out.finished && out.row.State != models.JobCanceled {
			return apperrors.JobFinished()
		}
		return err
	})
	if err != nil {
		return models.Job{}, err
	}
	if out.changed {
		if err := s.publishByUser(ctx, eventbus.JobCancelRequested, p, org, id, map[string]string{"State": out.row.State}); err != nil {
			return models.Job{}, err
		}
	}
	return jobOf(out.row), nil
}

type canceled struct {
	row      sqlc.Job
	changed  bool
	finished bool
}

func (s *Service) cancelJob(ctx context.Context, q *sqlc.Queries, org, id, user uuid.UUID) (canceled, error) {
	job, err := q.LockJob(ctx, sqlc.LockJobParams{ID: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return canceled{}, apperrors.NotFound()
	}
	if err != nil {
		return canceled{}, err
	}
	switch {
	case job.State == models.JobQueued:
		row, err := q.CancelQueuedJob(ctx, sqlc.CancelQueuedJobParams{ID: id, UserID: requester(user)})
		if err != nil {
			return canceled{}, err
		}
		return canceled{row: row, changed: true}, s.jobFinished(ctx, q, row)
	case !models.JobFinished(job.State) && job.CancelRequestedAt == nil:
		row, err := q.RequestJobCancel(ctx, sqlc.RequestJobCancelParams{ID: id, UserID: requester(user)})
		return canceled{row: row, changed: err == nil}, err
	default:
		return canceled{row: job, finished: models.JobFinished(job.State)}, nil
	}
}

func requester(user uuid.UUID) *uuid.UUID {
	if user == uuid.Nil {
		return nil
	}
	return &user
}

func (s *Service) publishByUser(ctx context.Context, name string, p auth.Principal, org, id uuid.UUID, data map[string]string) error {
	actor := p.UserID
	return s.bus.Publish(ctx, eventbus.Event{Name: name, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{org}, ActorUserID: &actor, TargetType: targetJob, TargetID: id.String(),
		IP: auth.RequestMetaFrom(ctx).IPString(), Data: data,
	}})
}
