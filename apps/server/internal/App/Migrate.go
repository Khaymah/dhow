package app

import (
	"context"
	"errors"
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
	migrateUsageFmt = "usage: dhow-server migrate %s|%s|%s"
)

var ErrMigrateUsage = fmt.Errorf(migrateUsageFmt, MigrateStatus, MigratePlan, MigrateDryRun)

func RunMigrate(ctx context.Context, args []string, out io.Writer) (retErr error) {
	if len(args) != 1 || (args[0] != MigrateStatus && args[0] != MigratePlan && args[0] != MigrateDryRun) {
		return ErrMigrateUsage
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
	return Migrate(ctx, m, pool, args[0], out)
}

func Migrate(ctx context.Context, m *migrations.Migrator, pool *pgxpool.Pool, cmd string, out io.Writer) error {
	switch cmd {
	case MigrateStatus:
		return printStatus(ctx, m, pool, out)
	case MigratePlan:
		return printPlan(ctx, m, out)
	case MigrateDryRun:
		return printDryRun(ctx, m, out)
	}
	return ErrMigrateUsage
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
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "State:\t%s\n", st.State)
	_, _ = fmt.Fprintf(w, "Database version:\t%d\n", st.DBVersion)
	_, _ = fmt.Fprintf(w, "Binary knows up to:\t%d (dhow %s)\n", st.Latest, st.Binary)
	_, _ = fmt.Fprintf(w, "Schema floor:\t%s\n", st.Floor)
	if st.Reason != "" {
		_, _ = fmt.Fprintf(w, "Why:\t%s\n", st.Reason)
	}
	var pending []string
	for _, e := range migrations.Pending(entries) {
		pending = append(pending, e.Name)
	}
	if len(pending) == 0 {
		pending = []string{"none"}
	}
	_, _ = fmt.Fprintf(w, "Pending:\t%s\n", strings.Join(pending, ", "))
	if st.DBVersion >= migrations.FloorMigration {
		live, err := sqlc.New(pool).ListLiveServerInstances(ctx, config.ServerInstanceStaleAfter.Seconds())
		if err != nil {
			return fmt.Errorf("list the running server instances: %w", err)
		}
		_, _ = fmt.Fprintf(w, "Running servers:\t%d\n", len(live))
		for _, r := range live {
			_, _ = fmt.Fprintf(w, "\t%s  dhow %s  schema %d  last seen %s\n", r.Hostname, r.Version, r.SchemaVersion, r.LastSeenAt.UTC().Format("2006-01-02 15:04:05Z"))
		}
	}
	return w.Flush()
}

func printPlan(ctx context.Context, m *migrations.Migrator, out io.Writer) error {
	entries, err := m.Plan(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "VERSION\tFILE\tPHASE\tFLOOR\tTRANSACTION\tSTATE")
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
		_, _ = fmt.Fprintf(w, "%05d\t%s\t%s\t%s\t%s\t%s\n", e.Version, e.Name, phaseLabel(e.Phase), floor, tx, state)
	}
	return w.Flush()
}

func printDryRun(ctx context.Context, m *migrations.Migrator, out io.Writer) error {
	res, runErr := m.DryRun(ctx)
	if len(res.Steps) == 0 && runErr == nil {
		_, err := fmt.Fprintln(out, "Nothing to try: no migration is pending.")
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "VERSION\tFILE\tPHASE\tRESULT\tTIME")
	for _, s := range res.Steps {
		result := string(s.Outcome)
		if s.Outcome == migrations.DryRunNotTried {
			result = "not tried (NO TRANSACTION)"
		}
		_, _ = fmt.Fprintf(w, "%05d\t%s\t%s\t%s\t%s\n", s.Version, s.Name, phaseLabel(s.Phase), result, s.Duration.Round(time.Millisecond))
	}
	if err := w.Flush(); err != nil {
		return errors.Join(runErr, err)
	}
	_, err := fmt.Fprintln(out, "Every change was rolled back. The database is unchanged.")
	return errors.Join(runErr, err)
}
