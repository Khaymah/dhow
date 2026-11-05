package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deployLabel = `{"Version":1,"Entrypoint":["/deploy"],"Mutating":true,"Inputs":{"target":{"Required":true},"artifact":{"Required":true,"Kind":"artifact"},"strategy":{"Kind":"value","Default":"atomic-symlink"}}}`

func TestAManifestDeclaresMutatingAndArtifactInputs(t *testing.T) {
	m, err := Parse(deployLabel)
	require.NoError(t, err)
	assert.True(t, m.Mutating)
	assert.True(t, m.Inputs["artifact"].IsArtifact())
	assert.False(t, m.Inputs["strategy"].IsArtifact())
	assert.Equal(t, map[string]string{"artifact": "site.tgz"}, m.ArtifactNames(map[string]string{"artifact": "site.tgz", "target": "prod"}))
	plain, err := Parse(good)
	require.NoError(t, err)
	assert.False(t, plain.Mutating, "a manifest without the field is not mutating")
}

func TestAManifestRefusesBadInputKinds(t *testing.T) {
	for _, bad := range []string{
		`{"Version":1,"Entrypoint":["/x"],"Inputs":{"a":{"Kind":"file"}}}`,
		`{"Version":1,"Entrypoint":["/x"],"Inputs":{"a":{"Kind":"artifact"}}}`,
		`{"Version":1,"Entrypoint":["/x"],"Inputs":{"a":{"Kind":"artifact","Required":true,"Default":"x"}}}`,
		`{"Version":1,"Entrypoint":["/x"],"Mutating":"yes"}`,
	} {
		_, err := Parse(bad)
		assert.ErrorIs(t, err, ErrManifest, bad)
	}
}
