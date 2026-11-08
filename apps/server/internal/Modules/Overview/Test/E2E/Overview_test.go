package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

type jobs struct{ Running, Queued, QueuedNoRunner, FailedLast24h int }

type runners struct{ Online, Offline, Paused int }

type invitations struct{ Pending int }

type approvals struct{ Pending int }

type deployments struct{ Running, FailedLast24h, RollbackFailed, Pending int }

type notifications struct{ Dead int }

type overview struct {
	Jobs          *jobs
	Runners       *runners
	Invitations   *invitations
	Approvals     *approvals
	Deployments   *deployments
	Notifications *notifications
}

type world struct {
	env   *apptest.Env
	org   uuid.UUID
	owner *apptest.Client
	ids   map[policy.Role]uuid.UUID
}

func at(t time.Time) *time.Time { return &t }

func newWorld(t *testing.T) world {
	t.Helper()
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	w := world{env: env, org: org, owner: owner, ids: map[policy.Role]uuid.UUID{policy.RoleOwner: uuid.MustParse(owner.UserID)}}
	for _, role := range []policy.Role{policy.RoleAdmin, policy.RoleMaintainer, policy.RoleDeveloper, policy.RoleViewer} {
		w.ids[role] = env.SeedUser(org, string(role)+"@example.com", "a long password for "+string(role), string(role))
	}
	return w
}

func (w world) seed() {
	creator := w.ids[policy.RoleOwner]
	now := w.env.Clock.Now()
	w.env.SeedRunnerWith(w.org, creator, apptest.RunnerSeed{LastSeenAt: at(now)})
	w.env.SeedRunnerWith(w.org, creator, apptest.RunnerSeed{Labels: []string{"arm64"}, LastSeenAt: at(now.Add(-3 * time.Hour))})
	w.env.SeedRunnerWith(w.org, creator, apptest.RunnerSeed{State: "disabled", Labels: []string{"gpu"}})
	w.env.SeedJobWith(w.org, creator, apptest.JobSeed{State: "running"})
	w.env.SeedJobWith(w.org, creator, apptest.JobSeed{})
	w.env.SeedJobWith(w.org, creator, apptest.JobSeed{Labels: []string{"arm64"}})
	w.env.SeedJobWith(w.org, creator, apptest.JobSeed{Labels: []string{"gpu"}})
	w.env.SeedJobWith(w.org, creator, apptest.JobSeed{Labels: []string{"gpu"}, Isolation: "gvisor"})
	w.env.SeedJobWith(w.org, creator, apptest.JobSeed{State: "failed", FinishedAt: at(now.Add(-time.Hour))})
	w.env.SeedJobWith(w.org, creator, apptest.JobSeed{State: "failed", FinishedAt: at(now.Add(-48 * time.Hour))})
	w.env.SeedInvitationWith(w.org, creator, apptest.InvitationSeed{ExpiresAt: now.Add(24 * time.Hour)})
}

func (w world) get(c *apptest.Client) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/overview")})
}

