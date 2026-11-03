package keys

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"slices"
	"strings"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

var ErrDiscardChanged = errors.New("the number of data keys to destroy changed since it was confirmed; nothing changed, run the command again")

type DiscardPlan struct {
	Missing    []int32
	Mismatched []int32
	DataKeys   int64
}

func (p DiscardPlan) Empty() bool { return len(p.Missing) == 0 && len(p.Mismatched) == 0 }

func (p DiscardPlan) Versions() []int32 {
	out := append(slices.Clone(p.Missing), p.Mismatched...)
	slices.Sort(out)
	return out
}

func (p DiscardPlan) Confirmation() string { return fmt.Sprintf("destroy-%d-keys", p.DataKeys) }

type DiscardRecorder func(ctx context.Context, q *sqlc.Queries, plan DiscardPlan) error

func (m *Manager) PlanDiscard(ctx context.Context) (DiscardPlan, error) {
	return m.planDiscard(ctx, sqlc.New(m.db))
}

func (m *Manager) planDiscard(ctx context.Context, q *sqlc.Queries) (DiscardPlan, error) {
	rows, err := q.ListKeyEncryptionKeys(ctx)
	if err != nil {
		return DiscardPlan{}, fmt.Errorf("read the KEK versions: %w", err)
	}
	var plan DiscardPlan
	local := m.provider.Versions()
	for _, r := range rows {
		if !slices.Contains(local, r.Version) {
			if r.State != KekRetired {
				plan.Missing = append(plan.Missing, r.Version)
			}
			continue
		}
		check, err := m.provider.CheckValue(ctx, r.Version)
		if err != nil {
			return DiscardPlan{}, err
		}
		if !hmac.Equal(check, r.CheckValue) {
			plan.Mismatched = append(plan.Mismatched, r.Version)
		}
	}
	if plan.Empty() {
		return plan, nil
	}
	plan.DataKeys, err = q.CountDataKeysOnVersions(ctx, plan.Versions())
	if err != nil {
		return DiscardPlan{}, fmt.Errorf("count the unreadable data keys: %w", err)
	}
	return plan, nil
}

func (m *Manager) Discard(ctx context.Context, confirmed string, staleAfterSeconds float64, record DiscardRecorder) (DiscardPlan, error) {
	var plan DiscardPlan
	err := dbtx.Run(ctx, m.db, func(q *sqlc.Queries) error {
		if err := q.LockKeyring(ctx); err != nil {
			return fmt.Errorf("lock the keyring: %w", err)
		}
		var err error
		if plan, err = m.planDiscard(ctx, q); err != nil || plan.Empty() {
			return err
		}
		if plan.Confirmation() != confirmed {
			return ErrDiscardChanged
		}
		if err := refuseWhileLoaded(ctx, q, plan.Versions(), staleAfterSeconds); err != nil {
			return err
		}
		if err := m.applyDiscard(ctx, q, plan); err != nil {
			return err
		}
		return record(ctx, q, plan)
	})
	return plan, err
}

func refuseWhileLoaded(ctx context.Context, q *sqlc.Queries, versions []int32, staleAfterSeconds float64) error {
	live, err := q.ListLiveServerInstanceKeks(ctx, staleAfterSeconds)
	if err != nil {
		return fmt.Errorf("read the running servers: %w", err)
	}
	var using []string
	for _, inst := range live {
		if slices.ContainsFunc(inst.KekVersions, func(v int32) bool { return slices.Contains(versions, v) }) {
			using = append(using, inst.InstanceKey+" ("+inst.Hostname+")")
		}
	}
	if len(using) > 0 {
		return newError(CodeKeysInUse, 0, "these running servers still hold the versions you want to discard, so their keyring may be the right one: "+
			strings.Join(using, ", ")+"; copy their keyring here instead, or stop them first")
	}
	return nil
}

func (m *Manager) applyDiscard(ctx context.Context, q *sqlc.Queries, plan DiscardPlan) error {
	n, err := q.DestroyDataKeysOnVersions(ctx, plan.Versions())
	if err != nil {
		return fmt.Errorf("mark the unreadable data keys destroyed: %w", err)
	}
	if n != plan.DataKeys {
		return ErrDiscardChanged
	}
	for _, v := range plan.Missing {
		if _, err := q.RetireKeyEncryptionKey(ctx, v); err != nil {
			return fmt.Errorf("retire KEK version %d: %w", v, err)
		}
	}
	for _, v := range plan.Mismatched {
		check, err := m.provider.CheckValue(ctx, v)
		if err != nil {
			return err
		}
		if _, err := q.ReplaceKeyEncryptionKeyCheck(ctx, sqlc.ReplaceKeyEncryptionKeyCheckParams{CheckValue: check, Version: v}); err != nil {
			return fmt.Errorf("record the new key of KEK version %d: %w", v, err)
		}
	}
	return nil
}
