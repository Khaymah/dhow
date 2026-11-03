package keys

import (
	"context"
	"fmt"
	"slices"
	"time"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type VersionStatus struct {
	Version        int32
	State          string
	Provider       string
	DataKeys       int64
	Unreadable     int64
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
	Unreadable     int64
	LastRotatedAt  *time.Time
	LastExportedAt *time.Time
	LocalVersions  []int32
}

func (s Status) Removable() []int32 {
	var out []int32
	for _, v := range s.Versions {
		if v.State == KekRetired && v.Local && v.DataKeys == 0 && v.Unreadable == 0 {
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
	perVersion, unreadable := map[int32]int64{}, map[int32]int64{}
	for _, c := range counts {
		perVersion[c.KekVersion] = c.Count
		unreadable[c.KekVersion] = c.Unreadable
	}
	local := m.provider.Versions()
	st := Status{Provider: m.provider.Name(), LocalVersions: local}
	for _, r := range rows {
		vs := VersionStatus{
			Version: r.Version, State: r.State, Provider: r.Provider, DataKeys: perVersion[r.Version], Unreadable: unreadable[r.Version], Local: slices.Contains(local, r.Version),
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
		st.Unreadable += vs.Unreadable
	}
	return st, nil
}

func latest(a, b *time.Time) *time.Time {
	if b == nil || (a != nil && !b.After(*a)) {
		return a
	}
	return b
}
