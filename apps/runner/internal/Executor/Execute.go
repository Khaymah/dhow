package executor

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	logs "github.com/Khaymah/dhow/apps/runner/internal/Logs"
	masking "github.com/Khaymah/dhow/apps/runner/internal/Masking"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var (
	ErrSecretsMissing = errors.New("the server sent no secrets for a job that references some")
	ErrSecretsKEM     = errors.New("the sealed secrets use an unknown KEM")
	ErrSecretsBundle  = errors.New("the sealed secrets do not decode")
	ErrSecretTooLarge = fmt.Errorf("a secret is larger than %d bytes, or all of them together are larger than %d", config.MaxSecretBytes, config.MaxSecretsBytes)
)

func reasonName(r runnerv1.FailureReason) string {
	const prefix = len("FAILURE_REASON_")
	if name := r.String(); len(name) > prefix {
		return name[prefix:]
	}
	return r.String()
}

func (a *attempt) execute(ctx context.Context) result {
	spec := a.grant.GetSpec()
	if reason, why := preflight(spec, a.e.o.Config, a.e.o.Driver.Isolation()); reason != runnerv1.FailureReason_FAILURE_REASON_UNSPECIFIED {
		a.pipe.Systemf(nil, "dhow: %s: %s", reasonName(reason), why)
		return failed(reason, 0)
	}
	segs := segments(spec, a.e.o.Config.CheckoutImage)
	if r, ok := a.openWorkspace(ctx); !ok {
		return r
	}
	if r, ok := a.prepare(ctx, 0, segs[0]); !ok {
		return r
	}
	if r, ok := a.start(ctx); !ok {
		return r
	}
	return a.runSegments(ctx, segs)
}

func (a *attempt) lostResult() result {
	if errors.Is(a.fence.LostErr(), ErrSelfFenced) {
		return lost(runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST)
	}
	return gone()
}

type prepared struct {
	box sandbox.Sandbox
	err error
}

func (a *attempt) prepare(ctx context.Context, seq int, seg segment) (result, bool) {
	spec := a.grant.GetSpec()
	if r, ok := a.checkWorkspace(ctx); !ok {
		return r, false
	}
	a.pipe.Systemf(nil, "dhow: preparing the sandbox for %s", seg.image)
	pctx, cancel := context.WithTimeout(ctx, a.e.o.Timing.ImagePull)
	defer cancel()
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	ch := make(chan prepared, 1)
	go func() {
		r := spec.GetResources()
		box, err := a.e.o.Driver.Prepare(pctx, sandbox.PrepareSpec{
			RunnerID: a.e.o.Identity.RunnerID, AttemptID: a.grant.GetAttemptId(), Seq: seq, Kind: seg.kind, Image: seg.image,
			Isolation: spec.GetIsolation(),
			Resources: sandbox.Resources{CPUMillis: r.GetCpuMillis(), MemoryBytes: r.GetMemoryBytes(), Pids: r.GetPids(), DiskBytes: r.GetDiskBytes()},
			Dir:       a.dir, DiskMode: a.e.o.Config.DiskMode, Egress: seg.egress, Workspace: ws,
		})
		ch <- prepared{box: box, err: err}
	}()
	var why result
	select {
	case p := <-ch:
		return a.prepared(ctx, pctx, p, seg.egress)
	case <-a.fence.Lost():
		why = a.lostResult()
	case <-a.fence.Canceled():
		why = canceledResult()
	case <-a.shutdown:
		why = lost(runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED)
	}
	cancel()
	if p := <-ch; p.err == nil && p.box != nil {
		a.setBox(ctx, p.box)
	}
	return why, false
}

