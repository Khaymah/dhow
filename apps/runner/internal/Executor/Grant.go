package executor

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var (
	ErrGrantMissing      = errors.New("the lease response has no grant")
	ErrGrantKeyVersion   = errors.New("the grant is signed with a server key version this runner did not pin")
	ErrGrantSignature    = errors.New("the grant signature does not verify against the pinned server key")
	ErrGrantMalformed    = errors.New("the grant does not decode")
	ErrGrantRunner       = errors.New("the grant names another runner")
	ErrGrantOrganization = errors.New("the grant names another organization")
	ErrGrantFields       = errors.New("the grant has invalid lease, attempt or job ids, or a non-positive fencing token")
	ErrGrantTTL          = errors.New("the grant has a lease TTL outside what the runner can fence safely")
	ErrGrantStale        = errors.New("the grant was issued too far from now, so it may be a replay")
)

func VerifyGrant(id *identity.Identity, signed *runnerv1.SignedLeaseGrant, now time.Time) (*runnerv1.LeaseGrant, error) {
	if signed == nil || len(signed.GetGrantBytes()) == 0 {
		return nil, ErrGrantMissing
	}
	if signed.GetKeyVersion() != id.ServerKeyVersion {
		return nil, ErrGrantKeyVersion
	}
	if !id.VerifyServer(runnersig.GrantMessage(signed.GetGrantBytes()), signed.GetSignature()) {
		return nil, ErrGrantSignature
	}
	var g runnerv1.LeaseGrant
	if err := proto.Unmarshal(signed.GetGrantBytes(), &g); err != nil {
		return nil, errors.Join(ErrGrantMalformed, err)
	}
	if g.GetRunnerId() != id.RunnerID {
		return nil, ErrGrantRunner
	}
	if g.GetOrganizationId() != id.OrganizationID {
		return nil, ErrGrantOrganization
	}
	for _, v := range []string{g.GetLeaseId(), g.GetAttemptId(), g.GetJobId()} {
		if _, err := uuid.Parse(v); err != nil {
			return nil, ErrGrantFields
		}
	}
	if g.GetFencingToken() < 1 || g.GetSpec() == nil {
		return nil, ErrGrantFields
	}
	if ttl := time.Duration(g.GetLeaseTtlSeconds()) * time.Second; ttl < config.MinLeaseTTL || ttl > config.MaxLeaseTTL {
		return nil, ErrGrantTTL
	}
	if g.GetIssuedAt() == nil || g.GetIssuedAt().CheckValid() != nil {
		return nil, ErrGrantStale
	}
	if age := now.Sub(g.GetIssuedAt().AsTime()); age > config.GrantMaxSkew || age < -config.GrantMaxSkew {
		return nil, ErrGrantStale
	}
	return &g, nil
}

func preflight(spec *runnerv1.JobSpec, cfg config.Config, levels []runnerv1.Isolation) (runnerv1.FailureReason, string) {
	if err := jobspec.Validate(spec); err != nil {
		return runnerv1.FailureReason_FAILURE_REASON_SPEC_REJECTED, err.Error()
	}
	if spec.GetCheckout() != nil && cfg.CheckoutImage == "" {
		return runnerv1.FailureReason_FAILURE_REASON_SPEC_REJECTED, "this runner has no checkout image, so it cannot run a job with a checkout"
	}
	r := spec.GetResources()
	l := cfg.Limits
	timeout := time.Duration(spec.GetTimeoutSeconds()) * time.Second
	if r.GetCpuMillis() > l.CPUMillis || r.GetMemoryBytes() > l.MemoryBytes || r.GetPids() > l.Pids ||
		r.GetDiskBytes() > l.DiskBytes || timeout > l.Timeout {
		return runnerv1.FailureReason_FAILURE_REASON_RESOURCE_REQUEST_EXCEEDS_RUNNER,
			"the job asks for more CPU, memory, processes, disk or time than this runner allows"
	}
	for _, level := range levels {
		if level == spec.GetIsolation() {
			return runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED, ""
		}
	}
	return runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, "this runner does not offer the isolation level the job asks for"
}
