package e2e_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
)

func (g gitWorld) startWithChange(t *testing.T, change string) bool {
	t.Helper()
	sha := apptest.DiscoveryCommit(pipeline(outputsPipeline)).Sha
	run := g.env.Deliver(g.trigger(triggers.EventPush, "refs/heads/main", sha)).RunID
	g.env.StartRuns()
	discovery := g.env.RunJobs(run)["dhow.discovery"]
	require.Equal(t, "active", discovery.State)
	lease := g.runner.MustLease(0)
	if change != "" {
		_, err := g.env.Pool.Exec(context.Background(), change, g.repo.ID, run, *discovery.Job)
		require.NoError(t, err, change)
	}
	start, err := g.runner.Start(lease)
	return err == nil && start.GetSecrets() != nil
}

func TestTheCheckoutKeyGoesOnlyToTheCurrentJobOfALiveRunAndTheSameRepository(t *testing.T) {
	assert.True(t, newGitWorld(t).startWithChange(t, ""), "the baseline gets its key")
	for name, change := range map[string]string{
		"host key changed after the job was queued": `UPDATE "Repositories" SET "HostKey" = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
			WHERE "Id" = $1 AND $2::uuid IS NOT NULL AND $3::uuid IS NOT NULL`,
		"address changed after the job was queued": `UPDATE "Repositories" SET "CloneUrl" = 'ssh://git@git.example.com/team/other.git'
			WHERE "Id" = $1 AND $2::uuid IS NOT NULL AND $3::uuid IS NOT NULL`,
		"run finished": `UPDATE "PipelineRuns" SET "State" = 'canceled', "FinishedAt" = now()
			WHERE "Id" = $2 AND $1::uuid IS NOT NULL AND $3::uuid IS NOT NULL`,
		"run deleted": `UPDATE "PipelineRuns" SET "IsDeleted" = true WHERE "Id" = $2 AND $1::uuid IS NOT NULL AND $3::uuid IS NOT NULL`,
		"job no longer current": `UPDATE "RunJobs" SET "State" = 'failed', "FinishedAt" = now(), "CurrentJobId" = NULL
			WHERE "CurrentJobId" = $3 AND $1::uuid IS NOT NULL AND $2::uuid IS NOT NULL`,
		"run job deleted": `UPDATE "RunJobs" SET "IsDeleted" = true WHERE "CurrentJobId" = $3 AND $1::uuid IS NOT NULL AND $2::uuid IS NOT NULL`,
		"job deleted":     `UPDATE "Jobs" SET "IsDeleted" = true WHERE "Id" = $3 AND $1::uuid IS NOT NULL AND $2::uuid IS NOT NULL`,
	} {
		t.Run(name, func(t *testing.T) {
			g := newGitWorld(t)
			assert.False(t, g.startWithChange(t, change), "no key once the job is not what it was queued as")
			g.assertNoKey(t)
		})
	}
}
