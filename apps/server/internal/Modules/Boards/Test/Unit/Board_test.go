package unit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	boards "github.com/Khaymah/dhow/apps/server/internal/Modules/Boards"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Boards/Models"
)

var (
	org     = uuid.MustParse("00000000-0000-7000-8000-00000000000a")
	project = uuid.MustParse("00000000-0000-7000-8000-00000000000b")
	env     = uuid.MustParse("00000000-0000-7000-8000-0000000000e1")
	target  = uuid.MustParse("00000000-0000-7000-8000-0000000000f1")
	host1   = uuid.MustParse("00000000-0000-7000-8000-000000000a01")
	host2   = uuid.MustParse("00000000-0000-7000-8000-000000000a02")
	now     = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	errDown = errors.New("database is down")
)

type store struct {
	envs      []sqlc.ListBoardEnvironmentsRow
	targets   []sqlc.ListBoardTargetsRow
	releases  []sqlc.ListBoardReleasesRow
	running   []sqlc.ListBoardRunningHostsRow
	checks    []sqlc.ListBoardHostChecksRow
	approvals []sqlc.CountBoardApprovalsRow
	failOn    string
	calls     []string
	limit     int32
	releaseBy []uuid.UUID
	checkBy   []uuid.UUID
}

func (s *store) hit(name string) error {
	s.calls = append(s.calls, name)
	if s.failOn == name {
		return errDown
	}
	return nil
}

func (s *store) ListBoardEnvironments(_ context.Context, _ sqlc.ListBoardEnvironmentsParams) ([]sqlc.ListBoardEnvironmentsRow, error) {
	return s.envs, s.hit("environments")
}

func (s *store) ListBoardTargets(_ context.Context, arg sqlc.ListBoardTargetsParams) ([]sqlc.ListBoardTargetsRow, error) {
	s.limit = arg.MaxRows
	return s.targets, s.hit("targets")
}

func (s *store) ListBoardReleases(_ context.Context, arg sqlc.ListBoardReleasesParams) ([]sqlc.ListBoardReleasesRow, error) {
	s.releaseBy = arg.TargetIds
	return s.releases, s.hit("releases")
}

func (s *store) ListBoardRunningHosts(_ context.Context, _ sqlc.ListBoardRunningHostsParams) ([]sqlc.ListBoardRunningHostsRow, error) {
	return s.running, s.hit("running")
}

func (s *store) ListBoardHostChecks(_ context.Context, arg sqlc.ListBoardHostChecksParams) ([]sqlc.ListBoardHostChecksRow, error) {
	s.checkBy = arg.TargetIds
	return s.checks, s.hit("checks")
}

func (s *store) CountBoardApprovals(_ context.Context, _ sqlc.CountBoardApprovalsParams) ([]sqlc.CountBoardApprovalsRow, error) {
	return s.approvals, s.hit("approvals")
}

func base() *store {
	return &store{
		envs:    []sqlc.ListBoardEnvironmentsRow{{Id: env, Name: "production"}},
		targets: []sqlc.ListBoardTargetsRow{{Id: target, EnvironmentId: env, Name: "web", Kind: "ssh", Hosts: 2}},
	}
}

func release(slot, id, state, kind string, started time.Time, finished *time.Time) sqlc.ListBoardReleasesRow {
	return sqlc.ListBoardReleasesRow{
		Slot: slot, Id: uuid.New(), TargetId: target, ReleaseId: id, Kind: kind, State: state, BatchSize: 1, StartedAt: started, FinishedAt: finished,
	}
}

func at(t time.Time) *time.Time { return &t }

func get(t *testing.T, s *store) models.Target {
	t.Helper()
	board, err := boards.NewService(boards.Options{Store: s, Now: func() time.Time { return now }}).Get(context.Background(), org, project)
	require.NoError(t, err)
	require.Len(t, board.Environments, 1)
	require.Len(t, board.Environments[0].Targets, 1)
	return board.Environments[0].Targets[0]
}

func TestTheCurrentReleaseOfARollbackIsTheOneItPutBack(t *testing.T) {
	s := base()
	rollback := release("current", "rel-9", "succeeded", "rollback", now.Add(-time.Hour), at(now.Add(-59*time.Minute)))
	rollback.RestoredReleaseId = pgtype.Text{String: "rel-5", Valid: true}
	rollback.ArtifactDigest = make([]byte, 32)
	s.releases = []sqlc.ListBoardReleasesRow{rollback}
	got := get(t, s)
	assert.Equal(t, "rel-5", got.CurrentReleaseID)
	assert.Len(t, got.CurrentDigest, 64)
	assert.False(t, got.Partial)
}

func TestAShortDigestIsLeftOutInsteadOfBeingGuessed(t *testing.T) {
	s := base()
	rel := release("current", "rel-1", "succeeded", "deploy", now.Add(-time.Hour), at(now.Add(-59*time.Minute)))
	rel.ArtifactDigest = []byte{1, 2, 3}
	s.releases = []sqlc.ListBoardReleasesRow{rel}
	assert.Empty(t, get(t, s).CurrentDigest)
}

