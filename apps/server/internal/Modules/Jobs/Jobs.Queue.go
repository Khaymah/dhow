package jobs

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	jobqueue "github.com/Khaymah/dhow/apps/server/internal/Core/JobQueue"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var ErrInvalidEnqueue = errors.New("invalid job queue request")

type queue struct {
	service *Service
}

var _ jobqueue.Queue = queue{}

func (s *Service) Queue() jobqueue.Queue { return queue{service: s} }

func knownFeatures() []string {
	return []string{jobqueue.FeatureArtifacts, jobqueue.FeatureCache, jobqueue.FeatureCheckout, jobqueue.FeaturePlugins}
}

func features(in []string) ([]string, bool) {
	out := make([]string, 0, len(in))
	for _, f := range in {
		if !slices.Contains(knownFeatures(), f) {
			return nil, false
		}
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out, true
}

func checkRequest(r jobqueue.Request) (int, []string, error) {
	if r.OriginKind != jobqueue.OriginPipeline || r.OriginID == uuid.Nil || r.OrganizationID == uuid.Nil || r.CreatedByUserID == uuid.Nil {
		return 0, nil, fmt.Errorf("%w: a queued job needs a pipeline origin, an organization and a creator", ErrInvalidEnqueue)
	}
	if !auth.ValidDisplayName(r.Name, config.JobNameMaxLength) {
		return 0, nil, apperrors.ValidationFailed(invalid("Name"))
	}
	if r.Priority < 0 || r.Priority > config.JobPriorityMax {
		return 0, nil, apperrors.ValidationFailed(invalid("Priority"))
	}
	retries, ok := boundedOr(r.MaxInfraRetries, config.JobDefaultInfraRetries, 0, config.JobMaxInfraRetries)
	if !ok {
		return 0, nil, apperrors.ValidationFailed(invalid("MaxInfraRetries"))
	}
	wanted, ok := features(r.RequiredFeatures)
	if !ok {
		return 0, nil, apperrors.ValidationFailed(invalid("RequiredFeatures"))
	}
	return retries, wanted, nil
}

func userSecrets(spec *runnerv1.JobSpec) []string {
	return slices.DeleteFunc(jobspec.SecretNames(spec), func(name string) bool { return name == jobspec.CheckoutSecret })
}

func checkOwnership(ctx context.Context, tx *sqlc.Queries, r jobqueue.Request) error {
	member, err := tx.IsEnabledMemberOf(ctx, sqlc.IsEnabledMemberOfParams{UserID: r.CreatedByUserID, OrganizationID: r.OrganizationID})
	if err != nil {
		return err
	}
	if !member {
		return fmt.Errorf("%w: the creator is not an active member of the organization", ErrInvalidEnqueue)
	}
	elsewhere, err := tx.OriginUsedElsewhere(ctx, sqlc.OriginUsedElsewhereParams{OriginID: &r.OriginID, OrganizationID: r.OrganizationID})
	if err != nil {
		return err
	}
	if elsewhere {
		return fmt.Errorf("%w: the origin already belongs to another organization", ErrInvalidEnqueue)
	}
	return nil
}

func (qu queue) Enqueue(ctx context.Context, tx *sqlc.Queries, r jobqueue.Request) (jobqueue.Enqueued, error) {
	retries, wanted, err := checkRequest(r)
	if err != nil {
		return jobqueue.Enqueued{}, err
	}
	spec, err := jobspec.Prepare(r.Spec)
	if err != nil {
		return jobqueue.Enqueued{}, specViolations(err)
	}
	wanted, ok := features(append(wanted, jobspec.Features(spec)...))
	if !ok {
		return jobqueue.Enqueued{}, apperrors.ValidationFailed(invalid("RequiredFeatures"))
	}
	if names := userSecrets(spec); len(names) > 0 {
		if err := qu.service.secrets.Accepts(ctx, r.OrganizationID, names); err != nil {
			if errors.Is(err, ErrSecretsUnavailable) {
				return jobqueue.Enqueued{}, apperrors.SecretsUnavailable()
			}
			return jobqueue.Enqueued{}, err
		}
	}
	if err := checkOwnership(ctx, tx, r); err != nil {
		return jobqueue.Enqueued{}, err
	}
	origin := r.OriginID
	row, digest, err := insertJob(ctx, tx, newJob{
		org: r.OrganizationID, user: r.CreatedByUserID, name: r.Name, spec: spec, priority: r.Priority, retries: retries,
		origin: jobqueue.OriginPipeline, originID: &origin, features: wanted,
	})
	if err != nil {
		return jobqueue.Enqueued{}, err
	}
	return jobqueue.Enqueued{JobID: row.Id, SpecDigest: digest}, nil
}

func (qu queue) Cancel(ctx context.Context, tx *sqlc.Queries, c jobqueue.Cancellation) (bool, error) {
	if c.OrganizationID == uuid.Nil || c.JobID == uuid.Nil || c.RequestedBy == uuid.Nil {
		return false, fmt.Errorf("%w: a cancellation needs an organization, a job and who asked", ErrInvalidEnqueue)
	}
	out, err := cancelJob(ctx, tx, c.OrganizationID, c.JobID, c.RequestedBy)
	if err != nil {
		return false, err
	}
	return out.changed, nil
}
