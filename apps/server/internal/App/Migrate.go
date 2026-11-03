package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	migrations "github.com/Khaymah/dhow/apps/server/internal/Database/Migrations"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

const (
	CommandMigrate  = "migrate"
	MigrateStatus   = "status"
	MigratePlan     = "plan"
	MigrateDryRun   = "dry-run"
	phaseUnmarked   = "expand (before the rule)"
	migrateUsageFmt = "usage: dhow-server migrate %s|%s|%s [--timeout 60s]"
)

var ErrMigrateUsage = fmt.Errorf(migrateUsageFmt, MigrateStatus, MigratePlan, MigrateDryRun)

type MigrateOptions struct {
	Command string
	Timeout time.Duration
}

func ParseMigrateArgs(args []string) (MigrateOptions, error) {
	if len(args) == 0 {
		return MigrateOptions{}, ErrMigrateUsage
	}
	opts := MigrateOptions{Command: args[0], Timeout: migrations.DefaultDryRunTimeout}
	switch opts.Command {
	case MigrateStatus, MigratePlan:
		if len(args) != 1 {
			return MigrateOptions{}, ErrMigrateUsage
		}
		return opts, nil
	case MigrateDryRun:
		flags := flag.NewFlagSet(MigrateDryRun, flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		flags.DurationVar(&opts.Timeout, "timeout", migrations.DefaultDryRunTimeout, "")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || opts.Timeout <= 0 {
			return MigrateOptions{}, ErrMigrateUsage
		}
		return opts, nil
	}
	return MigrateOptions{}, ErrMigrateUsage
}

func RunMigrate(ctx context.Context, args []string, out io.Writer) (retErr error) {
	opts, err := ParseMigrateArgs(args)
	if err != nil {
		return err
	}
	cfg, err := config.LoadMigrateFromEnv()
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	pool, err := config.OpenDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	m, err := migrations.NewMigrator(pool, migrations.Binary{Version: buildinfo.Get().Version, Dev: cfg.Dev})
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, m.Close()) }()
	return Migrate(ctx, m, pool, opts, out)
}

func Migrate(ctx context.Context, m *migrations.Migrator, pool *pgxpool.Pool, opts MigrateOptions, out io.Writer) error {
	switch opts.Command {
	case MigrateStatus:
		return printStatus(ctx, m, pool, out)
	case MigratePlan:
		return printPlan(ctx, m, out)
	case MigrateDryRun:
		return printDryRun(ctx, m, opts.Timeout, out)
	}
	return ErrMigrateUsage
}

type printer struct {
	w   *tabwriter.Writer
	err error
}

func newPrinter(out io.Writer) *printer {
	return &printer{w: tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)}
}

func (p *printer) row(format string, args ...any) {
	if p.err != nil {
		return
	}
	if _, err := fmt.Fprintf(p.w, format+"\n", args...); err != nil {
		p.err = fmt.Errorf("write the output: %w", err)
	}
}

func (p *printer) flush() error {
	if p.err != nil {
		return p.err
	}
	if err := p.w.Flush(); err != nil {
		return fmt.Errorf("write the output: %w", err)
	}
	return nil
}

func phaseLabel(phase string) string {
	if phase == "" {
		return phaseUnmarked
	}
	return phase
}

func printStatus(ctx context.Context, m *migrations.Migrator, pool *pgxpool.Pool, out io.Writer) error {
	st, err := m.Status(ctx)
	if err != nil {
		return err
	}
	entries, err := m.Plan(ctx)
	if err != nil {
		return err
	}
	p := newPrinter(out)
	p.row("State:\t%s", st.State)
	p.row("Database version:\t%d", st.DBVersion)
	p.row("Binary knows up to:\t%d (dhow %s)", st.Latest, st.Binary)
	p.row("Schema floor:\t%s", st.Floor)
	if st.Reason != "" {
		p.row("Why:\t%s", st.Reason)
	}
	pending := []string{}
	for _, e := range migrations.Pending(entries) {
		pending = append(pending, e.Name)
	}
	if len(pending) == 0 {
		pending = []string{"none"}
	}
	p.row("Pending:\t%s", strings.Join(pending, ", "))
	if st.DBVersion >= migrations.FloorMigration {
		live, err := sqlc.New(pool).ListLiveServerInstances(ctx, config.ServerInstanceStaleAfter.Seconds())
		if err != nil {
			return fmt.Errorf("list the running server instances: %w", err)
		}
		p.row("Running servers:\t%d", len(live))
		for _, r := range live {
			p.row("\t%s (%s)  dhow %s  schema %d  last seen %s", r.InstanceKey, r.Hostname, r.Version, r.SchemaVersion,
				r.LastSeenAt.UTC().Format("2006-01-02 15:04:05Z"))
		}
	}
	return p.flush()
}

func printPlan(ctx context.Context, m *migrations.Migrator, out io.Writer) error {
	entries, err := m.Plan(ctx)
	if err != nil {
		return err
	}
	p := newPrinter(out)
	p.row("VERSION\tFILE\tPHASE\tFLOOR\tTRANSACTION\tSTATE")
	for _, e := range entries {
		tx, state, floor := "yes", "pending", e.Floor
		if !e.UseTx {
			tx = "no"
		}
		if e.Applied {
			state = "applied"
		}
		if floor == "" {
			floor = "-"
		}
		p.row("%05d\t%s\t%s\t%s\t%s\t%s", e.Version, e.Name, phaseLabel(e.Phase), floor, tx, state)
	}
	return p.flush()
}

func printDryRun(ctx context.Context, m *migrations.Migrator, timeout time.Duration, out io.Writer) error {
	res, runErr := m.DryRun(ctx, migrations.DryRunOptions{Timeout: timeout})
	p := newPrinter(out)
	if len(res.Steps) == 0 && runErr == nil {
		p.row("Nothing to try: no migration is pending.")
		return p.flush()
	}
	if len(res.Steps) > 0 {
		p.row("VERSION\tFILE\tPHASE\tRESULT\tTIME")
	}
	for _, s := range res.Steps {
		result := string(s.Outcome)
		if s.Outcome == migrations.DryRunNotTried {
			result = "not tried (NO TRANSACTION)"
		}
		p.row("%05d\t%s\t%s\t%s\t%s", s.Version, s.Name, phaseLabel(s.Phase), result, s.Duration.Round(time.Millisecond))
	}
	if errors.Is(runErr, migrations.ErrTransactionEnded) {
		p.row("A statement ended the transaction, so the statements before it may be committed. Check the database before you upgrade.")
	} else {
		p.row("Every change was rolled back. The database is unchanged.")
	}
	return errors.Join(runErr, p.flush())
}
