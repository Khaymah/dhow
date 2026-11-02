package e2e_test

import (
	"context"
	"crypto/ed25519"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var checkoutAndPlugins = []runnerv1.RunnerFeature{runnerv1.RunnerFeature_RUNNER_FEATURE_PLUGINS, runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT}

func (w world) features(t *testing.T, runnerID string) []string {
	t.Helper()
	var out []string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(), `SELECT "Features" FROM "Runners" WHERE "Id" = $1`, runnerID).Scan(&out))
	return out
}

func authenticateWith(rc *apptest.RunnerClient, runnerID string, c *runnerv1.IssuedChallenge, sig []byte, features []runnerv1.RunnerFeature) error {
	_, err := rc.Auth.Authenticate(context.Background(), connect.NewRequest(&runnerv1.AuthenticateRequest{
		Runner: &runnerv1.RunnerRef{RunnerId: runnerID}, Challenge: c, Signature: sig, Features: features,
	}))
	return err
}

func TestRegistrationStoresTheRunnerFeatures(t *testing.T) {
	w := newWorld(t)
	token, _ := w.env.CreateRegistrationToken(w.owner, w.org, apptest.RegistrationTokenSpec{})
	id := apptest.NewRunnerIdentity(t)
	req := id.RegisterRequest(token)
	req.Features = checkoutAndPlugins
	out, err := w.env.RunnerClient(apptest.RunnerClientOptions{}).Registration.Register(context.Background(), connect.NewRequest(req))
	require.NoError(t, err)
	assert.Equal(t, []string{runnersig.FeatureCheckout, runnersig.FeaturePlugins}, w.features(t, out.Msg.GetRunnerId()))

	token, _ = w.env.CreateRegistrationToken(w.owner, w.org, apptest.RegistrationTokenSpec{})
	bad := apptest.NewRunnerIdentity(t).RegisterRequest(token)
	bad.Features = []runnerv1.RunnerFeature{runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT, runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT}
	_, err = w.env.RunnerClient(apptest.RunnerClientOptions{}).Registration.Register(context.Background(), connect.NewRequest(bad))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr(t, err).Code())
}

func TestSignedAuthenticationSetsTheFeaturesAndNothingElseCan(t *testing.T) {
	w := newWorld(t)
	id, reg := w.registerOK(t, apptest.RegistrationTokenSpec{})
	runnerID := reg.GetRunnerId()
	rc := w.env.RunnerClient(apptest.RunnerClientOptions{})
	assert.Empty(t, w.features(t, runnerID), "an M3 runner declares nothing")

	names := []string{runnersig.FeatureCheckout, runnersig.FeaturePlugins}
	c := challenge(t, rc, runnerID)
	require.NoError(t, authenticateWith(rc, runnerID, c, id.SignWithFeatures(c, names), checkoutAndPlugins))
	assert.Equal(t, names, w.features(t, runnerID))

	forgeries := []struct {
		name string
		sig  func(*runnerv1.IssuedChallenge) []byte
		want []runnerv1.RunnerFeature
	}{
		{"an M3 signature with features added", id.Sign, []runnerv1.RunnerFeature{runnerv1.RunnerFeature_RUNNER_FEATURE_CACHE}},
		{"a signature over fewer features", func(c *runnerv1.IssuedChallenge) []byte {
			return id.SignWithFeatures(c, []string{runnersig.FeatureCheckout})
		}, checkoutAndPlugins},
		{"a signature over another origin", func(c *runnerv1.IssuedChallenge) []byte {
			return ed25519.Sign(id.Signing, runnersig.AuthFeaturesMessage("https://evil.example", 1, c.GetChallengeBytes(), names))
		}, checkoutAndPlugins},
		{"an unknown feature", func(c *runnerv1.IssuedChallenge) []byte { return id.SignWithFeatures(c, names) },
			[]runnerv1.RunnerFeature{runnerv1.RunnerFeature(42)}},
	}
	for _, f := range forgeries {
		t.Run(f.name, func(t *testing.T) {
			c := challenge(t, rc, runnerID)
			requireRPCError(t, authenticateWith(rc, runnerID, c, f.sig(c), f.want), connect.CodeUnauthenticated, "INVALID_RUNNER_CREDENTIALS")
			assert.Equal(t, names, w.features(t, runnerID))
		})
	}

	_, err := w.env.Authenticate(rc, id, runnerID)
	require.NoError(t, err)
	assert.Empty(t, w.features(t, runnerID), "a runner that signs in as M3 again loses the features")
}
