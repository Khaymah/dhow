package jobspec

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

type Violation struct {
	Field  string
	Reason string
}

type Error struct {
	Violations []Violation
}

func (e *Error) Error() string {
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, v.Field+": "+v.Reason)
	}
	return "invalid job spec: " + strings.Join(parts, ", ")
}

var ErrNilSpec = errors.New("job spec is required")

func Prepare(spec *runnerv1.JobSpec) (*runnerv1.JobSpec, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}
	out := Normalize(spec)
	if err := Validate(out); err != nil {
		return nil, err
	}
	return out, nil
}

func Normalize(spec *runnerv1.JobSpec) *runnerv1.JobSpec {
	out, ok := proto.Clone(spec).(*runnerv1.JobSpec)
	if !ok || out == nil {
		out = &runnerv1.JobSpec{}
	}
	out.Image = QualifyImage(strings.TrimSpace(out.GetImage()))
	for _, s := range out.GetSteps() {
		if p := s.GetPlugin(); p != nil {
			p.Image = QualifyImage(strings.TrimSpace(p.GetImage()))
		}
	}
	normalizeCheckout(out.GetCheckout())
	if out.GetIsolation() == runnerv1.Isolation_ISOLATION_UNSPECIFIED {
		out.Isolation = runnerv1.Isolation_ISOLATION_CONTAINER
	}
	if out.GetTimeoutSeconds() == 0 {
		out.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if out.Resources == nil {
		out.Resources = &runnerv1.Resources{}
	}
	r := out.GetResources()
	r.CpuMillis = orDefault(r.GetCpuMillis(), DefaultCPUMillis)
	r.MemoryBytes = orDefault(r.GetMemoryBytes(), DefaultMemoryBytes)
	r.Pids = orDefault(r.GetPids(), DefaultPids)
	r.DiskBytes = orDefault(r.GetDiskBytes(), DefaultDiskBytes)
	if out.Network == nil {
		out.Network = &runnerv1.Network{}
	}
	egress := make([]string, 0, len(out.GetNetwork().GetEgress()))
	for _, e := range out.GetNetwork().GetEgress() {
		egress = append(egress, strings.ToLower(strings.TrimSpace(e)))
	}
	out.Network.Egress = sortedUnique(egress)
	out.Labels = sortedUnique(out.GetLabels())
	for _, s := range out.GetSecrets() {
		s.StepIndexes = sortedUniqueInts(s.GetStepIndexes())
	}
	return out
}

func orDefault(v, def int64) int64 {
	if v == 0 {
		return def
	}
	return v
}

func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		return []string{}
	}
	return out
}

func sortedUniqueInts(in []int32) []int32 {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func Digest(spec *runnerv1.JobSpec) ([]byte, error) {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("encode job spec: %w", err)
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}

func SecretNames(spec *runnerv1.JobSpec) []string {
	names := make([]string, 0, len(spec.GetSecrets()))
	for _, s := range spec.GetSecrets() {
		names = append(names, s.GetName())
	}
	return sortedUnique(names)
}

func IsolationName(v runnerv1.Isolation) string {
	switch v {
	case runnerv1.Isolation_ISOLATION_CONTAINER:
		return "container"
	case runnerv1.Isolation_ISOLATION_GVISOR:
		return "gvisor"
	default:
		return ""
	}
}

func ParseIsolation(v string) (runnerv1.Isolation, bool) {
	switch v {
	case "", "container":
		return runnerv1.Isolation_ISOLATION_CONTAINER, true
	case "gvisor":
		return runnerv1.Isolation_ISOLATION_GVISOR, true
	default:
		return runnerv1.Isolation_ISOLATION_UNSPECIFIED, false
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func QualifyImage(image string) string {
	if !imagePattern.MatchString(image) && !imageAnyRef.MatchString(image) {
		return image
	}
	host, rest, found := strings.Cut(image, "/")
	if found && (strings.ContainsAny(host, ".:") || host == "localhost") {
		return image
	}
	if !found {
		return DefaultRegistry + "/" + DefaultNamespace + "/" + image
	}
	return DefaultRegistry + "/" + host + "/" + rest
}
