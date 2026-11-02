package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	engine "github.com/Khaymah/dhow/internal/installer"
)

const setupURLKey = "setup_url"

var ErrBlocked = errors.New("the install cannot continue")

type App struct {
	Sys     System
	DB      DatabaseChecker
	In      io.Reader
	InFD    int
	Out     io.Writer
	Locale  string
	Version string
	Sleep   func(time.Duration)
}

type InstallOptions struct {
	Answers string
	DryRun  bool
	Yes     bool
	Lang    string
}

func (app App) say(format string, args ...any) { _, _ = fmt.Fprintf(app.Out, format, args...) }

func (app App) Install(ctx context.Context, opts InstallOptions) error {
	det := Detect(ctx, app.Sys)
	a, err := app.initialAnswers(opts)
	if err != nil {
		return err
	}
	m := NewMessages(ChooseLang(opts.Lang, a.Lang, app.Locale))
	if !opts.DryRun && !det.Root {
		return errors.New(m.T(KeyNeedRoot))
	}
	app.say("%s\n\n", m.T(KeyWelcome, app.Version))
	app.printDetection(det, m)
	if opts.Answers == "" {
		iv := Interview{
			P: NewPrompter(app.In, app.Out, app.InFD, m), M: m, Det: det, DB: app.DB, Out: app.Out,
			KeepDatabaseURL: exists(app.Sys, DatabaseURLPath),
		}
		if a, err = iv.Run(ctx, a); err != nil {
			return err
		}
	} else if err := app.resolveDatabaseURL(ctx, &a, m); err != nil {
		return err
	}
	exe, err := app.Sys.Executable()
	if err != nil {
		return fmt.Errorf("find the running binary: %w", err)
	}
	pgdg := CheckPGDG(ctx, app.Sys, det, a)
	b := &Builder{Sys: app.Sys, Det: det, A: a, M: m, Version: app.Version, Exe: exe, Sleep: app.Sleep, PGDG: pgdg}
	plan := b.Build()
	app.printPlan(ctx, plan, m)
	if len(plan.Blockers) > 0 {
		if pgdg.Support == PGDGMissing {
			return fmt.Errorf("%w: %w", ErrBlocked, ErrPGDGUnsupported)
		}
		return ErrBlocked
	}
	if opts.DryRun {
		app.say("\n%s\n", m.T(KeyDryRunDone))
		return nil
	}
	if !opts.Yes {
		ok, err := NewPrompter(app.In, app.Out, app.InFD, m).Confirm(m.T(KeyConfirmPlan), false)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New(m.T(KeyAborted))
		}
	}
	app.say("\n")
	if err := (Runner{Out: app.Out, M: m}).Execute(ctx, plan.Steps); err != nil {
		return fmt.Errorf("%w\n%s", err, m.T(KeyRerunHint))
	}
	app.finish(ctx, a, m)
	return nil
}

func (app App) initialAnswers(opts InstallOptions) (Answers, error) {
	if opts.Answers != "" {
		data, err := app.Sys.ReadFile(opts.Answers)
		if err != nil {
			return Answers{}, fmt.Errorf("read answers: %w", err)
		}
		return ParseInstalledAnswers(bytes.NewReader(data), exists(app.Sys, DatabaseURLPath))
	}
	if data, err := app.Sys.ReadFile(AnswersPath); err == nil {
		if a, err := ParseInstalledAnswers(bytes.NewReader(data), exists(app.Sys, DatabaseURLPath)); err == nil {
			return a, nil
		}
	}
	return DefaultAnswers(), nil
}

func (app App) resolveDatabaseURL(ctx context.Context, a *Answers, m Messages) error {
	if a.Mode != ModeNative || a.Database.Mode != DatabaseExisting {
		return nil
	}
	if a.Database.URLFile == "" {
		app.say("%s\n", m.T(KeyDatabaseKept, DatabaseURLPath))
		return nil
	}
	raw, err := readSecret(app.Sys, a.Database.URLFile)
	if err != nil {
		return fmt.Errorf("database.url_file: %w", err)
	}
	if err := config.ValidateDatabaseURL(raw); err != nil {
		return fmt.Errorf("database.url_file: %w", err)
	}
	if app.DB != nil {
		major, err := app.DB.ServerMajor(ctx, raw)
		if err != nil {
			return errors.New(m.T(KeyDatabaseUnreachable, err.Error()))
		}
		if major < RequiredPostgres {
			return errors.New(m.T(KeyDatabaseTooOld, major, RequiredPostgres))
		}
		app.say("%s\n", m.T(KeyDatabaseOK, major, sslModeOf(raw)))
	}
	a.SetDatabaseURL(raw)
	return nil
}

