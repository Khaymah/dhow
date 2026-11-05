package unit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	overview "github.com/Khaymah/dhow/apps/server/internal/Modules/Overview"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Overview/Models"
)

var errDown = errors.New("database is down")

type fakeCounter struct {
	calls      []string
	jobs       sqlc.CountOverviewJobsParams
	runners    sqlc.CountOverviewRunnersParams
	invites    sqlc.CountOverviewInvitationsParams
	failOn     string
	jobRow     sqlc.CountOverviewJobsRow
	runnerRow  sqlc.CountOverviewRunnersRow
	pendingRow int64
}

func (f *fakeCounter) fail(name string) error {
	f.calls = append(f.calls, name)
	if f.failOn == name {
		return errDown
	}
	return nil
}

func (f *fakeCounter) CountOverviewJobs(_ context.Context, arg sqlc.CountOverviewJobsParams) (sqlc.CountOverviewJobsRow, error) {
	f.jobs = arg
	return f.jobRow, f.fail("jobs")
}

func (f *fakeCounter) CountOverviewRunners(_ context.Context, arg sqlc.CountOverviewRunnersParams) (sqlc.CountOverviewRunnersRow, error) {
	f.runners = arg
	return f.runnerRow, f.fail("runners")
}

func (f *fakeCounter) CountOverviewApprovals(_ context.Context, _ sqlc.CountOverviewApprovalsParams) (int64, error) {
	return 0, f.fail("approvals")
}

func (f *fakeCounter) CountOverviewInvitations(_ context.Context, arg sqlc.CountOverviewInvitationsParams) (int64, error) {
	f.invites = arg
	return f.pendingRow, f.fail("invitations")
}

var (
	org = uuid.MustParse("00000000-0000-7000-8000-00000000000a")
	now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
)

func asRole(role policy.Role) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{Subject: policy.Subject{
		UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: role}},
	}})
}

func newService(c *fakeCounter) *overview.Service {
	return overview.NewService(overview.Options{Counter: c, Now: func() time.Time { return now }})
}

func TestEachBlockFollowsThePermissionOfItsList(t *testing.T) {
	tests := []struct {
		role  policy.Role
		calls []string
	}{
		{policy.RoleOwner, []string{"jobs", "runners", "invitations", "approvals"}},
		{policy.RoleAdmin, []string{"jobs", "runners", "invitations", "approvals"}},
		{policy.RoleMaintainer, []string{"jobs", "runners", "approvals"}},
		{policy.RoleDeveloper, []string{"jobs", "runners", "approvals"}},
		{policy.RoleViewer, []string{"jobs", "approvals"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			c := &fakeCounter{
				jobRow:     sqlc.CountOverviewJobsRow{Running: 1, Queued: 3, QueuedNoRunner: 2, FailedLast24h: 4},
				runnerRow:  sqlc.CountOverviewRunnersRow{Online: 2, Offline: 1, Paused: 1},
				pendingRow: 5,
			}
			got, err := newService(c).Get(asRole(tt.role), org)
			require.NoError(t, err)
			assert.Equal(t, tt.calls, c.calls, "a block the role may not read must not be queried")
			assert.Equal(t, &models.Jobs{Running: 1, Queued: 3, QueuedNoRunner: 2, FailedLast24h: 4}, got.Jobs)
			assert.Equal(t, policy.Allows(tt.role, policy.RunnerRead), got.Runners != nil)
			assert.Equal(t, policy.Allows(tt.role, policy.MemberManage), got.Invitations != nil)
			if got.Runners != nil {
				assert.Equal(t, models.Runners{Online: 2, Offline: 1, Paused: 1}, *got.Runners)
			}
			if got.Invitations != nil {
				assert.Equal(t, 5, got.Invitations.Pending)
			}
		})
	}
}

func TestTheWindowsComeFromTheClock(t *testing.T) {
	c := &fakeCounter{}
	_, err := newService(c).Get(asRole(policy.RoleOwner), org)
	require.NoError(t, err)
	assert.Equal(t, sqlc.CountOverviewJobsParams{OrganizationID: org, FailedSince: now.Add(-config.JobRecentFailureWindow)}, c.jobs)
	assert.Equal(t, sqlc.CountOverviewRunnersParams{OrganizationID: org, OnlineSince: now.Add(-config.RunnerOnlineWindow)}, c.runners)
	assert.Equal(t, sqlc.CountOverviewInvitationsParams{OrganizationID: org, Now: now}, c.invites)
	assert.Equal(t, 24*time.Hour, config.JobRecentFailureWindow)
}

func TestATokenOnlyGetsTheBlocksItsScopesAllow(t *testing.T) {
	c := &fakeCounter{}
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Subject: policy.Subject{
		UserID: uuid.New(), TokenID: uuid.New(), TokenScopes: []policy.Action{policy.OrgRead, policy.RunnerRead},
		Memberships: []policy.Membership{{OrganizationID: org, Role: policy.RoleOwner}},
	}})
	got, err := newService(c).Get(ctx, org)
	require.NoError(t, err)
	assert.Equal(t, []string{"runners"}, c.calls)
	assert.Nil(t, got.Jobs)
	assert.NotNil(t, got.Runners)
	assert.Nil(t, got.Invitations)
}

func TestAnotherOrganizationGetsNoBlock(t *testing.T) {
	c := &fakeCounter{}
	got, err := newService(c).Get(asRole(policy.RoleOwner), uuid.New())
	require.NoError(t, err)
	assert.Empty(t, c.calls)
	assert.Equal(t, models.Overview{}, got)
}

func TestAFailedCountFailsTheRequest(t *testing.T) {
	for _, block := range []string{"jobs", "runners", "invitations"} {
		t.Run(block, func(t *testing.T) {
			_, err := newService(&fakeCounter{failOn: block}).Get(asRole(policy.RoleOwner), org)
			require.ErrorIs(t, err, errDown)
		})
	}
}

func TestNoPrincipalIsUnauthenticated(t *testing.T) {
	_, err := newService(&fakeCounter{}).Get(context.Background(), org)
	require.Error(t, err)
}
