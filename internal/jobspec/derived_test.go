package jobspec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func withDerived(paths ...string) *runnerv1.JobSpec {
	s := withCheckout()
	s.Env = map[string]string{"CI": "true", "VERSION": "1.2.3"}
	s.Steps[0].Env = map[string]string{"TAG": "1.2.3", "PLAIN": "yes"}
	s.DerivedValues = paths
	return s
}

func TestDerivedValuesMustNameAValueOfTheSpec(t *testing.T) {
	_, err := jobspec.Prepare(withDerived("Env.VERSION", "Steps[0].Env.TAG", "Steps[1].With.channel"))
	require.NoError(t, err)
	for _, bad := range []string{
		"Env.MISSING", "Steps[0].Env.MISSING", "Steps[9].Env.TAG", "Steps[00].Env.TAG", "Steps[-1].Env.TAG",
		"Steps[0].With.TAG", "Steps[0].Run.TAG", "Env.", "Secrets[0]", "",
	} {
		_, err := jobspec.Prepare(withDerived(bad))
		require.Error(t, err, bad)
		assert.Contains(t, violations(t, err), jobspec.Violation{Field: "DerivedValues[0]", Reason: jobspec.ReasonInvalid}, bad)
	}
	_, err = jobspec.Prepare(withDerived("Env.VERSION", "Env.VERSION"))
	assert.Contains(t, violations(t, err), jobspec.Violation{Field: "DerivedValues[1]", Reason: jobspec.ReasonInvalid})
}

func TestRedactHidesEveryDerivedValueAndLeavesTheSpecAlone(t *testing.T) {
	s := withDerived("Env.VERSION", "Steps[0].Env.TAG", "Steps[1].With.channel")
	out := jobspec.Redact(s)
	assert.Equal(t, jobspec.RedactedValue, out.GetSteps()[1].GetPlugin().GetInputs()["channel"])
	assert.Equal(t, "#ops", s.GetSteps()[1].GetPlugin().GetInputs()["channel"])
	assert.Equal(t, map[string]string{"CI": "true", "VERSION": jobspec.RedactedValue}, out.GetEnv())
	assert.Equal(t, map[string]string{"TAG": jobspec.RedactedValue, "PLAIN": "yes"}, out.GetSteps()[0].GetEnv())
	assert.Equal(t, "1.2.3", s.GetEnv()["VERSION"], "the stored spec keeps its value")
	assert.Equal(t, "1.2.3", s.GetSteps()[0].GetEnv()["TAG"])
}
