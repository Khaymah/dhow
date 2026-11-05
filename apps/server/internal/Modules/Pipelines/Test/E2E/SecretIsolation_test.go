package e2e_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const isolationPipeline = `version: 1
on:
  push: {}
jobs:
  deploy:
    image: IMAGE
    steps:
      - run: ./deploy.sh
        env:
          TOKEN: ${{ secrets.DEPLOY_TOKEN }}
`

const twoJobPipeline = `version: 1
on:
  push: {}
jobs:
  deploy:
    image: IMAGE
    steps:
      - run: ./deploy.sh
        env:
          TOKEN: ${{ secrets.DEPLOY_TOKEN }}
  audit:
    image: IMAGE
    steps:
      - run: ./audit.sh
        env:
          KEY: ${{ secrets.AUDIT_KEY }}
`

type tenant struct {
	org     uuid.UUID
	project uuid.UUID
	repo    uuid.UUID
	runners []*apptest.TestRunner
}

type leased struct {
	runner *apptest.TestRunner
	grant  *apptest.Grant
}

func (w world) home() tenant {
	return tenant{org: w.org, project: w.project, repo: w.repo, runners: w.runners}
}

func (w world) secondProject() tenant {
	w.env.T.Helper()
	project := w.env.SeedProject(w.org, w.ownerID)
	return tenant{org: w.org, project: project, repo: w.env.SeedRepository(w.org, project, w.ownerID), runners: w.runners}
}

func (w world) foreignOrganization() tenant {
	w.env.T.Helper()
	org := w.env.SeedOrganization("Other Organization", "other-organization")
	user := w.env.SeedUser(org, "owner@other.example.com", "a-long-enough-password-1", "owner")
	project := w.env.SeedProject(org, user)
	runner := w.env.SeedConnectedRunner(org, user, apptest.SeedRunnerSpec{Capacity: 8, Features: checkoutFeatures()})
	return tenant{org: org, project: project, repo: w.env.SeedRepository(org, project, user), runners: []*apptest.TestRunner{runner}}
}

func (w world) pushTo(t tenant) uuid.UUID {
	w.env.T.Helper()
	tr := w.trigger("push", "refs/heads/main", shaA)
	tr.OrganizationID, tr.ProjectID, tr.RepositoryID = t.org, t.project, t.repo
	got := w.env.Deliver(tr)
	require.True(w.env.T, got.Created)
	return got.RunID
}

func (t tenant) lease(tb testing.TB) []leased {
	tb.Helper()
	var out []leased
	for _, r := range t.runners {
		for {
			g, err := r.Lease(context.Background(), 1, 0)
			require.NoError(tb, err)
			if g == nil {
				break
			}
			out = append(out, leased{runner: r, grant: g})
		}
	}
	return out
}

func (w world) projectOf(grant *apptest.Grant) uuid.UUID {
	w.env.T.Helper()
	var project uuid.UUID
	require.NoError(w.env.T, w.env.Pool.QueryRow(context.Background(),
		`SELECT pr."ProjectId" FROM "Jobs" j JOIN "RunJobs" rj ON rj."Id" = j."OriginId" JOIN "PipelineRuns" pr ON pr."Id" = rj."RunId" WHERE j."Id" = $1`,
		uuid.MustParse(grant.Grant.GetJobId())).Scan(&project))
	return project
}

func (w world) lastUsed(org uuid.UUID, project *uuid.UUID, name string) *string {
	w.env.T.Helper()
	var used *string
	require.NoError(w.env.T, w.env.Pool.QueryRow(context.Background(),
		`SELECT "LastUsedAt"::text FROM "Secrets" WHERE "OrganizationId" = $1 AND "ProjectId" IS NOT DISTINCT FROM $2 AND "Name" = $3`,
		org, project, name).Scan(&used))
	return used
}

func (w world) deliveredAudits() int { return w.env.AuditCount("job.secrets_delivered") }

func requireLeaseLost(t *testing.T, err error) {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeFailedPrecondition, ce.Code())
	assert.Equal(t, "LEASE_LOST", ce.Message())
}

type isolationCase struct {
	name   string
	tenant tenant
	scoped bool
	value  string
}

func (c isolationCase) project() *uuid.UUID {
	if c.scoped {
		return &c.tenant.project
	}
	return nil
}

func TestTwoProjectsAndTwoOrganizationsWithOneSecretNameEachGetOnlyTheirOwnValue(t *testing.T) {
	w := newWorld(t)
	cases := []isolationCase{
		{"first project of the organization", w.home(), true, "value-of-the-first-project"},
		{"second project of the organization", w.secondProject(), true, "value-of-the-second-project"},
		{"organization secret of the other organization", w.foreignOrganization(), false, "value-of-the-other-organization"},
	}
	w.defs.Put(shaA, pipeline(isolationPipeline))
	for _, c := range cases {
		w.env.SeedSecret(apptest.SecretRef(c.tenant.org, c.project(), ""), "DEPLOY_TOKEN", c.value)
		w.pushTo(c.tenant)
	}
	w.env.StartRuns()

	byProject := map[uuid.UUID]leased{}
	for _, l := range append(cases[0].tenant.lease(t), cases[2].tenant.lease(t)...) {
		byProject[w.projectOf(l.grant)] = l
	}
	for _, c := range cases {
		assert.Nil(t, w.lastUsed(c.tenant.org, c.project(), "DEPLOY_TOKEN"), "%s: nothing is opened before its job starts", c.name)
	}
	require.Len(t, byProject, len(cases), "every project got its own job")

	started := map[string]bool{}
	for _, c := range cases {
		l := byProject[c.tenant.project]
		start, err := l.runner.Start(l.grant)
		require.NoError(t, err, c.name)
		assert.Equal(t, map[string]string{"DEPLOY_TOKEN": c.value}, openStart(t, l.runner, l.grant, start), c.name)
		started[c.name] = true
		assert.Equal(t, len(started), w.deliveredAudits(), c.name)
		for _, other := range cases {
			assert.Equal(t, started[other.name], w.lastUsed(other.tenant.org, other.project(), "DEPLOY_TOKEN") != nil,
				"%s: only the secrets of jobs that started are marked used (checked after %s)", other.name, c.name)
		}
	}
}

