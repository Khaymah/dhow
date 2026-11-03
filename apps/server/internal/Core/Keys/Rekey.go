package keys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	CodeRekeyRunning     = "REKEY_RUNNING"
	CodeRekeyUnreadable  = "REKEY_UNREADABLE"
	CodeRekeyNotDrained  = "REKEY_NOT_DRAINED"
	DefaultRekeyBatch    = 200
	DefaultRekeyGrace    = 10 * time.Second
	rekeyDrainAttempts   = 3
	rekeyUnreadableShown = 20
)

type Pinner interface {
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

type RekeyOptions struct {
	Grace time.Duration
	Batch int32
	Wait  func(ctx context.Context, d time.Duration) bool
}

type RekeyResult struct {
	Run        uuid.UUID
	Resumed    bool
	Superseded int64
	Resealed   int64
	Destroyed  int64
}

type rekeyer struct {
	m      *Manager
	conn   *pgxpool.Conn
	values *Values
	reg    *Registry
	opts   RekeyOptions
	run    sqlc.GetOpenKeyRekeyRow
}

func (m *Manager) Rekey(ctx context.Context, pinner Pinner, values *Values, reg *Registry, opts RekeyOptions) (RekeyResult, error) {
	if opts.Batch <= 0 {
		opts.Batch = DefaultRekeyBatch
	}
	if opts.Grace < 0 {
		opts.Grace = 0
	}
	if opts.Wait == nil {
		opts.Wait = waitFor
	}
	conn, err := pinner.Acquire(ctx)
	if err != nil {
		return RekeyResult{}, fmt.Errorf("connect for the rekey: %w", err)
	}
	defer conn.Release()
	locked, err := sqlc.New(conn).TryLockKeyRekey(ctx)
	if err != nil {
		return RekeyResult{}, fmt.Errorf("lock the rekey: %w", err)
	}
	if !locked {
		return RekeyResult{}, newError(CodeRekeyRunning, 0, "another dhow-server keys rekey is running against this database")
	}
	defer unlockRekey(ctx, conn)
	r := &rekeyer{m: m, conn: conn, values: values, reg: reg, opts: opts}
	return r.runAll(ctx)
}

func unlockRekey(ctx context.Context, conn *pgxpool.Conn) {
	ctx = context.WithoutCancel(ctx)
	if ok, err := sqlc.New(conn).UnlockKeyRekey(ctx); err != nil || !ok {
		_ = conn.Conn().Close(ctx)
	}
}

func (r *rekeyer) runAll(ctx context.Context) (RekeyResult, error) {
	resumed, err := r.open(ctx)
	if err != nil {
		return RekeyResult{}, err
	}
	res := RekeyResult{Run: r.run.Id, Resumed: resumed}
	if res.Superseded, err = r.supersede(ctx); err != nil {
		return res, err
	}
	if err := r.drain(ctx, &res); err != nil {
		return res, err
	}
	ids, err := sqlc.New(r.conn).ListSupersededDataKeyIds(ctx)
	if err != nil {
		return res, fmt.Errorf("list the replaced data keys: %w", err)
	}
	err = dbtx.Run(ctx, r.conn, func(q *sqlc.Queries) error {
		n, err := q.DestroySupersededDataKeys(ctx, ids)
		if err != nil {
			return fmt.Errorf("destroy the replaced data keys: %w", err)
		}
		res.Destroyed = n
		if err := q.AddKeyRekeyProgress(ctx, sqlc.AddKeyRekeyProgressParams{Destroyed: n, ID: r.run.Id}); err != nil {
			return fmt.Errorf("record the rekey: %w", err)
		}
		return q.FinishKeyRekey(ctx, r.run.Id)
	})
	return res, err
}

func (r *rekeyer) open(ctx context.Context) (bool, error) {
	q := sqlc.New(r.conn)
	run, err := q.GetOpenKeyRekey(ctx)
	if err == nil {
		r.run = run
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read the open rekey: %w", err)
	}
	created, err := q.InsertKeyRekey(ctx)
	if err != nil {
		return false, fmt.Errorf("start a rekey: %w", err)
	}
	r.run = sqlc.GetOpenKeyRekeyRow(created)
	return false, nil
}

func (r *rekeyer) supersede(ctx context.Context) (int64, error) {
	var total int64
	for {
		var rows int
		err := dbtx.Run(ctx, r.conn, func(q *sqlc.Queries) error {
			list, err := q.ListDataKeysToSupersede(ctx, sqlc.ListDataKeysToSupersedeParams{Cutoff: r.run.Cutoff, Batch: r.opts.Batch})
			if err != nil {
				return fmt.Errorf("list the data keys to replace: %w", err)
			}
			rows = len(list)
			if rows == 0 {
				return nil
			}
			version, err := q.GetActiveKeyEncryptionKey(ctx)
			if err != nil {
				return fmt.Errorf("read the active KEK version: %w", err)
			}
			var n int64
			for _, row := range list {
				ok, err := r.replaceOne(ctx, q, row, version)
				if err != nil {
					return err
				}
				if ok {
					n++
				}
			}
			total += n
			return q.AddKeyRekeyProgress(ctx, sqlc.AddKeyRekeyProgressParams{Superseded: n, ID: r.run.Id})
		})
		if err != nil || rows < int(r.opts.Batch) {
			return total, err
		}
	}
}

func (r *rekeyer) replaceOne(ctx context.Context, q *sqlc.Queries, row sqlc.ListDataKeysToSupersedeRow, version int32) (bool, error) {
	n, err := q.SupersedeDataKey(ctx, row.Id)
	if err != nil {
		return false, fmt.Errorf("replace data key %s: %w", row.Id, err)
	}
	if n != 1 {
		return false, nil
	}
	owner := ownerFrom(row.OwnerKind, row.OwnerId)
	dk, err := wrapNewDataKey(ctx, r.m.provider, owner, version)
	if err != nil {
		return false, err
	}
	inserted, err := q.InsertDataKey(ctx, sqlc.InsertDataKeyParams{
		ID: dk.ID, OwnerKind: string(owner.Kind), OwnerID: owner.idPtr(), KekVersion: dk.KekVersion, Wrapped: dk.Wrapped,
	})
	if err != nil {
		return false, fmt.Errorf("store the new data key of %s: %w", owner, err)
	}
	if inserted != 1 {
		return false, fmt.Errorf("the new data key of %s was not stored; the active KEK version changed, so run the rekey again", owner)
	}
	return true, nil
}

func (r *rekeyer) drain(ctx context.Context, res *RekeyResult) error {
	for attempt := 0; ; attempt++ {
		ids, err := sqlc.New(r.conn).ListSupersededDataKeyIds(ctx)
		if err != nil {
			return fmt.Errorf("list the replaced data keys: %w", err)
		}
		left, err := r.countUnder(ctx, ids)
		if err != nil || (left == 0 && attempt > 0) {
			return err
		}
		if attempt > rekeyDrainAttempts {
			return newError(CodeRekeyNotDrained, 0, fmt.Sprintf("%d values still use a replaced data key; run the rekey again", left))
		}
		n, err := r.resealPass(ctx, ids)
		res.Resealed += n
		if err != nil {
			return err
		}
		if !r.opts.Wait(ctx, r.opts.Grace) {
			return ctx.Err()
		}
	}
}

func (r *rekeyer) countUnder(ctx context.Context, ids []uuid.UUID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	raw := make([][]byte, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id[:])
	}
	var total int64
	for _, l := range r.reg.Locations() {
		n, err := l.CountUnder(ctx, r.conn, raw)
		if err != nil {
			return 0, fmt.Errorf("count the values of %s: %w", l.Spec().Name(), err)
		}
		total += n
	}
	return total, nil
}

