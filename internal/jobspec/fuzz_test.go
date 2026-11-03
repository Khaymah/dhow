package jobspec_test

import (
	"bytes"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func FuzzPrepare(f *testing.F) {
	for _, spec := range []*runnerv1.JobSpec{
		valid(),
		{Image: "busybox", Steps: []*runnerv1.Step{{Run: "true"}}},
		{Image: image, Steps: []*runnerv1.Step{{Plugin: &runnerv1.Plugin{Image: image}}}, Network: &runnerv1.Network{Egress: []string{"*.example.com"}}},
		{Image: image, Steps: []*runnerv1.Step{{Run: "x"}}, DerivedValues: []string{"env.X", "steps.0.env.Y"}},
	} {
		raw, err := proto.Marshal(spec)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		spec := &runnerv1.JobSpec{}
		if proto.Unmarshal(raw, spec) != nil {
			return
		}
		before := proto.Clone(spec)
		out, err := jobspec.Prepare(spec)
		if !proto.Equal(before, spec) {
			t.Fatal("Prepare changed its input")
		}
		if err != nil {
			var invalid *jobspec.Error
			if !errors.As(err, &invalid) || len(invalid.Violations) == 0 {
				t.Fatalf("Prepare failed without a violation: %v", err)
			}
			return
		}
		again, err := jobspec.Prepare(out)
		if err != nil {
			t.Fatalf("a prepared spec is refused when prepared again: %v", err)
		}
		first, err := jobspec.Digest(out)
		if err != nil {
			t.Fatal(err)
		}
		second, err := jobspec.Digest(again)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatal("preparing a prepared spec changed its digest")
		}
		kept := proto.Clone(out)
		_ = jobspec.Redact(out)
		if !proto.Equal(kept, out) {
			t.Fatal("Redact changed its input")
		}
	})
}