func TestARunningDeployReportsItsBatches(t *testing.T) {
	s := base()
	running := release("running", "rel-2", "running", "deploy", now.Add(-time.Minute), nil)
	running.BatchSize = 2
	s.releases = []sqlc.ListBoardReleasesRow{running}
	s.running = []sqlc.ListBoardRunningHostsRow{{ReleaseRowId: running.Id, Hosts: 5, Done: 2, Batch: 1}}
	got := get(t, s)
	require.NotNil(t, got.Running)
	assert.Equal(t, "rel-2", got.Running.ReleaseID)
	assert.Equal(t, 5, got.Running.Hosts)
	assert.Equal(t, 2, got.Running.HostsDone)
	assert.Equal(t, 2, got.Running.BatchSize)
	assert.Equal(t, 3, got.Running.Batches, "5 hosts in batches of 2 are 3 batches")
	assert.Equal(t, 1, got.Running.Batch)
}

func TestARunningDeployWithoutHostRowsYetIsOneBatchOfNothingDone(t *testing.T) {
	s := base()
	s.releases = []sqlc.ListBoardReleasesRow{release("running", "rel-2", "running", "deploy", now, nil)}
	got := get(t, s)
	require.NotNil(t, got.Running)
	assert.Equal(t, 0, got.Running.Hosts)
	assert.Equal(t, 1, got.Running.Batches)
}

func TestTheLastFinishedReleaseSetsTheBadges(t *testing.T) {
	for _, tt := range []struct {
		state        string
		auto, failed bool
	}{
		{"succeeded", false, false},
		{"rolled_back", true, false},
		{"rollback_failed", false, true},
		{"failed", false, false},
	} {
		t.Run(tt.state, func(t *testing.T) {
			s := base()
			s.releases = []sqlc.ListBoardReleasesRow{release("last", "rel-3", tt.state, "deploy", now.Add(-time.Hour), at(now.Add(-59*time.Minute)))}
			got := get(t, s)
			assert.Equal(t, tt.auto, got.AutoRolledBack)
			assert.Equal(t, tt.failed, got.RollbackFailed)
			require.NotNil(t, got.Last)
			assert.Equal(t, tt.state, got.Last.State)
		})
	}
}

func check(host uuid.UUID, releaseID string, checked time.Time) sqlc.ListBoardHostChecksRow {
	return sqlc.ListBoardHostChecksRow{TargetId: target, HostId: host, CurrentReleaseId: pgtype.Text{String: releaseID, Valid: releaseID != ""}, CheckedAt: checked}
}

func TestHostsOnDifferentReleasesAreMixedVersions(t *testing.T) {
	s := base()
	finished := now.Add(-time.Hour)
	s.releases = []sqlc.ListBoardReleasesRow{release("last", "rel-3", "succeeded", "deploy", finished.Add(-time.Minute), at(finished))}
	s.checks = []sqlc.ListBoardHostChecksRow{check(host1, "rel-3", now), check(host2, "rel-2", now)}
	assert.True(t, get(t, s).Partial)

	s.checks = []sqlc.ListBoardHostChecksRow{check(host1, "rel-3", now), check(host2, "rel-3", now)}
	assert.False(t, get(t, s).Partial)
}

func TestACheckOlderThanTheLastReleaseSaysNothingAboutIt(t *testing.T) {
	s := base()
	finished := now.Add(-time.Hour)
	s.releases = []sqlc.ListBoardReleasesRow{release("last", "rel-3", "succeeded", "deploy", finished.Add(-time.Minute), at(finished))}
	s.checks = []sqlc.ListBoardHostChecksRow{check(host1, "rel-3", finished.Add(-time.Hour)), check(host2, "rel-2", finished.Add(-time.Hour))}
	assert.False(t, get(t, s).Partial, "the old check ran before the release and cannot count against it")
}

func TestAFailedRollbackOrAnUnknownStateStaysMixedUntilEveryHostIsCheckedOnOneRelease(t *testing.T) {
	for _, state := range []string{"rollback_failed", "unknown"} {
		t.Run(state, func(t *testing.T) {
			s := base()
			finished := now.Add(-time.Hour)
			s.releases = []sqlc.ListBoardReleasesRow{release("last", "rel-3", state, "deploy", finished.Add(-time.Minute), at(finished))}
			assert.True(t, get(t, s).Partial, "no check yet")

			s.checks = []sqlc.ListBoardHostChecksRow{check(host1, "rel-2", now)}
			assert.True(t, get(t, s).Partial, "one host is not all hosts")

			s.checks = []sqlc.ListBoardHostChecksRow{check(host1, "rel-2", now), check(host2, "rel-2", now)}
			assert.False(t, get(t, s).Partial, "every host answered with the same release")

			s.checks = []sqlc.ListBoardHostChecksRow{check(host1, "rel-2", now), check(host2, "", now)}
			assert.True(t, get(t, s).Partial, "a host that did not say what it runs is not confirmed")
		})
	}
}

