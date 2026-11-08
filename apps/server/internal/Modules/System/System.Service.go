package system

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Models"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

const readinessTimeout = 2 * time.Second

var errSchemaNotReady = errors.New("the database schema does not fit this binary")

type Pinger interface {
	Ping(ctx context.Context) error
}

type MigrationStatus interface {
	State(ctx context.Context) (migrations.SchemaState, error)
}

type InstanceQuerier interface {
	GetInstance(ctx context.Context) (sqlc.Instance, error)
}

type ReadinessCheck struct {
	Name models.HealthCheckName
	Run  func(ctx context.Context) error
}

type HealthService struct {
	db         Pinger
	migrations MigrationStatus
	checks     []ReadinessCheck
	log        *slog.Logger
}

func NewHealthService(db Pinger, migrations MigrationStatus, log *slog.Logger, checks ...ReadinessCheck) *HealthService {
	return &HealthService{db: db, migrations: migrations, checks: checks, log: log}
}

func (s *HealthService) Liveness() models.HealthStatus {
	return models.HealthStatus{Status: models.HealthOK, Checks: []models.HealthCheck{}}
}

func (s *HealthService) Readiness(ctx context.Context) models.HealthStatus {
	dbOK := s.check(ctx, models.CheckDatabase, s.db.Ping)
	migOK := dbOK && s.check(ctx, models.CheckMigrations, s.schemaReady)
	status := models.HealthStatus{
		Status: models.HealthOK,
		Checks: []models.HealthCheck{
			{Name: models.CheckDatabase, Ok: dbOK},
			{Name: models.CheckMigrations, Ok: migOK},
		},
	}
	if !dbOK || !migOK {
		status.Status = models.HealthUnavailable
	}
	for _, c := range s.checks {
		ok := migOK && s.check(ctx, c.Name, c.Run)
		status.Checks = append(status.Checks, models.HealthCheck{Name: c.Name, Ok: ok})
		if !ok {
			status.Status = models.HealthUnavailable
		}
	}
	return status
}

func (s *HealthService) schemaReady(ctx context.Context) error {
	state, err := s.migrations.State(ctx)
	if err != nil {
		return err
	}
	if !state.Ready() {
		return fmt.Errorf("%w: %s", errSchemaNotReady, state)
	}
	return nil
}

func (s *HealthService) check(ctx context.Context, name models.HealthCheckName, fn func(context.Context) error) bool {
	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	if err := fn(ctx); err != nil {
		s.log.WarnContext(ctx, "readiness check failed", "check", string(name), "error", err)
		return false
	}
	return true
}

type MetaService struct {
	instances InstanceQuerier
	build     buildinfo.Info
}

func NewMetaService(instances InstanceQuerier, build buildinfo.Info) *MetaService {
	return &MetaService{instances: instances, build: build}
}

func (s *MetaService) Get(ctx context.Context) (models.Meta, error) {
	row, err := s.instances.GetInstance(ctx)
	if err != nil {
		return models.Meta{}, apperrors.Internal(err)
	}
	inst := instanceFromRow(row)
	return models.Meta{
		Version:       s.build.Version,
		Commit:        s.build.Commit,
		Build:         buildOf(s.build),
		SetupRequired: inst.SetupRequired(),
	}, nil
}

func instanceFromRow(row sqlc.Instance) models.Instance {
	return models.Instance{
		ID:               row.Id,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		SetupCompletedAt: row.SetupCompletedAt,
	}
}
