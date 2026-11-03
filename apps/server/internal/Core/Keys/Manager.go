package keys

import (
	"context"
	"crypto/hmac"
	"fmt"
	"log/slog"
	"slices"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

const (
	KekPending  = "pending"
	KekActive   = "active"
	KekRetiring = "retiring"
	KekRetired  = "retired"
)

type Manager struct {
	db       DB
	provider KeyProvider
	log      *slog.Logger
	envelope *Envelope
}

func NewManager(db DB, provider KeyProvider, log *slog.Logger) *Manager {
	return &Manager{db: db, provider: provider, log: log, envelope: NewEnvelope(NewPostgresStore(db), provider)}
}

func (m *Manager) Envelope() *Envelope { return m.envelope }

func (m *Manager) Provider() KeyProvider { return m.provider }

type Registration struct {
	Versions   []int32
	Registered []int32
	Active     int32
	Activated  bool
}

func (m *Manager) Register(ctx context.Context) (Registration, error) {
	var reg Registration
	err := dbtx.Run(ctx, m.db, func(q *sqlc.Queries) error {
		if err := q.LockKeyring(ctx); err != nil {
			return fmt.Errorf("lock the keyring: %w", err)
		}
		rows, err := q.ListKeyEncryptionKeys(ctx)
		if err != nil {
			return fmt.Errorf("read the KEK versions: %w", err)
		}
		if err := m.verifyRows(ctx, rows, true); err != nil {
			return err
		}
		reg, err = m.registerMissing(ctx, q, rows)
		return err
	})
	if err != nil {
		return Registration{}, err
	}
	reg.Versions = m.provider.Versions()
	return reg, nil
}

func (m *Manager) registerMissing(ctx context.Context, q *sqlc.Queries, rows []sqlc.ListKeyEncryptionKeysRow) (Registration, error) {
	var reg Registration
	known := map[int32]bool{}
	for _, r := range rows {
		known[r.Version] = true
		if r.State == KekActive {
			reg.Active = r.Version
		}
	}
	for _, v := range m.provider.Versions() {
		if known[v] {
			continue
		}
		check, err := m.provider.CheckValue(ctx, v)
		if err != nil {
			return Registration{}, err
		}
		if err := q.InsertKeyEncryptionKey(ctx, sqlc.InsertKeyEncryptionKeyParams{
			Version: v, CheckValue: check, Provider: m.provider.Name(), State: KekPending,
		}); err != nil {
			return Registration{}, fmt.Errorf("register KEK version %d: %w", v, err)
		}
		reg.Registered = append(reg.Registered, v)
	}
	if reg.Active != 0 {
		return reg, nil
	}
	versions := m.provider.Versions()
	if len(versions) == 0 {
		return Registration{}, newError(CodeKeyringMalformed, 0, "the keyring holds no versions")
	}
	latest := versions[len(versions)-1]
	if _, err := q.PromoteKeyEncryptionKey(ctx, latest); err != nil {
		return Registration{}, fmt.Errorf("activate KEK version %d: %w", latest, err)
	}
	reg.Active, reg.Activated = latest, true
	return reg, nil
}

func (m *Manager) verifyRows(ctx context.Context, rows []sqlc.ListKeyEncryptionKeysRow, everyLive bool) error {
	have := map[int32]bool{}
	for _, v := range m.provider.Versions() {
		have[v] = true
	}
	for _, r := range rows {
		needed := r.State == KekActive || r.State == KekRetiring || (everyLive && r.State == KekPending)
		if !have[r.Version] {
			if needed {
				return newError(CodeKeyringMissingVersion, r.Version,
					fmt.Sprintf("the database needs this %s version and the keyring file does not have it; restore the keyring file that holds it", r.State))
			}
			continue
		}
		check, err := m.provider.CheckValue(ctx, r.Version)
		if err != nil {
			return err
		}
		if !hmac.Equal(check, r.CheckValue) {
			return newError(CodeKeyringMismatch, r.Version,
				"the key in the keyring file does not match the check value in the database; this is not the keyring of this database")
		}
	}
	return nil
}

func (m *Manager) Verify(ctx context.Context) error {
	rows, err := sqlc.New(m.db).ListKeyEncryptionKeys(ctx)
	if err != nil {
		return fmt.Errorf("read the KEK versions: %w", err)
	}
	return m.verifyRows(ctx, rows, false)
}

type Activation struct {
	From int32
	To   int32
}

func (m *Manager) Activate(ctx context.Context, version int32, staleAfterSeconds float64) (Activation, error) {
	var act Activation
	err := dbtx.Run(ctx, m.db, func(q *sqlc.Queries) error {
		if err := q.LockKeyring(ctx); err != nil {
			return fmt.Errorf("lock the keyring: %w", err)
		}
		rows, err := q.ListKeyEncryptionKeys(ctx)
		if err != nil {
			return fmt.Errorf("read the KEK versions: %w", err)
		}
		from, err := m.checkActivation(ctx, rows, version)
		if err != nil {
			return err
		}
		if err := checkEverywhere(ctx, q, version, staleAfterSeconds); err != nil {
			return err
		}
		if _, err := q.DemoteActiveKeyEncryptionKey(ctx); err != nil {
			return fmt.Errorf("retire the active KEK version: %w", err)
		}
		n, err := q.PromoteKeyEncryptionKey(ctx, version)
		if err != nil {
			return fmt.Errorf("activate KEK version %d: %w", version, err)
		}
		if n != 1 {
			return newError(CodeKekNotFound, version, "this version could not be activated")
		}
		act = Activation{From: from, To: version}
		return nil
	})
	return act, err
}

func (m *Manager) checkActivation(ctx context.Context, rows []sqlc.ListKeyEncryptionKeysRow, version int32) (int32, error) {
	var from int32
	var target *sqlc.ListKeyEncryptionKeysRow
	for i := range rows {
		if rows[i].State == KekActive {
			from = rows[i].Version
		}
		if rows[i].Version == version {
			target = &rows[i]
		}
	}
	switch {
	case target == nil:
		return 0, newError(CodeKekNotFound, version,
			"the database does not know this version; add it with dhow-server keys generate, copy the keyring to every server and restart them first")
	case target.State == KekActive:
		return 0, newError(CodeKekAlreadyActive, version, "this version is already active")
	case target.State == KekRetired:
		return 0, newError(CodeKekRetired, version, "a retired version cannot be activated again; generate a new one")
	}
	if !slices.Contains(m.provider.Versions(), version) {
		return 0, newError(CodeKeyringMissingVersion, version, "the keyring file used by this command does not have this version")
	}
	if err := m.verifyRows(ctx, []sqlc.ListKeyEncryptionKeysRow{*target}, true); err != nil {
		return 0, err
	}
	return from, nil
}

func checkEverywhere(ctx context.Context, q *sqlc.Queries, version int32, staleAfterSeconds float64) error {
	live, err := q.ListLiveServerInstanceKeks(ctx, staleAfterSeconds)
	if err != nil {
		return fmt.Errorf("read the running servers: %w", err)
	}
	var missing []string
	for _, inst := range live {
		if !slices.Contains(inst.KekVersions, version) {
			missing = append(missing, inst.InstanceKey+" ("+inst.Hostname+")")
		}
	}
	if len(missing) > 0 {
		return &NotEverywhereError{Version: version, Instances: missing}
	}
	return nil
}

func (m *Manager) MarkExported(ctx context.Context, versions []int32) error {
	if _, err := sqlc.New(m.db).MarkKeyringExported(ctx, versions); err != nil {
		return fmt.Errorf("record the export: %w", err)
	}
	return nil
}