func (r *rekeyer) resealPass(ctx context.Context, ids []uuid.UUID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	replaced := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		replaced[id] = true
	}
	var total int64
	var unreadable []string
	for _, l := range r.reg.Locations() {
		col, err := r.values.Column(l.Spec())
		if err != nil {
			return total, err
		}
		n, bad, err := r.resealLocation(ctx, l, col, replaced)
		total += n
		unreadable = append(unreadable, bad...)
		if err != nil {
			return total, err
		}
	}
	if len(unreadable) > 0 {
		shown := unreadable[:min(len(unreadable), rekeyUnreadableShown)]
		return total, newError(CodeRekeyUnreadable, 0, fmt.Sprintf(
			"%d values under a replaced data key cannot be opened, so their old key is kept: %s; fix or clear them and run the rekey again",
			len(unreadable), strings.Join(shown, ", ")))
	}
	return total, nil
}

func (r *rekeyer) resealLocation(ctx context.Context, l Location, col *Column, replaced map[uuid.UUID]bool) (int64, []string, error) {
	var total int64
	var unreadable []string
	after := ""
	for {
		var rows int
		err := dbtx.RunTx(ctx, r.conn, func(tx pgx.Tx) error {
			values, err := l.Scan(ctx, tx, after, r.opts.Batch, false)
			if err != nil {
				return fmt.Errorf("read the values of %s: %w", l.Spec().Name(), err)
			}
			rows = len(values)
			var n int64
			for _, v := range values {
				after = v.Key
				id, ok := DataKeyIDOf(v.Sealed)
				if !ok || !replaced[id] {
					continue
				}
				sealed, err := col.Reseal(ctx, v.Owner, v.Context, v.Sealed)
				if errors.Is(err, ErrSealed) {
					unreadable = append(unreadable, l.Spec().Name()+" row "+v.Key)
					continue
				}
				if err != nil {
					return err
				}
				changed, err := l.Replace(ctx, tx, v, sealed)
				if err != nil {
					return fmt.Errorf("store a value of %s: %w", l.Spec().Name(), err)
				}
				if changed {
					n++
				}
			}
			total += n
			return sqlc.New(tx).AddKeyRekeyProgress(ctx, sqlc.AddKeyRekeyProgressParams{Resealed: n, ID: r.run.Id})
		})
		if err != nil || rows < int(r.opts.Batch) {
			return total, unreadable, err
		}
	}
}

func waitFor(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
