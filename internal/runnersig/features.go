package runnersig

import (
	"errors"
	"slices"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	AuthFeaturesLabel = "dhow/runner-auth/v2"
	MaxFeatures       = 8

	FeatureCheckout  = "checkout"
	FeaturePlugins   = "plugins"
	FeatureArtifacts = "artifacts"
	FeatureCache     = "cache"
	FeatureRegistry  = "registry_auth"
	FeatureDeploy    = "deploy_targets"
)

var ErrFeature = errors.New("the runner declared an unknown or repeated feature")

func featureName(f runnerv1.RunnerFeature) string {
	switch f {
	case runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT:
		return FeatureCheckout
	case runnerv1.RunnerFeature_RUNNER_FEATURE_PLUGINS:
		return FeaturePlugins
	case runnerv1.RunnerFeature_RUNNER_FEATURE_ARTIFACTS:
		return FeatureArtifacts
	case runnerv1.RunnerFeature_RUNNER_FEATURE_CACHE:
		return FeatureCache
	case runnerv1.RunnerFeature_RUNNER_FEATURE_REGISTRY_AUTH:
		return FeatureRegistry
	case runnerv1.RunnerFeature_RUNNER_FEATURE_DEPLOY_TARGETS:
		return FeatureDeploy
	default:
		return ""
	}
}

func FeatureNames(features []runnerv1.RunnerFeature) ([]string, error) {
	if len(features) > MaxFeatures {
		return nil, ErrFeature
	}
	out := make([]string, 0, len(features))
	for _, f := range features {
		name := featureName(f)
		if name == "" || slices.Contains(out, name) {
			return nil, ErrFeature
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out, nil
}

func AuthFeaturesMessage(origin string, serverKeyVersion int32, challenge []byte, features []string) []byte {
	b := newBuilder(AuthFeaturesLabel)
	b.bytes([]byte(origin))
	b.int32(serverKeyVersion)
	b.bytes(challenge)
	b.int32(int32(len(features))) //nolint:gosec // At most MaxFeatures.
	for _, f := range features {
		b.bytes([]byte(f))
	}
	return b.buf
}
