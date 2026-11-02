package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

type Recovery struct {
	Config  config.Config
	Log     *slog.Logger
	Backoff client.Backoff
	Store   identity.Store
	Promote func(pendingPath, path string, next *identity.Identity) error
}

type Recovered struct {
	Identity *identity.Identity
	Client   *client.Client
}

func serverURL(cfg config.Config, id *identity.Identity) (*url.URL, error) {
	if cfg.URL != nil {
		return cfg.URL, nil
	}
	u, err := config.ParseServerURL(id.ServerURL, cfg.Dev)
	if err != nil {
		return nil, fmt.Errorf("the server address saved at registration: %w", err)
	}
	return u, nil
}

func clientFor(cfg config.Config, id *identity.Identity, log *slog.Logger) (*client.Client, error) {
	u, err := serverURL(cfg, id)
	if err != nil {
		return nil, err
	}
	return client.New(client.Options{BaseURL: u, CAFile: cfg.CAFile, Dev: cfg.Dev, Version: buildinfo.Get().Version, Identity: id, Log: log, Features: runnerFeatures(cfg)})
}

func loadPending(store identity.Store, current *identity.Identity) (*identity.Identity, error) {
	path := store.PendingPath()
	pending, err := store.LoadPending()
	if errors.Is(err, identity.ErrPendingSealed) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("the new identity of an unfinished key rotation (%s): %w", path, err)
	}
	if pending == nil {
		return nil, nil
	}
	if err := current.Matches(pending); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return pending, nil
}

type verdict int

const (
	keepCurrent verdict = iota
	takePending
	neitherKey
)

func checkPending(ctx context.Context, cfg config.Config, log *slog.Logger, current, next *identity.Identity) (verdict, *client.Client, error) {
	old, err := clientFor(cfg, current, log)
	if err != nil {
		return 0, nil, err
	}
	err = old.Authenticate(ctx)
	if err == nil {
		return keepCurrent, old, nil
	}
	old.CloseIdle()
	if !errors.Is(err, client.ErrCredentials) {
		return 0, nil, err
	}
	fresh, err := clientFor(cfg, next, log)
	if err != nil {
		return 0, nil, err
	}
	err = fresh.Authenticate(ctx)
	switch {
	case err == nil:
		return takePending, fresh, nil
	case errors.Is(err, client.ErrCredentials):
		fresh.CloseIdle()
		return neitherKey, nil, nil
	default:
		fresh.CloseIdle()
		return 0, nil, err
	}
}

func (r Recovery) Run(ctx context.Context, current *identity.Identity) (Recovered, error) {
	if r.Log == nil {
		r.Log = slog.New(slog.DiscardHandler)
	}
	if r.Store == nil {
		r.Store = identity.NewFileStore(r.Config.StateDir)
	}
	if r.Promote == nil {
		store := r.Store
		r.Promote = func(_, _ string, next *identity.Identity) error { return store.Promote(next) }
	}
	if r.Backoff == (client.Backoff{}) {
		r.Backoff = client.DefaultBackoff()
	}
	if err := r.Store.RemoveStaleTemps(); err != nil {
		return Recovered{}, err
	}
	pending, err := loadPending(r.Store, current)
	if errors.Is(err, identity.ErrPendingSealed) {
		return r.resolveSealed(ctx, current)
	}
	if err != nil || pending == nil {
		return Recovered{Identity: current}, err
	}
	if current.SameKeys(pending) {
		if err := r.Store.RemovePending(); err != nil {
			return Recovered{}, fmt.Errorf("remove the finished pending identity: %w", err)
		}
		return Recovered{Identity: current}, nil
	}
	next, err := current.Next(pending.Keys)
	if err != nil {
		return Recovered{}, err
	}
	return r.resolve(ctx, current, next)
}

func (r Recovery) resolve(ctx context.Context, current, next *identity.Identity) (Recovered, error) {
	for n := 0; ; n++ {
		v, c, err := checkPending(ctx, r.Config, r.Log, current, next)
		switch {
		case err == nil && v == keepCurrent:
			if err := r.Store.Discard(); err != nil {
				return Recovered{}, err
			}
			r.Log.Warn("removed the new keys of a key rotation the server never took; the runner keeps its current key, and dhow-runner rotate-keys makes fresh ones")
			return Recovered{Identity: current, Client: c}, nil
		case err == nil && v == takePending:
			if err := r.Promote(r.Store.PendingPath(), r.Store.Path(), next); err != nil {
				c.CloseIdle()
				return Recovered{}, err
			}
			r.Log.Info("finished an interrupted key rotation: the server had taken the new keys", "key_version", next.KeyVersion)
			return Recovered{Identity: next, Client: c}, nil
		case err == nil:
			r.Log.Error("the server refuses both the current key and the new key of an unfinished rotation")
			return Recovered{Identity: current}, nil
		case client.Fatal(err) || !client.Transient(err):
			return Recovered{}, err
		}
		r.Log.Warn("could not check an unfinished key rotation with the server yet", "code", client.Code(err).String(), "reason", client.Reason(err))
		if err := r.Backoff.Sleep(ctx, n, err); err != nil {
			return Recovered{}, err
		}
	}
}

func (r Recovery) resolveSealed(ctx context.Context, current *identity.Identity) (Recovered, error) {
	c, err := clientFor(r.Config, current, r.Log)
	if err != nil {
		return Recovered{}, err
	}
	for n := 0; ; n++ {
		err := c.Authenticate(ctx)
		switch {
		case err == nil:
			if err := r.Store.Discard(); err != nil {
				c.CloseIdle()
				return Recovered{}, err
			}
			r.Log.Warn("removed the encrypted new keys of a key rotation the server never took; the runner keeps its current key")
			return Recovered{Identity: current, Client: c}, nil
		case errors.Is(err, client.ErrCredentials):
			c.CloseIdle()
			return Recovered{}, errors.Join(identity.ErrPendingSealed, err)
		case client.Fatal(err) || !client.Transient(err):
			c.CloseIdle()
			return Recovered{}, err
		}
		r.Log.Warn("could not check an unfinished key rotation with the server yet", "code", client.Code(err).String(), "reason", client.Reason(err))
		if err := r.Backoff.Sleep(ctx, n, err); err != nil {
			c.CloseIdle()
			return Recovered{}, err
		}
	}
}
