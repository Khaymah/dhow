package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheSshPluginDeclaresAReasonForEveryExitCodeItHas(t *testing.T) {
	m := shippedManifest(t)
	want := map[int32]string{
		1: "DEPLOY_FAILED", 2: "PLUGIN_INPUT_INVALID", 10: "DEPLOY_ROLLED_BACK", 11: "DEPLOY_HEALTH_FAILED", 12: "DEPLOY_ROLLBACK_FAILED",
		13: "HOST_KEY_MISMATCH", 14: "ARTIFACT_UNSAFE", 15: "TARGET_LOCKED", 16: "CURRENT_NOT_A_SYMLINK", 17: "TARGET_HOST_UNREACHABLE",
		18: "TARGET_STATE_CORRUPT", 19: "TARGET_CHECK_FAILED",
	}
	for code, name := range want {
		got, ok := m.ExitReason(code)
		assert.True(t, ok, code)
		assert.Equal(t, name, got, code)
	}
	_, ok := m.ExitReason(137)
	assert.False(t, ok, "a code the plugin does not declare stays a failed step")
	assert.False(t, m.Inputs["artifact"].Required, "a check needs no artifact")
	assert.Equal(t, "deploy", *m.Inputs["mode"].Default)
}

func TestAManifestOnlyMapsExitCodesToTheClosedListOfDeployReasons(t *testing.T) {
	for _, bad := range []string{
		`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"ExitReasons":{"10":"OOM_KILLED"}}`,
		`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"ExitReasons":{"10":"CANCELED"}}`,
		`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"ExitReasons":{"0":"DEPLOY_FAILED"}}`,
		`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"ExitReasons":{"256":"DEPLOY_FAILED"}}`,
		`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"ExitReasons":{"010":"DEPLOY_FAILED"}}`,
		`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"ExitReasons":{"x":"DEPLOY_FAILED"}}`,
		`{"Version":1,"Entrypoint":["/x"],"ExitReasons":{"10":"DEPLOY_FAILED"}}`,
	} {
		_, err := Parse(bad)
		assert.ErrorIs(t, err, ErrManifest, bad)
	}
	m, err := Parse(`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"ExitReasons":{"10":"DEPLOY_ROLLED_BACK"}}`)
	require.NoError(t, err)
	name, ok := m.ExitReason(10)
	assert.True(t, ok)
	assert.Equal(t, "DEPLOY_ROLLED_BACK", name)
}

func TestAnArtifactInputMayBeOptionalOnlyForAMutatingPlugin(t *testing.T) {
	_, err := Parse(`{"Version":1,"Entrypoint":["/x"],"Mutating":true,"Inputs":{"a":{"Kind":"artifact"}}}`)
	assert.NoError(t, err)
	_, err = Parse(`{"Version":1,"Entrypoint":["/x"],"Inputs":{"a":{"Kind":"artifact"}}}`)
	assert.ErrorIs(t, err, ErrManifest)
}
