package jobs

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var retryableFailures = []string{
	failureName(runnerv1.FailureReason_FAILURE_REASON_IMAGE_PULL_FAILED),
	failureName(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED),
	failureName(runnerv1.FailureReason_FAILURE_REASON_RUNNER_DISK_FULL),
	failureName(runnerv1.FailureReason_FAILURE_REASON_RESOURCE_REQUEST_EXCEEDS_RUNNER),
}

type CompleteInput struct {
	Lease    *runnerv1.LeaseRef
	Result   runnerv1.AttemptResult
	ExitCode int32
	Reason   runnerv1.FailureReason
	Digest   []byte
	Outputs  map[string]string
}

type resultRule struct {
	state    string
	fallback runnerv1.FailureReason
	reasons  []runnerv1.FailureReason
}

func deployReasons() []runnerv1.FailureReason {
	return []runnerv1.FailureReason{
		runnerv1.FailureReason_FAILURE_REASON_DEPLOY_FAILED, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_ROLLED_BACK,
		runnerv1.FailureReason_FAILURE_REASON_DEPLOY_HEALTH_FAILED, runnerv1.FailureReason_FAILURE_REASON_DEPLOY_ROLLBACK_FAILED,
		runnerv1.FailureReason_FAILURE_REASON_TARGET_LOCKED, runnerv1.FailureReason_FAILURE_REASON_CURRENT_NOT_A_SYMLINK,
		runnerv1.FailureReason_FAILURE_REASON_TARGET_HOST_UNREACHABLE, runnerv1.FailureReason_FAILURE_REASON_TARGET_STATE_CORRUPT,
		runnerv1.FailureReason_FAILURE_REASON_TARGET_CHECK_FAILED,
	}
}

func resultRules() map[runnerv1.AttemptResult]resultRule {
	rules := baseRules()
	failed := rules[runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED]
	failed.reasons = append(failed.reasons, deployReasons()...)
	rules[runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED] = failed
	return rules
}

func baseRules() map[runnerv1.AttemptResult]resultRule {
	unspecified := runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED
	return map[runnerv1.AttemptResult]resultRule{
		runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED: {models.AttemptSucceeded, unspecified, nil},
		runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED: {models.AttemptFailed, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, []runnerv1.FailureReason{
			runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, runnerv1.FailureReason_FAILURE_REASON_OOM_KILLED,
			runnerv1.FailureReason_FAILURE_REASON_PIDS_LIMIT_REACHED, runnerv1.FailureReason_FAILURE_REASON_IMAGE_PULL_FAILED,
			runnerv1.FailureReason_FAILURE_REASON_SHELL_NOT_FOUND, runnerv1.FailureReason_FAILURE_REASON_SPEC_REJECTED,
			runnerv1.FailureReason_FAILURE_REASON_RESOURCE_REQUEST_EXCEEDS_RUNNER, runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED,
			runnerv1.FailureReason_FAILURE_REASON_RUNNER_DISK_FULL, runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_FAILED,
			runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_REF_MOVED, runnerv1.FailureReason_FAILURE_REASON_HOST_KEY_MISMATCH,
			runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_DISK_FULL, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID,
			runnerv1.FailureReason_FAILURE_REASON_PLUGIN_MANIFEST_INVALID, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_DOWNLOAD_FAILED,
			runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UNSAFE, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UPLOAD_FAILED,
			runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_TOO_LARGE, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_CONTAINS_SECRET,
		}},
		runnerv1.AttemptResult_ATTEMPT_RESULT_CANCELED: {
			models.AttemptCanceled, runnerv1.FailureReason_FAILURE_REASON_CANCELED,
			[]runnerv1.FailureReason{runnerv1.FailureReason_FAILURE_REASON_CANCELED},
		},
		runnerv1.AttemptResult_ATTEMPT_RESULT_TIMED_OUT: {
			models.AttemptTimedOut, runnerv1.FailureReason_FAILURE_REASON_TIMED_OUT,
			[]runnerv1.FailureReason{runnerv1.FailureReason_FAILURE_REASON_TIMED_OUT},
		},
		runnerv1.AttemptResult_ATTEMPT_RESULT_LOST: {models.AttemptLost, runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST, []runnerv1.FailureReason{
			runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED, runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST,
		}},
	}
}