func TestEveryEnvironmentIsARowAndPendingApprovalsAreCountedPerEnvironment(t *testing.T) {
	s := base()
	other := uuid.New()
	s.envs = append(s.envs, sqlc.ListBoardEnvironmentsRow{Id: other, Name: "staging", Paused: true})
	s.approvals = []sqlc.CountBoardApprovalsRow{{EnvironmentId: env, Pending: 3}}
	board, err := boards.NewService(boards.Options{Store: s}).Get(context.Background(), org, project)
	require.NoError(t, err)
	require.Len(t, board.Environments, 2)
	assert.Equal(t, 3, board.Environments[0].PendingApprovals)
	assert.Len(t, board.Environments[0].Targets, 1)
	assert.True(t, board.Environments[1].Paused)
	assert.NotNil(t, board.Environments[1].Targets, "an environment without a target has an empty list, not null")
	assert.Empty(t, board.Environments[1].Targets)
}

func TestTheBoardStopsAtFiveHundredTargetsAndSaysSo(t *testing.T) {
	s := base()
	s.targets = nil
	for range boards.MaxTargets + 1 {
		s.targets = append(s.targets, sqlc.ListBoardTargetsRow{Id: uuid.New(), EnvironmentId: env, Name: "t"})
	}
	board, err := boards.NewService(boards.Options{Store: s}).Get(context.Background(), org, project)
	require.NoError(t, err)
	assert.True(t, board.Truncated)
	assert.Len(t, board.Environments[0].Targets, boards.MaxTargets)
	assert.Equal(t, int32(boards.MaxTargets+1), s.limit, "one more than the cap is asked for, to know there is more")
}

func TestARunningReleaseIsAskedForItsHostsOnlyWhenThereIsOne(t *testing.T) {
	s := base()
	_, err := boards.NewService(boards.Options{Store: s}).Get(context.Background(), org, project)
	require.NoError(t, err)
	assert.NotContains(t, s.calls, "running")
	assert.ElementsMatch(t, []string{"environments", "targets", "releases", "checks", "approvals"}, s.calls, "the number of queries does not grow with the board")
}

func TestAFailedQueryFailsTheBoard(t *testing.T) {
	for _, name := range []string{"environments", "targets", "releases", "checks", "approvals"} {
		t.Run(name, func(t *testing.T) {
			s := base()
			s.failOn = name
			_, err := boards.NewService(boards.Options{Store: s}).Get(context.Background(), org, project)
			require.ErrorIs(t, err, errDown)
		})
	}
	s := base()
	s.releases = []sqlc.ListBoardReleasesRow{release("running", "rel-2", "running", "deploy", now, nil)}
	s.failOn = "running"
	_, err := boards.NewService(boards.Options{Store: s}).Get(context.Background(), org, project)
	require.ErrorIs(t, err, errDown)
}

func TestTheReleaseAndCheckQueriesAreBoundToTheRetainedTargetsOnly(t *testing.T) {
	s := base()
	s.targets = nil
	for range boards.MaxTargets + 1 {
		s.targets = append(s.targets, sqlc.ListBoardTargetsRow{Id: uuid.New(), EnvironmentId: env, Name: "t"})
	}
	_, err := boards.NewService(boards.Options{Store: s}).Get(context.Background(), org, project)
	require.NoError(t, err)
	assert.Len(t, s.releaseBy, boards.MaxTargets, "the extra target that was cut is not asked about")
	assert.Equal(t, s.releaseBy, s.checkBy)
	assert.NotContains(t, s.releaseBy, s.targets[boards.MaxTargets].Id)
}

type fakeLimiter struct {
	allowed bool
	took    int
}

func (f *fakeLimiter) Take(_ context.Context, l ratelimit.Limit, _ string) (ratelimit.Decision, error) {
	f.took++
	return ratelimit.Decision{Allowed: f.allowed, RetryAfter: l.Window}, nil
}

func TestTheBoardReadsAreRateLimitedPerUser(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Subject: policy.Subject{UserID: uuid.New()}})
	limiter := &fakeLimiter{allowed: true}
	service := boards.NewService(boards.Options{Store: base(), Limiter: limiter})
	_, err := service.Get(ctx, org, project)
	require.NoError(t, err)
	assert.Equal(t, 1, limiter.took)

	limiter.allowed = false
	s := base()
	_, err = boards.NewService(boards.Options{Store: s, Limiter: limiter}).Get(ctx, org, project)
	var ae *apperrors.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeRateLimited, ae.Code)
	assert.Empty(t, s.calls, "a refused read touches no table")

	_, err = service.Get(context.Background(), org, project)
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeUnauthenticated, ae.Code)
}