func TestEveryRoleSeesTheBlocksItsListsAllow(t *testing.T) {
	w := newWorld(t)
	w.seed()
	full := overview{
		Jobs:          &jobs{Running: 1, Queued: 4, QueuedNoRunner: 2, FailedLast24h: 1},
		Runners:       &runners{Online: 1, Offline: 1, Paused: 1},
		Invitations:   &invitations{Pending: 1},
		Approvals:     &approvals{Pending: 0},
		Deployments:   &deployments{},
		Notifications: &notifications{},
	}
	for _, role := range policy.Roles() {
		t.Run(string(role), func(t *testing.T) {
			res := w.get(w.env.SeedSession(w.ids[role]))
			require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
			assert.True(t, res.Success)
			assert.Equal(t, "overview.loaded", res.Message)

			var raw map[string]json.RawMessage
			res.Decode(t, &raw)
			assert.Len(t, raw, 6)
			blocks := map[string]policy.Action{
				"Jobs": policy.JobRead, "Runners": policy.RunnerRead, "Invitations": policy.MemberManage, "Approvals": policy.RunRead,
				"Deployments": policy.RunRead, "Notifications": policy.OrgNotificationManage,
			}
			for name, action := range blocks {
				require.Contains(t, raw, name)
				if !policy.Allows(role, action) {
					assert.JSONEq(t, "null", string(raw[name]), "%s must be null, not zero, for %s", name, role)
				}
			}

			var got overview
			res.Decode(t, &got)
			want := full
			if !policy.Allows(role, policy.RunnerRead) {
				want.Runners = nil
			}
			if !policy.Allows(role, policy.MemberManage) {
				want.Invitations = nil
			}
			if !policy.Allows(role, policy.OrgNotificationManage) {
				want.Notifications = nil
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestAFreshOrganizationCountsZeroNotNull(t *testing.T) {
	w := newWorld(t)
	res := w.get(w.owner)
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	assert.JSONEq(t, `{"Jobs":{"Running":0,"Queued":0,"QueuedNoRunner":0,"FailedLast24h":0},
		"Runners":{"Online":0,"Offline":0,"Paused":0},"Invitations":{"Pending":0},"Approvals":{"Pending":0},
		"Deployments":{"Running":0,"FailedLast24h":0,"RollbackFailed":0,"Pending":0},"Notifications":{"Dead":0}}`, string(res.Data))
}

func TestQueuedNoRunnerMatchesTheReasonTheJobListShows(t *testing.T) {
	w := newWorld(t)
	w.seed()
	list := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/jobs?State=queued&Limit=100")})
	require.Equal(t, http.StatusOK, list.Status, string(list.Raw))
	var listed []struct{ QueuedReason string }
	list.Decode(t, &listed)
	stuck := 0
	for _, j := range listed {
		if j.QueuedReason == "NO_MATCHING_RUNNER" {
			stuck++
		}
	}

	var got overview
	w.get(w.owner).Decode(t, &got)
	require.NotNil(t, got.Jobs)
	assert.Equal(t, len(listed), got.Jobs.Queued)
	assert.Equal(t, stuck, got.Jobs.QueuedNoRunner)
}

func TestOnlineAndOfflineMatchTheRunnerList(t *testing.T) {
	w := newWorld(t)
	w.seed()
	list := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/runners")})
	require.Equal(t, http.StatusOK, list.Status, string(list.Raw))
	var listed []struct{ Status string }
	list.Decode(t, &listed)
	status := map[string]int{}
	for _, r := range listed {
		status[r.Status]++
	}

	var got overview
	w.get(w.owner).Decode(t, &got)
	require.NotNil(t, got.Runners)
	assert.Equal(t, runners{Online: status["online"], Offline: status["offline"], Paused: status["disabled"]}, *got.Runners)
}

func TestStrangersAreTurnedAway(t *testing.T) {
	w := newWorld(t)
	other := w.env.SeedOrganization("Other Org", "other-org")
	outsider := w.env.SeedSession(w.env.SeedUser(other, "outsider@example.com", "a long password for outsider", "owner"))

	res := w.get(outsider)
	assert.Equal(t, http.StatusNotFound, res.Status, string(res.Raw))
	require.NotNil(t, res.Error)
	assert.Equal(t, "NOT_FOUND", res.Error.Code)

	anonymous := w.get(&apptest.Client{Env: w.env})
	assert.Equal(t, http.StatusUnauthorized, anonymous.Status, string(anonymous.Raw))
}

func TestATokenSeesOnlyTheBlocksOfItsScopes(t *testing.T) {
	w := newWorld(t)
	w.seed()
	token := w.env.SeedAPIToken(w.ids[policy.RoleOwner], w.org, "org.read", "job.read")
	res := w.env.Bearer(token.Value, apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/overview")})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var raw map[string]json.RawMessage
	res.Decode(t, &raw)
	assert.NotEqual(t, "null", string(raw["Jobs"]))
	assert.JSONEq(t, "null", string(raw["Runners"]))
	assert.JSONEq(t, "null", string(raw["Invitations"]))

	orgOnly := w.env.SeedAPIToken(w.ids[policy.RoleOwner], w.org, "org.read")
	bare := w.env.Bearer(orgOnly.Value, apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/overview")})
	require.Equal(t, http.StatusOK, bare.Status, string(bare.Raw))
	var none map[string]json.RawMessage
	bare.Decode(t, &none)
	for _, block := range []string{"Jobs", "Runners", "Invitations", "Approvals", "Deployments", "Notifications"} {
		assert.JSONEq(t, "null", string(none[block]), block)
	}

	narrow := w.env.SeedAPIToken(w.ids[policy.RoleOwner], w.org, "job.read")
	denied := w.env.Bearer(narrow.Value, apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/overview")})
	assert.Equal(t, http.StatusForbidden, denied.Status, string(denied.Raw))
}

func (w world) seedDeploys(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	project := w.env.SeedProject(w.org, w.ids[policy.RoleOwner])
	target, _ := w.env.SeedDeployTarget(project, "production", "web")
	var environment uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(ctx, `SELECT "EnvironmentId" FROM "DeployTargets" WHERE "Id" = $1`, target).Scan(&environment))
	deployment := func(number int, state string, finished string) uuid.UUID {
		var id uuid.UUID
		require.NoError(t, w.env.Pool.QueryRow(ctx,
			`INSERT INTO "Deployments" ("OrganizationId", "EnvironmentId", "RunId", "RunNumber", "RunJobId", "JobId", "JobKey", "Ref", "RuleVersion", "State", "FinishedAt")
			 VALUES ($1, $2, gen_random_uuid(), $3, gen_random_uuid(), gen_random_uuid(), 'deploy', 'main', 1, $4, CASE WHEN $5 = '' THEN NULL ELSE now() - $5::interval END)
			 RETURNING "Id"`, w.org, environment, number, state, finished).Scan(&id))
		return id
	}
	deployment(1, "running", "")
	deployment(2, "running", "")
	deployment(3, "failed", "1 hour")
	deployment(4, "failed", "2 days")
	deployment(5, "succeeded", "1 hour")
	last := deployment(6, "failed", "1 minute")
	_, err := w.env.Pool.Exec(ctx,
		`INSERT INTO "DeploymentReleases" ("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex", "Kind", "ReleaseId", "State", "StartedAt", "FinishedAt")
		 VALUES ($1, $2, gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), $3, 0, 'deploy', 'rel-1', 'succeeded', now() - interval '2 hours', now() - interval '119 minutes'),
		        ($1, $2, gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), $3, 0, 'deploy', 'rel-2', 'rollback_failed', now() - interval '10 minutes', now() - interval '9 minutes')`,
		w.org, last, target)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(ctx,
		`INSERT INTO "DeploymentApprovals" ("OrganizationId", "EnvironmentId", "RunId", "RunNumber", "Ref", "Round", "RuleVersion", "State", "ExpiresAt")
		 VALUES ($1, $2, gen_random_uuid(), 1, 'main', 1, 1, 'pending', now() + interval '1 day'),
		        ($1, $2, gen_random_uuid(), 2, 'main', 1, 1, 'pending', now() - interval '1 day')`, w.org, environment)
	require.NoError(t, err)
	var channel uuid.UUID
	require.NoError(t, w.env.Pool.QueryRow(ctx,
		`INSERT INTO "NotificationChannels" ("OrganizationId", "Kind", "Name", "CreatedByUserId") VALUES ($1, 'email', 'overview-test', $2) RETURNING "Id"`,
		w.org, w.ids[policy.RoleOwner]).Scan(&channel))
	for _, state := range []string{"dead", "dead", "pending", "sent"} {
		_, err = w.env.Pool.Exec(ctx,
			`INSERT INTO "NotificationOutbox" ("OrganizationId", "ChannelId", "Kind", "EventId", "EventName", "Payload", "State")
			 VALUES ($1, $2, 'email', gen_random_uuid(), 'deployment.finished', '{}', $3)`, w.org, channel, state)
		require.NoError(t, err)
	}
}

func TestTheDeployBlocksCountWhatTheyNameAndOnlyForWhoMayRead(t *testing.T) {
	w := newWorld(t)
	w.seedDeploys(t)

	var owner overview
	w.get(w.owner).Decode(t, &owner)
	assert.Equal(t, &deployments{Running: 2, FailedLast24h: 2, RollbackFailed: 1, Pending: 1}, owner.Deployments,
		"two running, two failed in a day (the old one and the finished one that was not in the window do not count), one target that could not roll back, one live request")
	assert.Equal(t, &notifications{Dead: 2}, owner.Notifications)

	var viewer overview
	w.get(w.env.SeedSession(w.ids[policy.RoleViewer])).Decode(t, &viewer)
	assert.Equal(t, owner.Deployments, viewer.Deployments, "a viewer may read runs, so the deploy counters are the same")
	assert.Nil(t, viewer.Notifications, "dead notifications are for Owner and Admin")

	var maintainer overview
	w.get(w.env.SeedSession(w.ids[policy.RoleMaintainer])).Decode(t, &maintainer)
	assert.Nil(t, maintainer.Notifications)
}

func TestTheOverviewKeepsItsETagWhileNothingChangesAndAnswers304(t *testing.T) {
	w := newWorld(t)
	w.seed()
	first := w.get(w.owner)
	require.Equal(t, http.StatusOK, first.Status, string(first.Raw))
	etag := first.Header.Get("ETag")
	require.NotEmpty(t, etag)

	again := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/overview"), Headers: map[string]string{"If-None-Match": etag}})
	assert.Equal(t, http.StatusNotModified, again.Status)
	assert.Empty(t, again.Raw)

	w.env.SeedJobWith(w.org, w.ids[policy.RoleOwner], apptest.JobSeed{})
	changed := w.owner.Do(apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/overview"), Headers: map[string]string{"If-None-Match": etag}})
	assert.Equal(t, http.StatusOK, changed.Status, "a change of the counts is a new answer")
	assert.NotEqual(t, etag, changed.Header.Get("ETag"))

	other := w.env.SeedSession(w.ids[policy.RoleViewer]).Do(apptest.Request{Method: http.MethodGet, Path: apptest.OrgPath(w.org, "/overview"), Headers: map[string]string{"If-None-Match": etag}})
	assert.Equal(t, http.StatusOK, other.Status, "the ETag of one reader never answers for another, whose blocks differ")
}