func yesNo(m Messages, v bool) string { return m.YesNo(v) }

func (app App) printDetection(det Detection, m Messages) {
	app.say("%s\n", m.T(KeyDetected))
	name := det.Distro.PrettyName
	if name == "" {
		name = "?"
	}
	support := m.T(KeySupported)
	if !det.Distro.Supported {
		support = m.T(KeyManualMode)
	}
	pg := m.T(KeyNone)
	if len(det.Postgres.Versions) > 0 {
		var vs []string
		for _, v := range det.Postgres.Versions {
			vs = append(vs, strconv.Itoa(v))
		}
		pg = strings.Join(vs, ", ")
	}
	var busy []string
	for _, p := range WatchedPorts {
		if det.BusyPort[p] {
			busy = append(busy, strconv.Itoa(p))
		}
	}
	firewall := det.Firewall
	if firewall == "" {
		firewall = m.T(KeyNone)
	}
	rows := [][2]string{
		{m.T(KeyRowSystem), name + " (" + support + ")"},
		{"systemd", yesNo(m, det.Systemd)},
		{m.T(KeyRowRoot), yesNo(m, det.Root)},
		{"Docker / Compose", yesNo(m, det.Docker) + " / " + yesNo(m, det.Compose)},
		{"PostgreSQL", pg},
		{"nginx / Caddy / certbot", yesNo(m, det.Nginx) + " / " + yesNo(m, det.Caddy) + " / " + yesNo(m, det.Certbot)},
		{m.T(KeyRowBusyPorts), strings.Join(append(busy, ""), " ")},
		{m.T(KeyRowFirewall), firewall},
		{m.T(KeyRowExisting), yesNo(m, det.Existing)},
	}
	for _, c := range CheckResources(det.Resources, m).All() {
		have := c.Have
		if c.Level == LevelUnknown {
			have = "?"
		}
		rows = append(rows, [2]string{c.Title, have})
	}
	for _, r := range rows {
		app.say("  %-26s %s\n", r[0], strings.TrimSpace(r[1]))
	}
}

func (app App) printPlan(ctx context.Context, plan Plan, m Messages) {
	engine.PrintPlan(ctx, app.Out, plan, m)
}

func (app App) finish(ctx context.Context, a Answers, m Messages) {
	app.say("\n%s\n", m.T(KeyInstalled, a.EffectivePublicURL()))
	link := app.setupLink(ctx, a)
	switch {
	case link != "":
		app.say("%s\n  %s\n", m.T(KeySetupLink), link)
	case a.Mode == ModeCompose:
		app.say("%s\n  docker compose --project-directory %s logs dhow | grep setup_url\n", m.T(KeySetupLinkWhere), ComposeDir)
	default:
		app.say("%s\n  journalctl -u %s | grep setup_url\n", m.T(KeySetupLinkWhere), ServiceName)
	}
	app.say("%s\n", m.T(KeyAfterInstall, AnswersPath))
}

func (app App) setupLink(ctx context.Context, a Answers) string {
	c := Cmd("journalctl", "--unit", ServiceName, "--output", "cat", "--no-pager")
	if a.Mode == ModeCompose {
		c = composeCmd("logs", "--no-color", "--no-log-prefix", "dhow")
	}
	for range 10 {
		out, err := app.Sys.Run(ctx, c)
		if err == nil {
			if link := LastSetupURL(out.Stdout); link != "" {
				return link
			}
		}
		if app.Sleep != nil {
			app.Sleep(time.Second)
		} else {
			time.Sleep(time.Second)
		}
	}
	return ""
}

func LastSetupURL(logs []byte) string {
	link := ""
	for _, line := range bytes.Split(logs, []byte("\n")) {
		i := bytes.IndexByte(line, '{')
		if i < 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line[i:], &rec); err != nil {
			continue
		}
		if v, ok := rec[setupURLKey].(string); ok && v != "" {
			link = v
		}
	}
	return link
}

func StdinFD(r io.Reader) int {
	if f, ok := r.(*os.File); ok {
		return int(f.Fd())
	}
	return -1
}

func readSecret(sys System, path string) (string, error) {
	info, err := sys.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir || info.Mode.Perm()&0o007 != 0 {
		return "", fmt.Errorf("%s %w (mode %04o); run chmod 600 %s", path, config.ErrSecretFileShared, info.Mode.Perm(), path)
	}
	data, err := sys.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("%s %w", path, config.ErrSecretFileEmpty)
	}
	return v, nil
}
