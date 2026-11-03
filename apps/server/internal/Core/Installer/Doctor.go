package installer

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	engine "github.com/Khaymah/dhow/internal/installer"
)

type (
	Severity = engine.Severity
	Finding  = engine.Finding
)

const (
	SeverityOK   = engine.SeverityOK
	SeverityInfo = engine.SeverityInfo
	SeverityWarn = engine.SeverityWarn
	SeverityFail = engine.SeverityFail
)

var ErrUnhealthy = engine.ErrUnhealthy

type DoctorOptions struct {
	Fix  bool
	Yes  bool
	Lang string
}

type Doctor struct {
	Sys      System
	Det      Detection
	A        Answers
	M        Messages
	Probe    ProbeIDs
	Settings SettingsReader
	b        *Builder
}

type ProbeIDs func() (string, error)

func (app App) loadInstalledAnswers() (Answers, bool) {
	data, err := app.Sys.ReadFile(AnswersPath)
	if err != nil {
		return Answers{}, false
	}
	a, err := ParseInstalledAnswers(bytes.NewReader(data), exists(app.Sys, DatabaseURLPath))
	if err != nil {
		return Answers{}, false
	}
	return a, true
}

func (app App) Doctor(ctx context.Context, opts DoctorOptions) error {
	a, found := app.loadInstalledAnswers()
	m := NewMessages(ChooseLang(opts.Lang, a.Lang, app.Locale))
	if !app.Sys.IsRoot() {
		return errors.New(m.T(KeyNeedRoot))
	}
	if !found {
		app.say("%s\n", m.T(KeyDoctorNotInstalled, AnswersPath))
		return ErrUnhealthy
	}
	det := Detect(ctx, app.Sys)
	d := Doctor{Sys: app.Sys, Det: det, A: a, M: m, Probe: func() (string, error) { return RandomHex(8) }}
	if reader, ok := app.DB.(SettingsReader); ok {
		d.Settings = reader
	}
	d.b = &Builder{Sys: app.Sys, Det: det, A: a, M: m, Version: app.Version, Sleep: app.Sleep}
	findings := d.Run(ctx)
	engine.PrintFindings(app.Out, findings, m)
	interactive := IsTerminal(app.InFD)
	remaining, err := engine.ApplyFixes(ctx, findings, engine.FixOptions{
		Fix: opts.Fix, Interactive: interactive, Prompter: NewPrompter(app.In, app.Out, app.InFD, m).Prompter, Out: app.Out, M: m,
	})
	if err != nil {
		return err
	}
	if remaining > 0 {
		if !opts.Fix && !interactive {
			app.say("\n%s\n", m.T(KeyDoctorFixHint))
		}
		return fmt.Errorf("%w: %d", ErrUnhealthy, remaining)
	}
	app.say("\n%s\n", m.T(cleanKey(findings)))
	return nil
}

func cleanKey(findings []Finding) Key {
	if engine.HasAdvisory(findings) {
		return engine.KeyDoctorAdvisories
	}
	return KeyDoctorClean
}

func (d Doctor) Run(ctx context.Context) []Finding {
	var out []Finding
	if d.A.Mode == ModeCompose {
		out = append(out, d.composeChecks(ctx)...)
	} else {
		out = append(out, d.nativeChecks(ctx)...)
	}
	out = append(out, d.trustedProxies())
	out = append(out, d.publicChecks(ctx)...)
	out = append(out, d.resources()...)
	out = append(out, d.disk()...)
	out = append(out, d.settingSources(ctx)...)
	return append(out, engine.KernelMPTCP(d.Sys, d.M))
}

func ok(title string) Finding { return engine.OK(title) }

func (d Doctor) nativeChecks(ctx context.Context) []Finding {
	out := []Finding{d.installed()}
	if out[0].Severity == SeverityFail {
		return out
	}
	out = append(out, d.unitDrift(), d.permissions(), d.secrets())
	out = append(out, d.keyring(ctx)...)
	out = append(out, d.service(ctx))
	return append(out, d.health(ctx)...)
}

func (d Doctor) composeChecks(ctx context.Context) []Finding {
	var out []Finding
	if !exists(d.Sys, ComposeFilePath) || !exists(d.Sys, ComposeEnvPath) {
		return []Finding{{Severity: SeverityFail, Title: d.M.T(KeyCheckInstalled), Detail: ComposeFilePath, Advice: d.M.T(KeyAdviceReinstall)}}
	}
	out = append(out, ok(d.M.T(KeyCheckInstalled)), d.permissionsOf([]permCheck{
		{ConfigDir, modeRootDir.Perm}, {ComposeDir, modeRootDir.Perm}, {ComposeEnvPath, modeRootPrivate.Perm},
	}))
	out = append(out, d.keyring(ctx)...)
	res, err := d.Sys.Run(ctx, composeCmd("ps", "--status", "running", "--services"))
	running := err == nil && bytes.Contains(res.Stdout, []byte("dhow")) && bytes.Contains(res.Stdout, []byte("postgres"))
	if running {
		out = append(out, ok(d.M.T(KeyCheckService)))
	} else {
		fix := CommandStep(d.Sys, d.M.T(KeyFixComposeUp), nil, []Command{composeUp()}, nil)
		out = append(out, Finding{Severity: SeverityFail, Title: d.M.T(KeyCheckService), Fix: &fix})
	}
	return append(out, d.health(ctx)...)
}
