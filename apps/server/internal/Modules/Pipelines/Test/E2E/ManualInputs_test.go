package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
)

type manualInputView struct {
	Name, Type  string
	Description *string
	Required    bool
	Default     *string
	Options     []string
}

type manualInputsView struct {
	Known, Enabled bool
	Ref            string
	Sha            *string
	Inputs         []manualInputView
}

func (w world) manualInputs(t *testing.T, c *apptest.Client) manualInputsView {
	t.Helper()
	res := c.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/manual-inputs"))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.Equal(t, "pipelines.manualInputs.loaded", res.Message)
	var v manualInputsView
	res.Decode(t, &v)
	return v
}

func TestManualInputsComeFromTheLatestFileOfTheDefaultBranch(t *testing.T) {
	w := newWorld(t)
	before := w.manualInputs(t, w.owner)
	assert.False(t, before.Known)
	assert.False(t, before.Enabled)
	assert.Equal(t, "refs/heads/main", before.Ref)
	assert.Empty(t, before.Inputs)

	w.defs.Put(shaA, pipeline(simplePipeline))
	w.push("refs/heads/main", shaA)
	w.env.StartRuns()
	got := w.manualInputs(t, w.owner)
	require.True(t, got.Known)
	assert.True(t, got.Enabled)
	require.NotNil(t, got.Sha)
	assert.Equal(t, shaA, *got.Sha)
	require.Len(t, got.Inputs, 2)
	assert.Equal(t, "dry", got.Inputs[0].Name)
	assert.Equal(t, "boolean", got.Inputs[0].Type)
	assert.Nil(t, got.Inputs[0].Default)
	assert.Empty(t, got.Inputs[0].Options)
	assert.Equal(t, "target", got.Inputs[1].Name)
	assert.Equal(t, "choice", got.Inputs[1].Type)
	assert.Equal(t, []string{"staging", "production"}, got.Inputs[1].Options)
	require.NotNil(t, got.Inputs[1].Default)
	assert.Equal(t, "staging", *got.Inputs[1].Default)

	w.defs.Put(shaB, pipeline("version: 1\non:\n  push: {}\njobs:\n  build:\n    image: IMAGE\n    steps:\n      - run: make\n"))
	feature := w.env.Deliver(w.trigger(triggers.EventPush, "refs/heads/feature", shaB))
	require.True(t, feature.Created)
	w.env.StartRuns()
	assert.True(t, w.manualInputs(t, w.owner).Enabled, "a run of another branch does not change the default branch's inputs")

	w.push("refs/heads/main", shaB)
	w.env.StartRuns()
	latest := w.manualInputs(t, w.owner)
	assert.True(t, latest.Known)
	assert.False(t, latest.Enabled, "the latest file of the default branch has no manual trigger")
	assert.Empty(t, latest.Inputs)

	viewer := w.env.SeedSession(w.env.SeedUser(w.org, "viewer-inputs@example.com", "a long password for the viewer", "viewer"))
	refused := viewer.Get(apptest.OrgPath(w.org, "/projects/"+w.project.String()+"/manual-inputs"))
	require.Equal(t, http.StatusForbidden, refused.Status, "only who may start a run reads its inputs: %s", refused.Raw)
	developer := w.env.SeedSession(w.env.SeedUser(w.org, "dev-inputs@example.com", "a long password for the developer", "developer"))
	assert.True(t, w.manualInputs(t, developer).Known)

	bare := w.env.SeedProject(w.org, w.ownerID)
	res := w.owner.Get(apptest.OrgPath(w.org, "/projects/"+bare.String()+"/manual-inputs"))
	require.Equal(t, http.StatusNotFound, res.Status, string(res.Raw))
	assert.Equal(t, "REPOSITORY_NOT_LINKED", res.Error.Code)
}
