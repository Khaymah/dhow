package jobspec

import (
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	RedactedValue    = "[REDACTED]"
	MaxDerivedValues = MaxEnvEntries * (MaxSteps + 1)

	derivedEnv  = "Env"
	derivedWith = "With"
)

type DerivedPath struct {
	Step  int
	Field string
	Name  string
}

func ParseDerived(path string) (DerivedPath, bool) {
	if name, ok := strings.CutPrefix(path, derivedEnv+"."); ok {
		return DerivedPath{Step: -1, Field: derivedEnv, Name: name}, name != ""
	}
	rest, ok := strings.CutPrefix(path, "Steps[")
	if !ok {
		return DerivedPath{}, false
	}
	index, field, ok := strings.Cut(rest, "].")
	i, err := strconv.Atoi(index)
	if !ok || err != nil || i < 0 || strconv.Itoa(i) != index {
		return DerivedPath{}, false
	}
	kind, name, ok := strings.Cut(field, ".")
	if !ok || name == "" || (kind != derivedEnv && kind != derivedWith) {
		return DerivedPath{}, false
	}
	return DerivedPath{Step: i, Field: kind, Name: name}, true
}

func (d DerivedPath) String() string {
	if d.Step < 0 {
		return d.Field + "." + d.Name
	}
	return "Steps[" + strconv.Itoa(d.Step) + "]." + d.Field + "." + d.Name
}

func (d DerivedPath) values(spec *runnerv1.JobSpec) map[string]string {
	switch {
	case d.Step < 0:
		return spec.GetEnv()
	case d.Step >= len(spec.GetSteps()):
		return nil
	case d.Field == derivedEnv:
		return spec.GetSteps()[d.Step].GetEnv()
	default:
		return spec.GetSteps()[d.Step].GetPlugin().GetInputs()
	}
}

func (c *checker) derived(spec *runnerv1.JobSpec) {
	paths := spec.GetDerivedValues()
	if len(paths) > MaxDerivedValues {
		c.add("DerivedValues", ReasonTooLong)
		return
	}
	seen := make(map[string]bool, len(paths))
	for i, p := range paths {
		d, ok := ParseDerived(p)
		if !ok || seen[p] {
			c.add(at("DerivedValues", i), ReasonInvalid)
			continue
		}
		seen[p] = true
		if _, found := d.values(spec)[d.Name]; !found {
			c.add(at("DerivedValues", i), ReasonInvalid)
		}
	}
}

func Redact(spec *runnerv1.JobSpec) *runnerv1.JobSpec {
	out, ok := proto.Clone(spec).(*runnerv1.JobSpec)
	if !ok || out == nil {
		return &runnerv1.JobSpec{}
	}
	for _, p := range out.GetDerivedValues() {
		d, ok := ParseDerived(p)
		if !ok {
			continue
		}
		if values := d.values(out); values != nil {
			if _, found := values[d.Name]; found {
				values[d.Name] = RedactedValue
			}
		}
	}
	return out
}
