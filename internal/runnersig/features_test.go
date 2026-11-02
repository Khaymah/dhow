package runnersig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestFeatureNames(t *testing.T) {
	got, err := FeatureNames([]runnerv1.RunnerFeature{runnerv1.RunnerFeature_RUNNER_FEATURE_PLUGINS, runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT})
	require.NoError(t, err)
	assert.Equal(t, []string{FeatureCheckout, FeaturePlugins}, got)
	got, err = FeatureNames(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	for _, bad := range [][]runnerv1.RunnerFeature{
		{runnerv1.RunnerFeature_RUNNER_FEATURE_UNSPECIFIED},
		{runnerv1.RunnerFeature(99)},
		{runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT, runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT},
	} {
		_, err := FeatureNames(bad)
		assert.ErrorIs(t, err, ErrFeature)
	}
}

func TestAuthFeaturesMessageBindsTheFeatures(t *testing.T) {
	a := AuthFeaturesMessage("https://dhow.example.com", 1, []byte("challenge"), []string{FeatureCheckout})
	b := AuthFeaturesMessage("https://dhow.example.com", 1, []byte("challenge"), []string{FeatureCheckout, FeaturePlugins})
	c := AuthMessage("https://dhow.example.com", 1, []byte("challenge"))
	assert.NotEqual(t, a, b)
	assert.NotEqual(t, a, c)
}
