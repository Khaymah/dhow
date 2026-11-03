package instancesettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
)

type Source string

const (
	SourceEnv      Source = "env"
	SourceDatabase Source = "database"
	SourceDefault  Source = "default"

	ChangedChannel = "dhow_settings_changed"
	contextPrefix  = "instance-settings:"
)

var (
	ErrUndecryptable = errors.New("cannot be decrypted with the keyring, or with DHOW_SECRET_KEY for a value saved before the keyring")
	ErrReloadFailed  = errors.New("the last instance settings reload failed, so this instance may serve stale settings")
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Listener func(ctx context.Context) (*pgx.Conn, error)

type Stored struct {
	Raw       settingspec.Raw
	Sealed    bool
	Version   int64
	UpdatedAt *time.Time
	UpdatedBy *uuid.UUID
}

type Snapshot struct {
	Values  settingspec.Settings
	Sources map[string]Source
	Stored  map[settingspec.Group]Stored
	Oidc    Oidc
	env     settingspec.Raw
}

var passwordBound = []string{settingspec.EnvSMTPHost, settingspec.EnvSMTPPort, settingspec.EnvSMTPTLS}

func (s *Snapshot) LockedBy(env string) string {
	if s.env.Has(env) {
		return env
	}
	if slices.Contains(passwordBound, env) && s.env.Has(settingspec.EnvSMTPPassword) {
		return settingspec.EnvSMTPPassword
	}
	return ""
}

func (s *Snapshot) Locked(env string) bool { return s.LockedBy(env) != "" }

func (s *Snapshot) SecretStored(env string) bool {
	k, ok := settingspec.KeyByEnv(env)
	if !ok || !k.Secret {
		return false
	}
	st := s.Stored[k.Group]
	return st.Sealed || st.Raw.Get(env) != ""
}

func (s *Snapshot) SecretSet(env string) bool {
	if s.env.Get(env) != "" {
		return true
	}
	k, ok := settingspec.KeyByEnv(env)
	return ok && s.Stored[k.Group].Raw.Get(env) != ""
}

func (s *Snapshot) Version(g settingspec.Group) int64 { return s.Stored[g].Version }

func (s *Snapshot) Text(env string) string {
	k, ok := settingspec.KeyByEnv(env)
	if !ok || k.Secret {
		return ""
	}
	if v, ok := s.env[env]; ok {
		return v
	}
	return s.Stored[k.Group].Raw.Get(env)
}

func (s *Snapshot) merged() settingspec.Raw {
	out := settingspec.Raw{}
	for _, st := range s.Stored {
		for k, v := range st.Raw {
			out[k] = v
		}
	}
	for k, v := range s.env {
		out[k] = v
	}
	return out
}

func (s *Snapshot) Instance() policy.InstanceSettings {
	return policy.InstanceSettings{AnyUserCreatesOrganizations: s.Values.AnyUserCreatesOrganizations()}
}

type Options struct {
	DB      DB
	Values  *keys.Values
	Env     settingspec.Raw
	EnvOIDC []settingspec.OIDCProvider
	Dev     bool
	Listen  Listener
	Log     *slog.Logger
	Now     func() time.Time
	Wait    func(ctx context.Context, d time.Duration) bool
}

type Store struct {
	db       DB
	secrets  *keys.Column
	oidcKeys *keys.Column
	env      settingspec.Raw
	envOIDC  []settingspec.OIDCProvider
	dev      bool
	listen   Listener
	log      *slog.Logger
	now      func() time.Time
	wait     func(ctx context.Context, d time.Duration) bool
	failed   atomic.Bool
	mu       sync.Mutex
	watchers []func(*Snapshot)
	current  atomic.Pointer[Snapshot]
}

func New(o Options) (*Store, error) {
	if o.Values == nil {
		return nil, errors.New("instance settings need the keyring to seal their secrets")
	}
	secrets, err := o.Values.Column(SecretsSpec())
	if err != nil {
		return nil, err
	}
	oidcKeys, err := o.Values.Column(OidcSecretSpec())
	if err != nil {
		return nil, err
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	wait := o.Wait
	if wait == nil {
		wait = sleep
	}
	env := o.Env.Clone()
	s := &Store{
		db: o.DB, secrets: secrets, oidcKeys: oidcKeys, env: env, envOIDC: append([]settingspec.OIDCProvider(nil), o.EnvOIDC...), dev: o.Dev,
		listen: o.Listen, log: o.Log, now: now, wait: wait,
	}
	snap, err := s.build(map[settingspec.Group]Stored{}, s.emptyOidc())
	if err != nil && s.db == nil {
		return nil, err
	}
	if err != nil {
		snap = &Snapshot{
			Values: settingspec.Defaults(), Sources: map[string]Source{}, Stored: map[settingspec.Group]Stored{}, Oidc: s.emptyOidc(), env: env,
		}
	}
	s.current.Store(snap)
	return s, nil
}

func (s *Store) Current() *Snapshot { return s.current.Load() }

func (s *Store) Instance() policy.InstanceSettings { return s.Current().Instance() }

func (s *Store) Load(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.load(ctx)
	if err != nil && ctx.Err() == nil {
		s.failed.Store(true)
	}
	if err == nil {
		s.failed.Store(false)
	}
	return err
}

func (s *Store) load(ctx context.Context) error {
	q := sqlc.New(s.db)
	stored, err := s.read(ctx, q)
	if err != nil {
		return err
	}
	oidc, err := s.readOidc(ctx, q)
	if err != nil {
		return err
	}
	snap, err := s.build(stored, oidc)
	if err != nil {
		return err
	}
	s.publish(snap)
	return nil
}

func (s *Store) read(ctx context.Context, q *sqlc.Queries) (map[settingspec.Group]Stored, error) {
	rows, err := q.ListInstanceSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read instance settings: %w", err)
	}
	out := make(map[settingspec.Group]Stored, len(rows))
	var errs []error
	for _, r := range rows {
		g, ok := settingspec.ParseGroup(r.Group)
		if !ok {
			errs = append(errs, fmt.Errorf("instance setting group %q in the database is unknown", r.Group))
			continue
		}
		raw, unread, err := s.decodeRow(ctx, g, r.Value, r.Secrets)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		at := r.UpdatedAt
		out[g] = Stored{Raw: raw, Sealed: unread, Version: r.Version, UpdatedAt: &at, UpdatedBy: r.UpdatedBy}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

func (s *Store) build(stored map[settingspec.Group]Stored, oidc Oidc) (*Snapshot, error) {
	snap := &Snapshot{Stored: stored, Oidc: oidc, env: s.env, Sources: make(map[string]Source, len(settingspec.Keys()))}
	for _, k := range settingspec.Keys() {
		switch {
		case s.env.Has(k.Env):
			snap.Sources[k.Env] = SourceEnv
		case stored[k.Group].Raw.Has(k.Env):
			snap.Sources[k.Env] = SourceDatabase
		default:
			snap.Sources[k.Env] = SourceDefault
		}
	}
	values, errs := settingspec.Parse(snap.merged())
	if len(errs) > 0 {
		out := make([]error, 0, len(errs))
		for _, e := range errs {
			out = append(out, fmt.Errorf("instance setting %s (from the %s): %s", e.Env, describe(snap.Sources[e.Env]), e.Message))
		}
		return nil, errors.Join(out...)
	}
	snap.Values = values
	return snap, nil
}

func describe(src Source) string {
	switch src {
	case SourceEnv:
		return "environment"
	case SourceDatabase:
		return "database"
	default:
		return "default"
	}
}

func sealContext(g settingspec.Group) []byte { return groupContext(string(g)) }

func (s *Store) secretsLocked(g settingspec.Group) bool {
	found := false
	for _, k := range settingspec.KeysOf(g) {
		if !k.Secret {
			continue
		}
		if !s.env.Has(k.Env) {
			return false
		}
		found = true
	}
	return found
}

func (s *Store) decodeRow(ctx context.Context, g settingspec.Group, value, secrets []byte) (settingspec.Raw, bool, error) {
	if len(secrets) > 0 && s.secretsLocked(g) {
		raw, err := s.decode(ctx, g, value, nil)
		return raw, true, err
	}
	raw, err := s.decode(ctx, g, value, secrets)
	return raw, false, err
}

func (s *Store) decode(ctx context.Context, g settingspec.Group, value, secrets []byte) (settingspec.Raw, error) {
	raw := settingspec.Raw{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil {
		return nil, fmt.Errorf("instance setting group %s in the database is not a JSON object", g)
	}
	for name, v := range fields {
		k, ok := settingspec.KeyByField(g, name)
		if !ok || k.Secret {
			return nil, fmt.Errorf("instance setting %s.%s in the database is unknown", g, name)
		}
		text, err := textOf(k, v)
		if err != nil {
			return nil, fmt.Errorf("instance setting %s (from the database): %w", k.Env, err)
		}
		if e := settingspec.ValidateValue(k.Env, text); e != nil {
			return nil, fmt.Errorf("instance setting %s (from the database): %s", k.Env, e.Message)
		}
		raw[k.Env] = text
	}
	if len(secrets) == 0 {
		return raw, nil
	}
	plain, err := s.secrets.Open(ctx, keys.InstanceOwner(), sealContext(g), secrets)
	if err != nil {
		return nil, fmt.Errorf("instance setting secrets of %s (from the database): %w", g, ErrUndecryptable)
	}
	defer clear(plain)
	var sealed map[string]string
	if err := json.Unmarshal(plain, &sealed); err != nil {
		return nil, fmt.Errorf("instance setting secrets of %s (from the database) are malformed", g)
	}
	for name, v := range sealed {
		k, ok := settingspec.KeyByField(g, name)
		if !ok || !k.Secret {
			return nil, fmt.Errorf("instance setting secret %s.%s in the database is unknown", g, name)
		}
		if e := settingspec.ValidateValue(k.Env, v); e != nil {
			return nil, fmt.Errorf("instance setting %s (from the database): %s", k.Env, e.Message)
		}
		raw[k.Env] = v
	}
	return raw, nil
}
