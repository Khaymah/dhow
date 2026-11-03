package keys

import (
	"context"
	"crypto/hmac"
	"fmt"
	"slices"
	"time"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

type VersionStatus struct {
	Version        int32
	State          string
	Provider       string
	DataKeys       int64
	Local          bool
	ActivatedAt    *time.Time
	RetiredAt      *time.Time
	LastExportedAt *time.Time
}

type Status struct {
	Provider       string
	Active         int32
	Versions       []VersionStatus
	ToRewrap       int64
	LastRotatedAt  *time.Time
	LastExportedAt *time.Time
	LocalVersions  []int32
}

func (s Status) Removable() []int32 {
	var out []int32
	for _, v := range s.Versions {
		if v.State == KekRetired && v.Local {
			out = append(out, v.Version)
		}
	}
	return out
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	q := sqlc.New(m.db)
	rows, err := q.ListKeyEncryptionKeys(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("read the KEK versions: %w", err)
	}
	counts, err := q.CountDataKeysByVersion(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("count the data keys: %w", err)
	}
	perVersion := map[int32]int64{}
	for _, c := range counts {
		perVersion[c.KekVersion] = c.Count
	}
	local := m.provider.Versions()
	st := Status{Provider: m.provider.Name(), LocalVersions: local}
	for _, r := range rows {
		vs := VersionStatus{
			Version: r.Version, State: r.State, Provider: r.Provider, DataKeys: perVersion[r.Version], Local: slices.Contains(local, r.Version),
			ActivatedAt: r.ActivatedAt, RetiredAt: r.RetiredAt, LastExportedAt: r.LastExportedAt,
		}
		st.Versions = append(st.Versions, vs)
		st.LastRotatedAt = latest(st.LastRotatedAt, r.ActivatedAt)
		st.LastExportedAt = latest(st.LastExportedAt, r.LastExportedAt)
		if r.State == KekActive {
			st.Active = r.Version
		}
	}
	for _, vs := range st.Versions {
		if vs.Version != st.Active {
			st.ToRewrap += vs.DataKeys
		}
	}
	return st, nil
}

func latest(a, b *time.Time) *time.Time {
	if b == nil || (a != nil && !b.After(*a)) {
		return a
	}
	return b
}

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

func (m *Manager) Discard(ctx context.Context) (DiscardPlan, error) {
	var plan DiscardPlan
	err := dbtx.Run(ctx, m.db, func(q *sqlc.Queries) error {
		if err := q.LockKeyring(ctx); err != nil {
			return fmt.Errorf("lock the keyring: %w", err)
		}
		var err error
		if plan, err = m.planDiscard(ctx, q); err != nil || plan.Empty() {
			return err
		}
		if plan.DataKeys, err = q.DestroyDataKeysOnVersions(ctx, plan.Versions()); err != nil {
			return fmt.Errorf("mark the unreadable data keys destroyed: %w", err)
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
	})
	return plan, err
}
