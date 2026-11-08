package unit_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Models"
	support "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Test/Support"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

type deadlinePinger struct{ deadline time.Time }

func (p *deadlinePinger) Ping(ctx context.Context) error {
	p.deadline, _ = ctx.Deadline()
	return nil
}

func TestLivenessNeverTouchesDependencies(t *testing.T) {
	svc := system.NewHealthService(support.FakeDB{Err: errors.New("down")}, support.FakeMigrations{Err: errors.New("down")}, support.DiscardLogger())
	status := svc.Liveness()
	assert.Equal(t, models.HealthOK, status.Status)
	assert.True(t, status.Ready())
	assert.Empty(t, status.Checks)
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		db         support.FakeDB
		migrations support.FakeMigrations
		want       models.HealthState
		wantChecks map[models.HealthCheckName]bool
	}{
		{"ready", support.FakeDB{}, support.FakeMigrations{}, models.HealthOK, map[models.HealthCheckName]bool{models.CheckDatabase: true, models.CheckMigrations: true}},
		{
			"database down",
			support.FakeDB{Err: errors.New("dial tcp 10.0.0.5:5432")},
			support.FakeMigrations{},
			models.HealthUnavailable,
			map[models.HealthCheckName]bool{models.CheckDatabase: false, models.CheckMigrations: false},
		},
		{
			"pending migrations",
			support.FakeDB{},
			support.FakeMigrations{Pending: true},
			models.HealthUnavailable,
			map[models.HealthCheckName]bool{models.CheckDatabase: true, models.CheckMigrations: false},
		},
		{
			"a newer schema whose floor allows this binary",
			support.FakeDB{},
			support.FakeMigrations{Schema: migrations.SchemaAhead},
			models.HealthOK,
			map[models.HealthCheckName]bool{models.CheckDatabase: true, models.CheckMigrations: true},
		},
		{
			"a newer schema whose floor is above this binary",
			support.FakeDB{},
			support.FakeMigrations{Schema: migrations.SchemaIncompatible},
			models.HealthUnavailable,
			map[models.HealthCheckName]bool{models.CheckDatabase: true, models.CheckMigrations: false},
		},
		{
			"migration status error",
			support.FakeDB{},
			support.FakeMigrations{Err: errors.New("boom")},
			models.HealthUnavailable,
			map[models.HealthCheckName]bool{models.CheckDatabase: true, models.CheckMigrations: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := system.NewHealthService(tt.db, tt.migrations, support.DiscardLogger()).Readiness(context.Background())
			assert.Equal(t, tt.want, status.Status)
			got := map[models.HealthCheckName]bool{}
			for _, c := range status.Checks {
				got[c.Name] = c.Ok
			}
			assert.Equal(t, tt.wantChecks, got)
		})
	}
}

func TestReadinessRunsTheExtraChecks(t *testing.T) {
	missing := system.ReadinessCheck{Name: models.CheckLogPartitions, Run: func(context.Context) error { return errors.New("no partition") }}
	present := system.ReadinessCheck{Name: models.CheckLogPartitions, Run: func(context.Context) error { return nil }}
	tests := []struct {
		name  string
		mig   support.FakeMigrations
		check system.ReadinessCheck
		want  models.HealthState
		ok    bool
	}{
		{"the partition of today exists", support.FakeMigrations{}, present, models.HealthOK, true},
		{"the partition of today is missing", support.FakeMigrations{}, missing, models.HealthUnavailable, false},
		{"an unmigrated schema skips the check", support.FakeMigrations{Pending: true}, present, models.HealthUnavailable, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := system.NewHealthService(support.FakeDB{}, tt.mig, support.DiscardLogger(), tt.check).Readiness(context.Background())
			assert.Equal(t, tt.want, status.Status)
			require.Len(t, status.Checks, 3)
			assert.Equal(t, models.HealthCheck{Name: models.CheckLogPartitions, Ok: tt.ok}, status.Checks[2])
		})
	}
}

func TestReadinessBoundsEachCheck(t *testing.T) {
	pinger := &deadlinePinger{}
	start := time.Now()
	system.NewHealthService(pinger, support.FakeMigrations{}, support.DiscardLogger()).Readiness(context.Background())
	require.False(t, pinger.deadline.IsZero(), "the database check must carry a deadline")
	assert.LessOrEqual(t, pinger.deadline.Sub(start), 2*time.Second+100*time.Millisecond)
}

func TestMetaGet(t *testing.T) {
	build := buildinfo.Info{Version: "v0.1.0", Commit: "abc123"}
	completedAt := time.Now()
	done := sqlc.Instance{SetupCompletedAt: &completedAt}
	tests := []struct {
		name     string
		queries  support.FakeQueries
		want     models.Meta
		wantCode string
	}{
		{"setup required", support.FakeQueries{}, models.Meta{Version: "v0.1.0", Commit: "abc123", Build: models.Build{Version: "v0.1.0", Commit: "abc123"}, SetupRequired: true}, ""},
		{"setup done", support.FakeQueries{Instance: done}, models.Meta{Version: "v0.1.0", Commit: "abc123", Build: models.Build{Version: "v0.1.0", Commit: "abc123"}}, ""},
		{"query error becomes internal", support.FakeQueries{Err: errors.New(`relation "Instance" does not exist`)}, models.Meta{}, apperrors.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := system.NewMetaService(tt.queries, build).Get(context.Background())
			if tt.wantCode != "" {
				require.Error(t, err)
				ae := apperrors.From(err)
				assert.Equal(t, tt.wantCode, ae.Code)
				assert.Equal(t, http.StatusInternalServerError, ae.Status)
				assert.Equal(t, tt.queries.Err, errors.Unwrap(ae))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, meta)
		})
	}
}

func TestInstanceSetupRequired(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		inst models.Instance
		want bool
	}{
		{"not completed", models.Instance{}, true},
		{"completed", models.Instance{SetupCompletedAt: &now}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.inst.SetupRequired())
		})
	}
}
