package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const FloorMigration int64 = 50

type Migrator struct {
	provider *goose.Provider
	locker   lock.SessionLocker
	db       *sql.DB
	pool     *pgxpool.Pool
	fsys     fs.FS
	bin      Binary
	latest   int64
}

func NewMigrator(pool *pgxpool.Pool, bin Binary) (*Migrator, error) {
	return NewMigratorFS(pool, FS, bin)
}

func NewMigratorFS(pool *pgxpool.Pool, fsys fs.FS, bin Binary) (*Migrator, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("create migration lock: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("load migrations: %w", err), sqlDB.Close())
	}
	sources := provider.ListSources()
	if len(sources) == 0 {
		return nil, errors.Join(errors.New("no migrations embedded"), sqlDB.Close())
	}
	return &Migrator{
		provider: provider, locker: locker, db: sqlDB, pool: pool, fsys: fsys, bin: bin,
		latest: sources[len(sources)-1].Version,
	}, nil
}

func (m *Migrator) Up(ctx context.Context) (version int64, retErr error) {
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, conn.Close()) }()
	if err := m.locker.SessionLock(ctx, conn); err != nil {
		return 0, fmt.Errorf("apply migrations: take the migration lock: %w", err)
	}
	defer func() {
		if err := m.locker.SessionUnlock(context.WithoutCancel(ctx), conn); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("release the migration lock: %w", err))
		}
	}()
	if err := m.dropInvalidIndexes(ctx); err != nil {
		return 0, err
	}
	if _, err := m.provider.Up(ctx); err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	return m.provider.GetDBVersion(ctx)
}

func (m *Migrator) dropInvalidIndexes(ctx context.Context) error {
	entries, err := m.Plan(ctx)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.Applied && !e.UseTx {
			names = append(names, e.Indexes...)
		}
	}
	if len(names) == 0 {
		return nil
	}
	drops, err := sqlc.New(m.pool).ListInvalidIndexDrops(ctx, names)
	if err != nil {
		return fmt.Errorf("look for indexes left invalid by an interrupted migration: %w", err)
	}
	for _, stmt := range drops {
		if _, err := m.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("drop an index left invalid by an interrupted migration: %w", err)
		}
	}
	return nil
}

func (m *Migrator) DownTo(ctx context.Context, version int64) error {
	if _, err := m.provider.DownTo(ctx, version); err != nil {
		return fmt.Errorf("roll back migrations: %w", err)
	}
	return nil
}

func (m *Migrator) floor(ctx context.Context, dbVersion int64) (string, error) {
	if dbVersion < FloorMigration {
		return DefaultFloor, nil
	}
	floor, err := sqlc.New(m.pool).GetSchemaFloor(ctx)
	if err != nil {
		return "", fmt.Errorf("read the schema floor: %w", err)
	}
	return floor, nil
}

func (m *Migrator) Status(ctx context.Context) (Status, error) {
	v, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("read the schema version: %w", err)
	}
	floor, err := m.floor(ctx, v)
	if err != nil {
		return Status{}, err
	}
	return Evaluate(v, m.latest, floor, m.bin), nil
}

func (m *Migrator) State(ctx context.Context) (SchemaState, error) {
	st, err := m.Status(ctx)
	if err != nil {
		return "", err
	}
	return st.State, nil
}

func (m *Migrator) CheckCompatible(ctx context.Context) (Status, error) {
	st, err := m.Status(ctx)
	if err != nil {
		return Status{}, err
	}
	if st.State == SchemaIncompatible {
		return st, &IncompatibleError{Status: st}
	}
	return st, nil
}

func (m *Migrator) Latest() int64 { return m.latest }

func (m *Migrator) Close() error { return m.db.Close() }
