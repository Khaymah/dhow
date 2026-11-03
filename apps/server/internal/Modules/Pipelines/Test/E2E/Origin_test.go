package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
)

type kindView struct {
	Jobs []struct{ Key, Kind, Name string }
}

type jobOriginView struct {
	Run *struct {
		ProjectId, ProjectName, RunId, Kind string
		RunNumber                     int64
	}
	Attempts []struct {
		RunnerId   string
		RunnerName *string
	}
}

func TestARunNamesItsSystemJobAndAJobKnowsItsRunAndRunner(t *testing.T) {
	g := newGitWorld(t)
	sha := apptest.DiscoveryCommit(pipeline(outputsPipeline)).Sha
	run := g.env.Deliver(g.trigger(triggers.EventPush, "refs/heads/main", sha)).RunID
	g.env.StartRuns()
	res := g.owner.Get(apptest.OrgPath(g.org, "/projects/"+g.project.String()+"/runs/"+run.String()))
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var v kindView
	res.Decode(t, &v)
	require.Len(t, v.Jobs, 1)
	assert.Equal(t, "discovery", v.Jobs[0].Kind)

	discovery := g.env.RunJobs(run)["dhow.discovery"]
	require.NotNil(t, discovery.Job)
	_, ok := g.runner.Discover(pipeline(outputsPipeline))
	require.True(t, ok)
	job := g.owner.Get(apptest.OrgPath(g.org, "/jobs/"+discovery.Job.String()))
	require.Equal(t, http.StatusOK, job.Status, string(job.Raw))
	var o jobOriginView
	job.Decode(t, &o)
	require.NotNil(t, o.Run)
	assert.Equal(t, run.String(), o.Run.RunId)
	assert.Equal(t, g.project.String(), o.Run.ProjectId)
	assert.Equal(t, int64(1), o.Run.RunNumber)
	assert.NotEmpty(t, o.Run.ProjectName)
	assert.Equal(t, "discovery", o.Run.Kind)
	require.Len(t, o.Attempts, 1)
	require.NotNil(t, o.Attempts[0].RunnerName)
	assert.NotEmpty(t, *o.Attempts[0].RunnerName)

	g.env.StartRuns()
	res = g.owner.Get(apptest.OrgPath(g.org, "/projects/"+g.project.String()+"/runs/"+run.String()))
	res.Decode(t, &v)
	kinds := map[string]string{}
	for _, j := range v.Jobs {
		kinds[j.Key] = j.Kind
	}
	assert.Equal(t, "job", kinds["build"])
	assert.Equal(t, "discovery", kinds["dhow.discovery"])
}
