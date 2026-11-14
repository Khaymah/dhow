package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	executor "github.com/Khaymah/dhow/apps/runner/internal/Executor"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	journal "github.com/Khaymah/dhow/apps/runner/internal/Journal"
	logs "github.com/Khaymah/dhow/apps/runner/internal/Logs"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

type Runner struct {
	cfg      config.Config
	id       *identity.Identity
	client   *client.Client
	driver   sandbox.Driver
	journal  *journal.Journal
	exec     *executor.Executor
	log      *slog.Logger
	backoff  client.Backoff
	wait     time.Duration
	outcomes chan<- executor.Outcome
}

type RunnerOptions struct {
	Config   config.Config
	Identity *identity.Identity
	Driver   sandbox.Driver
	Log      *slog.Logger
	Timing   executor.Timing
	Backoff  client.Backoff
	Wait     time.Duration
	Outcomes chan<- executor.Outcome
	Logs     func(*logs.Options)
	Client   *client.Client
}

func runRun(ctx context.Context, args []string, env Env) error {
	fs := newFlags(CommandRun, env.Stdio.Err)
	dev := fs.Bool("dev", false, "allow plain http:// to localhost, for local development only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%w: %s", ErrExtraArgs, strings.Join(fs.Args(), " "))
	}
	if err := env.Harden(); err != nil {
		return err
	}
	machine, err := env.Machine()
	if err != nil {
		return err
	}
	cfg, err := config.Load(env.Getenv, config.Options{Dev: *dev, Machine: machine})
	if err != nil {
		return err
	}
	log := newLogger(env.Stdio.Err, cfg.LogLevel)
	store, err := openStore(cfg, env)
	if err != nil {
		return err
	}
	if _, err := store.Load(); err != nil {
		return err
	}
	lock, err := config.LockState(cfg.LockPath())
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	id, err := store.Load()
	if err != nil {
		return err
	}
	driver, err := newDriver(ctx, cfg, log)
	if err != nil {
		return err
	}
	recovered, err := Recovery{Config: cfg, Log: log, Store: store}.Run(ctx, id)
	if err != nil {
		return err
	}
	r, err := NewRunner(RunnerOptions{Config: cfg, Identity: recovered.Identity, Client: recovered.Client, Driver: driver, Log: log})
	if err != nil {
		return err
	}
	return r.Run(ctx)
}

func NewRunner(o RunnerOptions) (*Runner, error) {
	cfg := o.Config
	u, err := serverURL(cfg, o.Identity)
	if err != nil {
		return nil, err
	}
	cfg.URL = u
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Backoff == (client.Backoff{}) {
		o.Backoff = client.DefaultBackoff()
	}
	if o.Wait == 0 {
		o.Wait = config.LeaseWait
	}
	log := o.Log.With("runner_id", o.Identity.RunnerID)
	c := o.Client
	if c == nil {
		if c, err = clientFor(cfg, o.Identity, log); err != nil {
			return nil, err
		}
	}
	if err := config.EnsurePrivateDir(cfg.StateDir); err != nil {
		return nil, err
	}
	j, err := journal.Open(cfg.JournalPath())
	if err != nil {
		return nil, err
	}
	exec := executor.New(executor.Options{
		API: c, Identity: o.Identity, Journal: j, Driver: o.Driver, Config: cfg, Log: log, Backoff: o.Backoff, Timing: o.Timing,
		LogOptions: o.Logs,
	})
	return &Runner{
		cfg: cfg, id: o.Identity, client: c, driver: o.Driver, journal: j, exec: exec, log: log, backoff: o.Backoff,
		wait: o.Wait, outcomes: o.Outcomes,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	defer r.client.CloseIdle()
	if err := r.authenticate(ctx); err != nil {
		return err
	}
	if err := r.cleanup(ctx); err != nil {
		return err
	}
	left, err := r.exec.Recover(ctx)
	if err != nil {
		return err
	}
	if len(left) > 0 {
		r.log.Warn("some attempts from the last run could not be reported yet; the runner keeps trying while it works", "count", len(left))
		var later sync.WaitGroup
		defer later.Wait()
		laterCtx, stopLater := context.WithCancel(ctx)
		defer stopLater()
		later.Go(func() { r.exec.RecoverLater(laterCtx, left) })
	}
	r.log.Info("the runner is ready", "capacity", r.cfg.Capacity, "driver", r.driver.Name())
	return r.loop(ctx)
}

func (r *Runner) authenticate(ctx context.Context) error {
	for n := 0; ; n++ {
		err := r.client.Authenticate(ctx)
		switch {
		case err == nil:
			return nil
		case client.Fatal(err) || !client.Transient(err):
			return err
		}
		r.log.Warn("could not sign in to the server yet", "code", client.Code(err).String(), "reason", client.Reason(err))
		if err := r.backoff.Sleep(ctx, n, err); err != nil {
			return err
		}
	}
}

func (r *Runner) cleanup(ctx context.Context) error {
	if err := r.driver.Cleanup(ctx, r.id.RunnerID); err != nil {
		return fmt.Errorf("remove the sandboxes the last run left behind: %w", err)
	}
	if err := os.RemoveAll(r.cfg.AttemptsDir()); err != nil {
		return fmt.Errorf("remove the attempt directories the last run left behind: %w", err)
	}
	return config.EnsurePrivateDir(r.cfg.AttemptsDir())
}

func (r *Runner) loop(ctx context.Context) error {
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, r.cfg.Capacity)
	freed := make(chan struct{}, 1)
	leaseCtx, quarantine := context.WithCancel(ctx)
	defer quarantine()
	for n := 0; leaseCtx.Err() == nil; {
		free := r.cfg.Capacity - len(slots)
		if free == 0 {
			select {
			case <-freed:
			case <-leaseCtx.Done():
			}
			continue
		}
		sent := time.Now()
		grant, err := r.client.Lease(leaseCtx, int32(free), r.wait) //nolint:gosec // At most 16 slots.
		if err != nil {
			if leaseCtx.Err() != nil {
				break
			}
			if client.Fatal(err) {
				r.log.Error("the server stopped this runner", "error", err)
				return err
			}
			r.log.Warn("could not ask for work", "code", client.Code(err).String(), "reason", client.Reason(err))
			if r.backoff.Sleep(ctx, n, err) != nil {
				break
			}
			n++
			continue
		}
		n = 0
		if grant == nil || leaseCtx.Err() != nil {
			continue
		}
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := r.exec.Run(ctx, grant, sent)
			if out.Survivors {
				r.log.Error("processes of a job survived the kill; the runner takes no more work and stops once its other jobs end", "lease", out.LeaseID)
				quarantine()
			}
			<-slots
			select {
			case freed <- struct{}{}:
			default:
			}
			if r.outcomes != nil {
				r.outcomes <- out
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	if leaseCtx.Err() != nil && ctx.Err() == nil {
		return fmt.Errorf("stop the runner: %w", sandbox.ErrSurvivors)
	}
	return nil
}