func TestARunnerCannotStartALeaseThatIsNotItsOwnAndNoSecretIsOpened(t *testing.T) {
	w := newWorld(t)
	home := w.home()
	foreign := w.foreignOrganization()
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, ""), "DEPLOY_TOKEN", deployValue)
	w.defs.Put(shaA, pipeline(isolationPipeline))
	w.pushTo(home)
	w.env.StartRuns()

	noForeignLease := foreign.lease(t)
	assert.Empty(t, noForeignLease, "a runner of another organization is never offered the job")

	held := home.lease(t)
	require.Len(t, held, 1)
	owner, g := held[0].runner, held[0].grant
	var sameOrg *apptest.TestRunner
	for _, r := range w.runners {
		if r != owner {
			sameOrg = r
			break
		}
	}
	require.NotNil(t, sameOrg)

	jobAsLease := &runnerv1.LeaseRef{LeaseId: g.Grant.GetJobId(), FencingToken: g.Ref.GetFencingToken()}
	attemptAsLease := &runnerv1.LeaseRef{LeaseId: g.Grant.GetAttemptId(), FencingToken: g.Ref.GetFencingToken()}
	cases := []struct {
		name   string
		runner *apptest.TestRunner
		ref    *runnerv1.LeaseRef
		sign   *apptest.Grant
	}{
		{"runner of another organization with the lease", foreign.runners[0], g.Ref, g},
		{"another runner of the organization with the lease", sameOrg, g.Ref, g},
		{"runner of another organization with the job id", foreign.runners[0], jobAsLease, g},
		{"another runner of the organization with the attempt id", sameOrg, attemptAsLease, g},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.runner.Client.Lease.Start(context.Background(),
				connect.NewRequest(&runnerv1.StartRequest{Lease: c.ref, PopSignature: c.runner.Proof(c.sign)}))
			requireLeaseLost(t, err)
			assert.Zero(t, w.deliveredAudits(), "no delivery is audited for a refused start")
			assert.Nil(t, w.lastUsed(w.org, &w.project, "DEPLOY_TOKEN"), "the secret was never opened")
		})
	}

	start, err := owner.Start(g)
	require.NoError(t, err, "the refused attempts did not use up the real start")
	assert.Equal(t, deployValue, openStart(t, owner, g, start)["DEPLOY_TOKEN"])
	assert.Equal(t, 1, w.deliveredAudits())
	assert.NotNil(t, w.lastUsed(w.org, &w.project, "DEPLOY_TOKEN"))
}

func TestTheStartRequestNamesNoJobSoTheDeliveredSetIsExactlyTheNamesOfTheLeasedJob(t *testing.T) {
	w := newWorld(t)
	home := w.home()
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, ""), "DEPLOY_TOKEN", deployValue)
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, ""), "AUDIT_KEY", "an audit key of the other job")
	w.defs.Put(shaA, pipeline(twoJobPipeline))
	w.pushTo(home)
	w.env.StartRuns()

	fields := (&runnerv1.StartRequest{}).ProtoReflect().Descriptor().Fields()
	names := make([]string, 0, fields.Len())
	for i := range fields.Len() {
		names = append(names, string(fields.Get(i).Name()))
	}
	assert.ElementsMatch(t, []string{"lease", "pop_signature"}, names, "a runner cannot name a job, a secret or an organization at start")

	held := home.lease(t)
	require.Len(t, held, 2)
	byKey := map[string]leased{}
	for _, l := range held {
		byKey[w.env.RunJobKey(uuid.MustParse(l.grant.Grant.GetJobId()))] = l
	}
	require.Len(t, byKey, 2)

	deploy := byKey["deploy"]
	start, err := deploy.runner.Start(deploy.grant)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"DEPLOY_TOKEN": deployValue}, openStart(t, deploy.runner, deploy.grant, start))
	assert.Nil(t, w.lastUsed(w.org, &w.project, "AUDIT_KEY"), "the secret of the other job in the run stays closed")
	assert.NotNil(t, w.lastUsed(w.org, &w.project, "DEPLOY_TOKEN"))

	var data string
	require.NoError(t, w.env.Pool.QueryRow(context.Background(),
		`SELECT "Data"::text FROM "AuditEvents" WHERE "Action" = 'job.secrets_delivered'`).Scan(&data))
	assert.Contains(t, data, "DEPLOY_TOKEN:project:v1")
	assert.NotContains(t, data, "AUDIT_KEY")

	audit := byKey["audit"]
	start, err = audit.runner.Start(audit.grant)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"AUDIT_KEY": "an audit key of the other job"}, openStart(t, audit.runner, audit.grant, start))
	assert.Equal(t, 2, w.deliveredAudits())
}
