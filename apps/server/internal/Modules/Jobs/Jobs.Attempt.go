package jobs

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/proto"

	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const failureReasonPrefix = "FAILURE_REASON_"

type leaseRef struct {
	id      uuid.UUID
	fencing int64
}

func parseRef(ref *runnerv1.LeaseRef) (leaseRef, error) {
	id, err := uuid.Parse(ref.GetLeaseId())
	if err != nil || ref.GetFencingToken() <= 0 {
		return leaseRef{}, leaseLost()
	}
	return leaseRef{id: id, fencing: ref.GetFencingToken()}, nil
}

func failureName(r runnerv1.FailureReason) string {
	if r == runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED {
		return ""
	}
	return strings.TrimPrefix(r.String(), failureReasonPrefix)
}

func (s *Service) withLease(ctx context.Context, ref *runnerv1.LeaseRef, fn func(q *sqlc.Queries, p runnerauth.Principal, l sqlc.LockActiveLeaseRow) error) error {
	p, err := runnerPrincipal(ctx)
	if err != nil {
		return err
	}
	r, err := parseRef(ref)
	if err != nil {
		return err
	}
	return dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		l, err := q.LockActiveLease(ctx, sqlc.LockActiveLeaseParams{LeaseID: r.id, FencingToken: r.fencing, RunnerID: p.RunnerID})
		if errors.Is(err, pgx.ErrNoRows) {
			return leaseLost()
		}
		if err != nil {
			return err
		}
		return fn(q, p, l)
	})
}

func (s *Service) Heartbeat(ctx context.Context, ref *runnerv1.LeaseRef) (*runnerv1.LeaseControl, error) {
	var ctrl *runnerv1.LeaseControl
	err := s.withLease(ctx, ref, func(q *sqlc.Queries, _ runnerauth.Principal, l sqlc.LockActiveLeaseRow) error {
		if _, err := q.ExtendLease(ctx, sqlc.ExtendLeaseParams{ID: l.Id, TtlSeconds: leaseTTL()}); err != nil {
			return err
		}
		ctrl = control(l.CancelRequestedAt)
		return nil
	})
	return ctrl, err
}

func leaseTTL() float64 { return float64(leaseTTLSeconds()) }

type started struct {
	secrets   *runnerv1.SealedSecrets
	ctrl      *runnerv1.LeaseControl
	noSecrets bool
}

func (s *Service) Start(ctx context.Context, ref *runnerv1.LeaseRef, proof []byte) (*runnerv1.SealedSecrets, *runnerv1.LeaseControl, error) {
	var out started
	err := s.withLease(ctx, ref, func(q *sqlc.Queries, p runnerauth.Principal, l sqlc.LockActiveLeaseRow) error {
		if l.AttemptState != models.AttemptLeased {
			return alreadyStarted()
		}
		if !runnersig.Verify(p.SigningPublicKey, runnersig.StartMessage(l.Id.String(), l.FencingToken, l.GrantDigest), proof) {
			return runnerauth.InvalidRunnerCredentials(0)
		}
		cancelAt, err := q.JobCancelRequestedAt(ctx, l.JobId)
		if err != nil {
			return err
		}
		if cancelAt != nil {
			return cancelRequested()
		}
		spec, err := decodeSpec(l.Spec)
		if err != nil {
			return err
		}
		values, err := s.resolveSecrets(ctx, l, spec)
		if reason, refused := secretRefusal(err); refused {
			out.noSecrets = true
			return s.finalize(ctx, q, l.Id, models.Outcome{
				State: models.AttemptFailed, FailureReason: reason, LeaseState: models.LeaseReleased,
			})
		}
		if err != nil {
			return err
		}
		defer wipe(values)
		if err := s.markStarted(ctx, q, l, spec); err != nil {
			return err
		}
		if len(values) > 0 {
			if out.secrets, err = seal(p, l, values); err != nil {
				return err
			}
		}
		out.ctrl = control(nil)
		return nil
	})
	if err == nil && out.noSecrets {
		return nil, nil, secretsUnavailable()
	}
	return out.secrets, out.ctrl, err
}

func (s *Service) markStarted(ctx context.Context, q *sqlc.Queries, l sqlc.LockActiveLeaseRow, spec *runnerv1.JobSpec) error {
	if !models.AttemptCanMove(l.AttemptState, models.AttemptRunning) {
		return errInvalidTransition
	}
	if _, err := q.StartAttempt(ctx, l.AttemptId); err != nil {
		return err
	}
	if _, err := q.MarkJobRunning(ctx, sqlc.MarkJobRunningParams{ID: l.JobId, TimeoutSeconds: float64(spec.GetTimeoutSeconds())}); err != nil {
		return err
	}
	_, err := q.ExtendLease(ctx, sqlc.ExtendLeaseParams{ID: l.Id, TtlSeconds: leaseTTL()})
	return err
}