func stepReasons() []runnerv1.FailureReason {
	return append(deployReasons(), []runnerv1.FailureReason{
		runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED,
		runnerv1.FailureReason_FAILURE_REASON_OOM_KILLED, runnerv1.FailureReason_FAILURE_REASON_PIDS_LIMIT_REACHED,
		runnerv1.FailureReason_FAILURE_REASON_TIMED_OUT, runnerv1.FailureReason_FAILURE_REASON_CANCELED,
		runnerv1.FailureReason_FAILURE_REASON_SHELL_NOT_FOUND, runnerv1.FailureReason_FAILURE_REASON_IMAGE_PULL_FAILED,
		runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID,
		runnerv1.FailureReason_FAILURE_REASON_PLUGIN_MANIFEST_INVALID,
		runnerv1.FailureReason_FAILURE_REASON_HOST_KEY_MISMATCH, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_UNSAFE,
	}...)
}

func outcomeOf(in CompleteInput) (models.Outcome, error) {
	rule, ok := resultRules()[in.Result]
	if !ok {
		return models.Outcome{}, runnerauth.ValidationFailed("Result")
	}
	if in.Result == runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED && in.ExitCode != 0 {
		return models.Outcome{}, runnerauth.ValidationFailed("Result")
	}
	reason := in.Reason
	switch {
	case reason == runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED:
		reason = rule.fallback
	case !slices.Contains(rule.reasons, reason):
		return models.Outcome{}, runnerauth.ValidationFailed("FailureReason")
	}
	exit := in.ExitCode
	out := models.Outcome{
		State: rule.state, FailureReason: failureName(reason), ExitCode: &exit, Digest: in.Digest, LeaseState: models.LeaseReleased,
	}
	if rule.state == models.AttemptSucceeded {
		out.Outputs = in.Outputs
	}
	return out, nil
}

func (s *Service) Complete(ctx context.Context, in CompleteInput) error {
	p, err := runnerPrincipal(ctx)
	if err != nil {
		return err
	}
	r, err := parseRef(in.Lease)
	if err != nil {
		return err
	}
	outcome, err := outcomeOf(in)
	if err != nil {
		return err
	}
	if len(in.Digest) != runnersig.DigestSize {
		return runnerauth.ValidationFailed("Digest")
	}
	late := uuid.Nil
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		row, err := q.LockLeaseForCompletion(ctx, sqlc.LockLeaseForCompletionParams{LeaseID: r.id, FencingToken: r.fencing, RunnerID: p.RunnerID})
		if errors.Is(err, pgx.ErrNoRows) {
			return leaseLost()
		}
		if err != nil {
			return err
		}
		want := runnersig.CompletionDigest(row.AttemptId.String(), r.fencing, int32(in.Result), in.ExitCode, int32(in.Reason))
		if subtle.ConstantTimeCompare(want, in.Digest) != 1 {
			return runnerauth.ValidationFailed("Digest")
		}
		if err := requireTargetForReason(row.Spec, in.Reason); err != nil {
			return err
		}
		if models.AttemptFinished(row.AttemptState) {
			switch {
			case bytes.Equal(row.CompletionDigest, in.Digest):
				return nil
			case row.CompletionDigest != nil:
				return completionConflict()
			default:
				late = row.AttemptId
				return nil
			}
		}
		if row.State != models.LeaseActive || !row.Live {
			late = row.AttemptId
			return nil
		}
		if !models.AttemptCanMove(row.AttemptState, outcome.State) {
			return notRunning()
		}
		return s.finalize(ctx, q, row.Id, outcome)
	})
	if err != nil || late == uuid.Nil {
		return err
	}
	s.log.InfoContext(ctx, "a runner completed an attempt after its lease ended",
		"attempt_id", late.String(), "result", outcome.State, "failure_reason", outcome.FailureReason)
	s.recordLateResult(ctx, late, outcome)
	return leaseLost()
}

