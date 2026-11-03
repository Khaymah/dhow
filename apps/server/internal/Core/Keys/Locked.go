package keys

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

var ErrNoLiveDataKey = errors.New("the owner has no live data key and a value is only sealed again under one")

func (e *Envelope) SealIn(ctx context.Context, q *sqlc.Queries, owner Owner, label string, rowContext, plaintext []byte) ([]byte, error) {
	return e.sealIn(ctx, q, owner, label, rowContext, plaintext, true)
}

func (e *Envelope) sealIn(ctx context.Context, q *sqlc.Queries, owner Owner, label string, rowContext, plaintext []byte, mayCreate bool) ([]byte, error) {
	if err := checkPurpose(owner, label, rowContext); err != nil {
		return nil, err
	}
	if len(plaintext) > MaxPlaintext {
		return nil, ErrTooLarge
	}
	dk, err := e.lockedDataKey(ctx, q, owner, mayCreate)
	if err != nil {
		return nil, err
	}
	return e.sealWith(ctx, dk, label, rowContext, plaintext)
}

func (e *Envelope) lockedDataKey(ctx context.Context, q *sqlc.Queries, owner Owner, mayCreate bool) (DataKey, error) {
	for range insertAttempts + 1 {
		row, err := q.LockActiveDataKeyForOwner(ctx, sqlc.LockActiveDataKeyForOwnerParams{OwnerKind: string(owner.Kind), OwnerID: owner.idPtr()})
		if err == nil {
			return DataKey{ID: row.Id, Owner: owner, KekVersion: row.KekVersion, Wrapped: row.Wrapped}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return DataKey{}, fmt.Errorf("lock the data key of %s: %w", owner, err)
		}
		if !mayCreate {
			states, err := q.GetOwnerDataKeyStates(ctx, sqlc.GetOwnerDataKeyStatesParams{OwnerKind: string(owner.Kind), OwnerID: owner.idPtr()})
			if err != nil {
				return DataKey{}, fmt.Errorf("read the data keys of %s: %w", owner, err)
			}
			if states.Destroyed > 0 && states.Live == 0 {
				return DataKey{}, ErrShredded
			}
		}
		if err := e.newDataKey(ctx, owner); err != nil {
			return DataKey{}, err
		}
	}
	return DataKey{}, fmt.Errorf("%w: %s", ErrNoLiveDataKey, owner)
}