func (s *Service) resolveSecrets(ctx context.Context, l sqlc.LockActiveLeaseRow, spec *runnerv1.JobSpec) (map[string][]byte, error) {
	if g, ok := s.secrets.(StartGuard); ok {
		if err := g.Admit(ctx, SecretRequest{OrganizationID: l.OrganizationId, JobID: l.JobId, AttemptID: l.AttemptId}); err != nil {
			return nil, err
		}
	}
	names := jobspec.SecretNames(spec)
	if len(names) == 0 {
		return nil, nil
	}
	registries := map[string]string{}
	for _, r := range spec.GetRegistryAuth() {
		registries[r.GetSecretName()] = r.GetHost()
	}
	values, err := s.secrets.Resolve(ctx, SecretRequest{
		OrganizationID: l.OrganizationId, JobID: l.JobId, AttemptID: l.AttemptId, Names: names, Checkout: spec.GetCheckout(),
		Registries: registries,
	})
	if err == nil {
		for _, n := range names {
			if _, ok := values[n]; !ok {
				wipe(values)
				err = ErrSecretsUnavailable
				break
			}
		}
	}
	if errors.Is(err, ErrSecretsUnavailable) {
		s.log.WarnContext(ctx, "the secrets of a job could not be resolved, so the attempt fails",
			"job_id", l.JobId.String(), "attempt_id", l.AttemptId.String(), "references", strings.Join(names, ","))
	}
	if err != nil {
		return nil, err
	}
	return values, nil
}

func secretRefusal(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrEnvironmentRuleChanged):
		return models.ReasonEnvironmentRule, true
	case errors.Is(err, ErrSecretsUnavailable):
		return models.ReasonSecretsMissing, true
	default:
		return "", false
	}
}

func seal(p runnerauth.Principal, l sqlc.LockActiveLeaseRow, values map[string][]byte) (*runnerv1.SealedSecrets, error) {
	bundle := &runnerv1.SecretBundle{}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		bundle.Secrets = append(bundle.Secrets, &runnerv1.SecretValue{Name: name, Value: values[name]})
	}
	plaintext, err := proto.MarshalOptions{Deterministic: true}.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	aad := runnersig.SecretsAAD(l.Id.String(), l.FencingToken, l.AttemptId.String(), p.RunnerID.String())
	enc, ciphertext, err := runnersig.SealSecrets(p.EncryptionPublicKey, aad, plaintext)
	if err != nil {
		return nil, err
	}
	return &runnerv1.SealedSecrets{Kem: runnerv1.HpkeKem_HPKE_KEM_MLKEM768_X25519, Enc: enc, Ciphertext: ciphertext}, nil
}

func wipe(values map[string][]byte) {
	for _, v := range values {
		clear(v)
	}
}

func stepOf(spec *runnerv1.JobSpec, index int32) (*runnerv1.Step, error) {
	if index < 0 || int(index) >= len(spec.GetSteps()) {
		return nil, runnerauth.ValidationFailed("Index")
	}
	return spec.GetSteps()[index], nil
}

func (s *Service) BeginStep(ctx context.Context, ref *runnerv1.LeaseRef, index int32, mutating bool) (*runnerv1.LeaseControl, error) {
	var ctrl *runnerv1.LeaseControl
	err := s.withLease(ctx, ref, func(q *sqlc.Queries, _ runnerauth.Principal, l sqlc.LockActiveLeaseRow) error {
		if l.AttemptState != models.AttemptRunning {
			return notRunning()
		}
		cancelAt, err := q.JobCancelRequestedAt(ctx, l.JobId)
		if err != nil {
			return err
		}
		if cancelAt != nil {
			return cancelRequested()
		}
		spec, err := decodeSpec(l.Spec)
		if err != nil {
			return err
		}
		step, err := stepOf(spec, index)
		if err != nil {
			return err
		}
		if step.GetMutating() != mutating {
			return runnerauth.ValidationFailed("Mutating")
		}
		if _, err := q.InsertAttemptStep(ctx, sqlc.InsertAttemptStepParams{
			AttemptID: l.AttemptId, Index: index, Name: step.GetName(), Mutating: step.GetMutating(),
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if step.GetMutating() {
			if err := q.MarkMutationStarted(ctx, l.AttemptId); err != nil {
				return err
			}
		}
		ctrl = control(nil)
		return nil
	})
	return ctrl, err
}

func (s *Service) EndStep(ctx context.Context, ref *runnerv1.LeaseRef, index, exitCode int32, reason runnerv1.FailureReason) (*runnerv1.LeaseControl, error) {
	var ctrl *runnerv1.LeaseControl
	err := s.withLease(ctx, ref, func(q *sqlc.Queries, _ runnerauth.Principal, l sqlc.LockActiveLeaseRow) error {
		if l.AttemptState != models.AttemptRunning {
			return notRunning()
		}
		spec, err := decodeSpec(l.Spec)
		if err != nil {
			return err
		}
		if _, err := stepOf(spec, index); err != nil {
			return err
		}
		if !slices.Contains(stepReasons(), reason) {
			return runnerauth.ValidationFailed("FailureReason")
		}
		state := models.StepSucceeded
		if exitCode != 0 || reason != runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED {
			state = models.StepFailed
		}
		n, err := q.EndAttemptStep(ctx, sqlc.EndAttemptStepParams{
			State: state, ExitCode: pgtype.Int4{Int32: exitCode, Valid: true}, FailureReason: failureName(reason),
			AttemptID: l.AttemptId, Index: index,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			if _, err := q.GetAttemptStepState(ctx, sqlc.GetAttemptStepStateParams{AttemptID: l.AttemptId, Index: index}); errors.Is(err, pgx.ErrNoRows) {
				return stepNotStarted()
			} else if err != nil {
				return err
			}
		}
		ctrl = control(l.CancelRequestedAt)
		return nil
	})
	return ctrl, err
}
