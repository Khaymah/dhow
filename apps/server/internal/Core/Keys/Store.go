package keys

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	stateActive    = "active"
	stateDestroyed = "destroyed"
)

type DataKey struct {
	ID         uuid.UUID
	Owner      Owner
	KekVersion int32
	Wrapped    []byte
	Destroyed  bool
}

type Store interface {
	ActiveKekVersion(ctx context.Context) (int32, error)
	DataKey(ctx context.Context, id uuid.UUID) (DataKey, error)
	ActiveDataKey(ctx context.Context, owner Owner) (DataKey, error)
	InsertDataKey(ctx context.Context, dk DataKey) error
	DestroyOwner(ctx context.Context, owner Owner) (int64, error)
}

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type PostgresStore struct {
	db DB
}

func NewPostgresStore(db DB) *PostgresStore { return &PostgresStore{db: db} }

func (s *PostgresStore) ActiveKekVersion(ctx context.Context) (int32, error) {
	v, err := sqlc.New(s.db).GetActiveKeyEncryptionKey(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, newError(CodeKekNoActive, 0, "no KEK version is active; start dhow-server once so it registers the keyring")
	}
	if err != nil {
		return 0, fmt.Errorf("read the active KEK version: %w", err)
	}
	return v, nil
}

func (s *PostgresStore) DataKey(ctx context.Context, id uuid.UUID) (DataKey, error) {
	row, err := sqlc.New(s.db).GetDataKey(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DataKey{}, ErrNoDataKey
	}
	if err != nil {
		return DataKey{}, fmt.Errorf("read a data key: %w", err)
	}
	return DataKey{
		ID: row.Id, Owner: ownerFrom(row.OwnerKind, row.OwnerId), KekVersion: row.KekVersion, Wrapped: row.Wrapped,
		Destroyed: row.State == stateDestroyed,
	}, nil
}

func (s *PostgresStore) ActiveDataKey(ctx context.Context, owner Owner) (DataKey, error) {
	row, err := sqlc.New(s.db).GetActiveDataKeyForOwner(ctx, sqlc.GetActiveDataKeyForOwnerParams{
		OwnerKind: string(owner.Kind), OwnerID: owner.idPtr(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return DataKey{}, ErrNoDataKey
	}
	if err != nil {
		return DataKey{}, fmt.Errorf("read the data key of %s: %w", owner, err)
	}
	return DataKey{ID: row.Id, Owner: ownerFrom(row.OwnerKind, row.OwnerId), KekVersion: row.KekVersion, Wrapped: row.Wrapped}, nil
}

func (s *PostgresStore) InsertDataKey(ctx context.Context, dk DataKey) error {
	if _, err := sqlc.New(s.db).InsertDataKey(ctx, sqlc.InsertDataKeyParams{
		ID: dk.ID, OwnerKind: string(dk.Owner.Kind), OwnerID: dk.Owner.idPtr(), KekVersion: dk.KekVersion, Wrapped: dk.Wrapped,
	}); err != nil {
		return fmt.Errorf("store the data key of %s: %w", dk.Owner, err)
	}
	return nil
}

func (s *PostgresStore) DestroyOwner(ctx context.Context, owner Owner) (int64, error) {
	n, err := sqlc.New(s.db).DestroyDataKeysOfOwner(ctx, sqlc.DestroyDataKeysOfOwnerParams{
		OwnerKind: string(owner.Kind), OwnerID: owner.idPtr(),
	})
	if err != nil {
		return 0, fmt.Errorf("destroy the data key of %s: %w", owner, err)
	}
	return n, nil
}
