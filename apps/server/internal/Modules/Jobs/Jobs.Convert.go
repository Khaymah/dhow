package jobs

import (
	"encoding/hex"
	"fmt"
	"math"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type specReader struct {
	details []apperrors.IErrorDetail
}

func (r *specReader) int32(field string, v *int) int32 {
	if v == nil {
		return 0
	}
	if *v < math.MinInt32 || *v > math.MaxInt32 {
		r.details = append(r.details, invalid(field))
		return 0
	}
	return int32(*v)
}

func (r *specReader) int64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func specFromGen(in gen.JobSpec) (*runnerv1.JobSpec, error) {
	r := &specReader{}
	out := &runnerv1.JobSpec{Image: in.Image, TimeoutSeconds: r.int32("Spec.TimeoutSeconds", in.TimeoutSeconds)}
	if in.Env != nil {
		out.Env = *in.Env
	}
	if in.Isolation != nil {
		iso, ok := jobspec.ParseIsolation(string(*in.Isolation))
		if !ok {
			r.details = append(r.details, invalid("Spec.Isolation"))
		}
		out.Isolation = iso
	}
	if in.Labels != nil {
		out.Labels = *in.Labels
	}
	if in.Network != nil && in.Network.Egress != nil {
		out.Network = &runnerv1.Network{Egress: *in.Network.Egress}
	}
	if in.Resources != nil {
		out.Resources = &runnerv1.Resources{
			CpuMillis: r.int64(in.Resources.CpuMillis), MemoryBytes: r.int64(in.Resources.MemoryBytes),
			Pids: r.int64(in.Resources.Pids), DiskBytes: r.int64(in.Resources.DiskBytes),
		}
	}
	for i, st := range in.Steps {
		step := &runnerv1.Step{Name: st.Name, Run: st.Run, TimeoutSeconds: r.int32(fmt.Sprintf("Spec.Steps[%d].TimeoutSeconds", i), st.TimeoutSeconds)}
		if st.Env != nil {
			step.Env = *st.Env
		}
		if st.Mutating != nil {
			step.Mutating = *st.Mutating
		}
		out.Steps = append(out.Steps, step)
	}
	if in.Secrets != nil {
		for i, sec := range *in.Secrets {
			out.Secrets = append(out.Secrets, r.secret(i, sec))
		}
	}
	if len(r.details) > 0 {
		return nil, apperrors.ValidationFailed(r.details...)
	}
	return out, nil
}

func (r *specReader) secret(i int, sec gen.JobSecretRef) *runnerv1.SecretRef {
	field := fmt.Sprintf("Spec.Secrets[%d]", i)
	ref := &runnerv1.SecretRef{Name: sec.Name}
	switch {
	case sec.AsEnv != nil && sec.AsFile != nil:
		r.details = append(r.details, invalid(field))
	case sec.AsEnv != nil:
		ref.Target = &runnerv1.SecretRef_AsEnv{AsEnv: *sec.AsEnv}
	case sec.AsFile != nil:
		ref.Target = &runnerv1.SecretRef_AsFile{AsFile: *sec.AsFile}
	}
	if sec.Steps != nil {
		for _, idx := range *sec.Steps {
			v := idx
			ref.StepIndexes = append(ref.StepIndexes, r.int32(field+".Steps", &v))
		}
	}
	return ref
}

func ptrOf[T any](v T) *T { return &v }

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func specToGen(stored *runnerv1.JobSpec) gen.JobSpec {
	s := jobspec.Redact(stored)
	out := gen.JobSpec{
		Image: s.GetImage(), TimeoutSeconds: ptrOf(int(s.GetTimeoutSeconds())), Labels: ptrOf(nonNil(s.GetLabels())),
		Isolation: ptrOf(gen.JobIsolation(jobspec.IsolationName(s.GetIsolation()))), Env: ptrOf(mapOf(s.GetEnv())),
		Network: &gen.JobNetwork{Egress: ptrOf(nonNil(s.GetNetwork().GetEgress()))},
		Resources: &gen.JobResources{
			CpuMillis: ptrOf(s.GetResources().GetCpuMillis()), MemoryBytes: ptrOf(s.GetResources().GetMemoryBytes()),
			Pids: ptrOf(s.GetResources().GetPids()), DiskBytes: ptrOf(s.GetResources().GetDiskBytes()),
		},
		Steps: make([]gen.JobStep, 0, len(s.GetSteps())),
	}
	for _, st := range s.GetSteps() {
		out.Steps = append(out.Steps, gen.JobStep{
			Name: st.GetName(), Run: st.GetRun(), Env: ptrOf(mapOf(st.GetEnv())), Mutating: ptrOf(st.GetMutating()),
			TimeoutSeconds: ptrOf(int(st.GetTimeoutSeconds())),
		})
	}
	secrets := make([]gen.JobSecretRef, 0, len(s.GetSecrets()))
	for _, sec := range s.GetSecrets() {
		ref := gen.JobSecretRef{Name: sec.GetName()}
		switch t := sec.GetTarget().(type) {
		case *runnerv1.SecretRef_AsEnv:
			ref.AsEnv = ptrOf(t.AsEnv)
		case *runnerv1.SecretRef_AsFile:
			ref.AsFile = ptrOf(t.AsFile)
		}
		steps := make([]int, 0, len(sec.GetStepIndexes()))
		for _, idx := range sec.GetStepIndexes() {
			steps = append(steps, int(idx))
		}
		ref.Steps = &steps
		secrets = append(secrets, ref)
	}
	out.Secrets = &secrets
	return out
}

func mapOf(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func jobToGen(j models.Job) gen.Job {
	out := gen.Job{
		Id: j.ID, OrganizationId: j.OrganizationID, Name: j.Name, State: gen.JobState(j.State), Labels: nonNil(j.Labels),
		Isolation: gen.JobIsolation(j.Isolation), Priority: j.Priority, Image: j.Image, SpecDigest: hex.EncodeToString(j.SpecDigest),
		AttemptCount: j.AttemptCount, MaxInfraRetries: j.MaxInfraRetries, QueuedAt: j.QueuedAt, StartedAt: j.StartedAt,
		FinishedAt: j.FinishedAt, Deadline: j.Deadline, QueueDeadline: j.QueueDeadline, CancelRequestedAt: j.CancelRequestedAt,
		CreatedAt: j.CreatedAt, CreatedByUserId: j.CreatedByUserID,
	}
	if j.FailureReason != "" {
		out.FailureReason = ptrOf(j.FailureReason)
	}
	if j.QueuedReason != "" {
		out.QueuedReason = ptrOf(gen.JobQueuedReason(j.QueuedReason))
	}
	return out
}

func detailToGen(d models.Detail) gen.JobDetail {
	out := gen.JobDetail{Job: jobToGen(d.Job), Spec: specToGen(d.Spec), Attempts: make([]gen.JobAttempt, 0, len(d.Attempts))}
	for _, a := range d.Attempts {
		attempt := gen.JobAttempt{
			Id: a.ID, Number: a.Number, RunnerId: a.RunnerID, FencingToken: a.FencingToken, State: gen.JobAttemptState(a.State),
			ExitCode: a.ExitCode,
			LeasedAt: a.LeasedAt, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt, MutationStartedAt: a.MutationStartedAt,
			Steps: make([]gen.JobAttemptStep, 0, len(a.Steps)),
		}
		if a.FailureReason != "" {
			attempt.FailureReason = ptrOf(a.FailureReason)
		}
		for _, s := range a.Steps {
			step := gen.JobAttemptStep{
				Index: s.Index, Name: s.Name, Mutating: s.Mutating, State: gen.JobStepState(s.State), ExitCode: s.ExitCode,
				StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
			}
			if s.FailureReason != "" {
				step.FailureReason = ptrOf(s.FailureReason)
			}
			attempt.Steps = append(attempt.Steps, step)
		}
		out.Attempts = append(out.Attempts, attempt)
	}
	return out
}