func (s *Service) finalize(ctx context.Context, q *sqlc.Queries, leaseID uuid.UUID, o models.Outcome) error {
	lease, err := q.GetLeaseAttempt(ctx, leaseID)
	if err != nil {
		return err
	}
	if !models.AttemptCanMove(lease.AttemptState, o.State) {
		o.State, o.FailureReason = models.AttemptLost, models.ReasonLeaseLost
	}
	exit := pgtype.Int4{}
	if o.ExitCode != nil {
		exit = pgtype.Int4{Int32: *o.ExitCode, Valid: true}
	}
	attempt, err := q.FinishAttempt(ctx, sqlc.FinishAttemptParams{
		State: o.State, FailureReason: o.FailureReason, ExitCode: exit, CompletionDigest: o.Digest, ID: lease.AttemptId,
	})
	if err != nil {
		return fmt.Errorf("finish attempt: %w", err)
	}
	if err := q.NotifyLogAppended(ctx, attempt.Id.String()+":"+strconv.FormatInt(attempt.LastSeq, 10)); err != nil {
		return err
	}
	if _, err := q.EndLease(ctx, sqlc.EndLeaseParams{State: o.LeaseState, ID: leaseID}); err != nil {
		return err
	}
	if err := q.InterruptAttemptSteps(ctx, attempt.Id); err != nil {
		return err
	}
	job, err := q.LockJobByID(ctx, lease.JobId)
	if err != nil {
		return err
	}
	to, reason := decide(job, attempt)
	if !models.JobCanMove(job.State, to) {
		return fmt.Errorf("job %s from %s to %s: %w", job.Id, job.State, to, errInvalidTransition)
	}
	if to == models.JobQueued {
		if _, err := q.RequeueJob(ctx, sqlc.RequeueJobParams{ID: job.Id, FromState: job.State}); err != nil {
			return err
		}
		return q.NotifyJobsQueued(ctx, job.OrganizationId.String())
	}
	finished, err := q.FinishJob(ctx, sqlc.FinishJobParams{ID: job.Id, State: to, FailureReason: reason, FromState: job.State})
	if err != nil {
		return err
	}
	return s.jobFinished(ctx, q, finished, o.Outputs)
}

func decide(job sqlc.Job, attempt sqlc.JobAttempt) (string, string) {
	switch attempt.State {
	case models.AttemptSucceeded:
		return models.JobSucceeded, ""
	case models.AttemptCanceled:
		return models.JobCanceled, models.ReasonCanceled
	case models.AttemptTimedOut:
		return models.JobTimedOut, attempt.FailureReason
	case models.AttemptFailed:
		if !slices.Contains(retryableFailures, attempt.FailureReason) {
			return models.JobFailed, attempt.FailureReason
		}
	}
	final := models.JobLost
	if attempt.State == models.AttemptFailed {
		final = models.JobFailed
	}
	switch {
	case job.CancelRequestedAt != nil:
		return models.JobCanceled, models.ReasonCanceled
	case attempt.MutationStartedAt != nil && final == models.JobLost:
		return models.JobLost, models.ReasonLostAfterMutation
	case attempt.MutationStartedAt != nil:
		return models.JobFailed, attempt.FailureReason
	case job.AttemptCount < 1+job.MaxInfraRetries:
		return models.JobQueued, ""
	default:
		return final, models.ReasonRetriesExhausted
	}
}

func requireTargetForReason(raw []byte, reason runnerv1.FailureReason) error {
	if !slices.Contains(deployReasons(), reason) {
		return nil
	}
	spec, err := decodeSpec(raw)
	if err != nil {
		return err
	}
	if len(spec.GetTargets()) == 0 {
		return runnerauth.ValidationFailed("FailureReason")
	}
	return nil
}
