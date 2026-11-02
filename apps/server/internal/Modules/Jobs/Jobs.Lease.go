package jobs

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func runnerPrincipal(ctx context.Context) (runnerauth.Principal, error) {
	p, ok := runnerauth.PrincipalFrom(ctx)
	if !ok {
		return runnerauth.Principal{}, runnerauth.Unauthenticated()
	}
	return p, nil
}

func leaseTTLSeconds() int32 { return int32(config.JobLeaseTTL / time.Second) }

func control(cancel *time.Time) *runnerv1.LeaseControl {
	return &runnerv1.LeaseControl{CancelRequested: cancel != nil, LeaseTtlSeconds: leaseTTLSeconds()}
}

func (s *Service) enterPoll(runner uuid.UUID) (func(), error) {
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	if _, busy := s.pollers[runner]; busy {
		return nil, exhausted(CodePollInProgress, config.JobPollRetryAfter)
	}
	if s.polls >= s.maxPolls {
		return nil, exhausted(CodeTooManyPolls, config.JobPollRetryAfter)
	}
	s.polls++
	s.pollers[runner] = struct{}{}
	return func() {
		s.pollMu.Lock()
		defer s.pollMu.Unlock()
		s.polls--
		delete(s.pollers, runner)
	}, nil
}

type claimBucket struct {
	tokens float64
	at     time.Time
}

func (s *Service) allowClaim(runner uuid.UUID) error {
	now := time.Now()
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	b, ok := s.buckets[runner]
	if !ok {
		b = claimBucket{tokens: config.JobClaimBurst, at: now}
	}
	b.tokens = min(config.JobClaimBurst, b.tokens+now.Sub(b.at).Seconds()/config.JobClaimRefill.Seconds())
	b.at = now
	if b.tokens < 1 {
		s.buckets[runner] = b
		return exhausted(CodeClaimRateLimited, config.JobClaimRefill)
	}
	b.tokens--
	s.buckets[runner] = b
	if len(s.buckets) > config.JobClaimTrackedRunners {
		full := time.Duration(config.JobClaimBurst) * config.JobClaimRefill
		for id, other := range s.buckets {
			if now.Sub(other.at) >= full {
				delete(s.buckets, id)
			}
		}
	}
	return nil
}

func fallbackDelay() time.Duration {
	return config.JobPollFallback/2 + rand.N(config.JobPollFallback) //nolint:gosec // Poll jitter needs no cryptographic randomness.
}

func (s *Service) Lease(ctx context.Context, freeSlots, waitSeconds int32) (*runnerv1.SignedLeaseGrant, error) {
	p, err := runnerPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if freeSlots < 0 {
		return nil, runnerauth.ValidationFailed("FreeSlots")
	}
	if waitSeconds < 0 {
		return nil, runnerauth.ValidationFailed("WaitSeconds")
	}
	if freeSlots == 0 {
		return nil, nil
	}
	wait := min(time.Duration(waitSeconds)*time.Second, config.JobMaxPollWait)
	canClaim := !p.Disabled
	if canClaim {
		if err := s.allowClaim(p.RunnerID); err != nil {
			return nil, err
		}
	}
	if wait == 0 {
		if !canClaim {
			return nil, nil
		}
		return s.claim(ctx, p)
	}
	release, err := s.enterPoll(p.RunnerID)
	if err != nil {
		return nil, err
	}
	defer release()
	woken, unsubscribe := s.hub.subscribe(p.OrganizationID)
	defer unsubscribe()
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	fallback := time.NewTimer(fallbackDelay())
	defer fallback.Stop()
	for {
		if canClaim {
			grant, err := s.claim(ctx, p)
			if err != nil || grant != nil {
				return grant, err
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, nil
		case <-s.stop:
			return nil, nil
		case <-woken:
			fallback.Reset(fallbackDelay())
		case <-fallback.C:
			fallback.Reset(fallbackDelay())
		}
	}
}

func (s *Service) claim(ctx context.Context, p runnerauth.Principal) (*runnerv1.SignedLeaseGrant, error) {
	var signed *runnerv1.SignedLeaseGrant
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if err := q.LockRunnerSlots(ctx, p.RunnerID.String()); err != nil {
			return err
		}
		slots, err := q.RunnerSlots(ctx, p.RunnerID)
		if err != nil {
			return err
		}
		if !slots.RunnerActive || slots.ActiveLeases >= int64(p.Capacity) {
			return nil
		}
		job, err := q.ClaimQueuedJob(ctx, sqlc.ClaimQueuedJobParams{
			OrganizationID: p.OrganizationID, Labels: nonNil(p.Labels), IsolationLevels: nonNil(p.IsolationLevels), RunnerID: p.RunnerID,
			Features: nonNil(p.Features),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		signed, err = s.grant(ctx, q, p, job, slots.Now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return signed, nil
}

func (s *Service) grant(ctx context.Context, q *sqlc.Queries, p runnerauth.Principal, job sqlc.Job, now time.Time) (*runnerv1.SignedLeaseGrant, error) {
	spec, err := decodeSpec(job.Spec)
	if err != nil {
		return nil, err
	}
	fencing, err := q.NextFencingToken(ctx)
	if err != nil {
		return nil, err
	}
	attemptID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("new attempt id: %w", err)
	}
	leaseID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("new lease id: %w", err)
	}
	grantBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(&runnerv1.LeaseGrant{
		LeaseId: leaseID.String(), AttemptId: attemptID.String(), JobId: job.Id.String(),
		OrganizationId: job.OrganizationId.String(), RunnerId: p.RunnerID.String(), FencingToken: fencing,
		LeaseTtlSeconds: leaseTTLSeconds(), IssuedAt: timestamppb.New(now), Spec: spec,
	})
	if err != nil {
		return nil, fmt.Errorf("encode lease grant: %w", err)
	}
	if _, err := q.InsertAttempt(ctx, sqlc.InsertAttemptParams{
		ID: attemptID, JobID: job.Id, OrganizationID: job.OrganizationId, RunnerID: p.RunnerID,
		Number: job.AttemptCount + 1, FencingToken: fencing, LogMaxBytes: pgtype.Int8{Int64: s.limits().JobLogMaxBytes, Valid: true},
	}); err != nil {
		return nil, err
	}
	if _, err := q.InsertLease(ctx, sqlc.InsertLeaseParams{
		ID: leaseID, AttemptID: attemptID, JobID: job.Id, OrganizationID: job.OrganizationId, RunnerID: p.RunnerID,
		FencingToken: fencing, GrantDigest: runnersig.GrantDigest(grantBytes), TtlSeconds: config.JobLeaseTTL.Seconds(),
	}); err != nil {
		return nil, err
	}
	if _, err := q.MarkJobLeased(ctx, job.Id); err != nil {
		return nil, err
	}
	return &runnerv1.SignedLeaseGrant{
		GrantBytes: grantBytes, Signature: s.keys.Sign(runnersig.GrantMessage(grantBytes)), KeyVersion: s.keys.SigningKeyVersion(),
	}, nil
}