func (a *attempt) prepared(ctx, pctx context.Context, p prepared, allow []string) (result, bool) {
	if p.err != nil {
		reason := runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED
		if errors.Is(p.err, sandbox.ErrImagePull) || errors.Is(pctx.Err(), context.DeadlineExceeded) {
			reason = runnerv1.FailureReason_FAILURE_REASON_IMAGE_PULL_FAILED
		}
		a.log.Error("could not prepare the sandbox", "error", a.masked(p.err))
		a.pipe.Systemf(nil, "dhow: %s: the sandbox could not be prepared", reasonName(reason))
		return failed(reason, 0), false
	}
	if !a.setBox(ctx, p.box) {
		return a.lostResult(), false
	}
	if err := a.startEgress(ctx, p.box, allow); err != nil {
		a.log.Error("could not start the egress proxy", "error", a.masked(err))
		a.pipe.Systemf(nil, "dhow: %s: the egress proxy could not start", reasonName(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED))
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	return result{}, true
}

func (a *attempt) start(ctx context.Context) (result, bool) {
	spec := a.grant.GetSpec()
	proof := a.e.o.Identity.Sign(runnersig.StartMessage(a.ref.GetLeaseId(), a.ref.GetFencingToken(), runnersig.GrantDigest(a.grantBytes)))
	var res *runnerv1.StartResponse
	retried := false
	err := a.retry(ctx, func(rctx context.Context) error {
		sctx, cancel := context.WithDeadline(rctx, a.fence.Deadline())
		defer cancel()
		sent := a.fence.now()
		r, err := a.e.o.API.Start(sctx, a.ref, proof)
		if err == nil {
			a.fence.extend(sent, r.GetControl())
			res = r
		} else if client.Transient(err) {
			retried = true
		}
		return err
	})
	switch {
	case err == nil:
	case client.Is(err, client.CodeCancelRequested):
		return canceledResult(), false
	case errors.Is(err, ErrSelfFenced):
		return lost(runnerv1.FailureReason_FAILURE_REASON_LEASE_LOST), false
	case client.IsLeaseLost(err), errors.Is(err, ErrLeaseLost):
		return gone(), false
	case client.Is(err, client.CodeSecretsUnavailable):
		a.log.Warn("the server could not provide the job secrets and closed the attempt")
		return gone(), false
	case client.Is(err, client.CodeAlreadyStarted) && retried && len(spec.GetSecrets()) == 0:
		a.log.Info("the attempt was already started by an earlier try whose answer was lost")
	case a.stopping():
		return lost(runnerv1.FailureReason_FAILURE_REASON_RUNNER_RESTARTED), false
	default:
		a.log.Error("the server refused to start the attempt", "code", client.Code(err).String(), "reason", client.Reason(err))
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	if a.fenced() {
		return a.lostResult(), false
	}
	if err := a.openSecrets(res.GetSecrets()); errors.Is(err, ErrSecretTooLarge) {
		a.log.Error("a job secret is too large to mask, so the job does not run")
		return failed(runnerv1.FailureReason_FAILURE_REASON_SPEC_REJECTED, 0), false
	} else if err != nil {
		a.log.Error("could not open the job secrets", "error", a.masked(err))
		a.pipe.Systemf(nil, "dhow: SANDBOX_FAILED: the job secrets could not be opened")
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	if a.fenced() {
		return a.lostResult(), false
	}
	if err := a.currentBox().Start(ctx); err != nil {
		a.log.Error("could not start the sandbox", "error", a.masked(err))
		return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0), false
	}
	return result{}, true
}

func (a *attempt) fenced() bool {
	select {
	case <-a.fence.Lost():
		return true
	default:
		return false
	}
}

func (a *attempt) currentBox() sandbox.Sandbox {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.box
}

func (a *attempt) openSecrets(sealed *runnerv1.SealedSecrets) error {
	refs := a.grant.GetSpec().GetSecrets()
	if len(refs) == 0 {
		return nil
	}
	if sealed == nil {
		return ErrSecretsMissing
	}
	if sealed.GetKem() != runnerv1.HpkeKem_HPKE_KEM_MLKEM768_X25519 {
		return ErrSecretsKEM
	}
	aad := runnersig.SecretsAAD(a.ref.GetLeaseId(), a.ref.GetFencingToken(), a.grant.GetAttemptId(), a.e.o.Identity.RunnerID)
	plain, err := runnersig.OpenSecrets(a.e.o.Identity.Encryption, sealed.GetEnc(), aad, sealed.GetCiphertext())
	if err != nil {
		return err
	}
	defer clear(plain)
	var bundle runnerv1.SecretBundle
	if err := proto.Unmarshal(plain, &bundle); err != nil {
		return ErrSecretsBundle
	}
	values := make(map[string][]byte, len(bundle.GetSecrets()))
	for _, s := range bundle.GetSecrets() {
		values[s.GetName()] = append([]byte(nil), s.GetValue()...)
		clear(s.Value)
	}
	total := 0
	batch := make([][]byte, 0, len(refs))
	for _, ref := range refs {
		v, ok := values[ref.GetName()]
		if !ok {
			return ErrSecretsMissing
		}
		total += len(v)
		if len(v) > config.MaxSecretBytes || total > config.MaxSecretsBytes {
			a.pipe.Systemf(nil, "dhow: SPEC_REJECTED: the secret %s is too large to mask (at most %d bytes each, %d together)",
				ref.GetName(), config.MaxSecretBytes, config.MaxSecretsBytes)
			return ErrSecretTooLarge
		}
		batch = append(batch, v)
	}
	a.scan.AddAll(batch)
	for i, added := range a.masker.AddAll(batch) {
		if !added {
			a.pipe.Systemf(nil, "dhow: warning: the secret %s is shorter than %d characters, so the log does not hide it",
				refs[i].GetName(), masking.MinLength)
		}
	}
	a.secrets = values
	return nil
}

func (a *attempt) fatalResult() result {
	err := a.pipe.Err()
	if errors.Is(err, logs.ErrLeaseLost) {
		a.fence.markLost(ErrLeaseLost)
		return gone()
	}
	a.log.Error("the log pipeline failed, so the job stops", "error", a.masked(err))
	if errors.Is(err, logs.ErrDiskFull) {
		return failed(runnerv1.FailureReason_FAILURE_REASON_RUNNER_DISK_FULL, 0)
	}
	return failed(runnerv1.FailureReason_FAILURE_REASON_SANDBOX_FAILED, 0)
}
