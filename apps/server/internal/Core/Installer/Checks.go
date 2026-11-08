package installer

import (
	"context"
	"time"

	engine "github.com/Khaymah/dhow/internal/installer"
)

const (
	CheckDisk  = "disk"
	CheckMPTCP = "mptcp"
	CheckClock = "clock"

	clockSkewWarn = 5 * time.Second
	clockSkewFail = 30 * time.Second
)

type ReadinessResult struct {
	Name string
	Ok   bool
}

func ReadinessFinding(m Messages, name string, healthy bool) Finding {
	title := m.T(KeyCheckNamed, name)
	if healthy {
		return ok(title)
	}
	f := Finding{Severity: SeverityFail, Title: title, Advice: m.T(KeyAdviceLogs)}
	if name == readyCheckSettings {
		f.Advice = m.T(KeyAdviceSettings)
	}
	return f
}

func DiskFinding(m Messages, title string, free uint64, err error) Finding {
	switch {
	case err != nil:
		return Finding{Severity: SeverityInfo, Title: title, Detail: err.Error()}
	case free < diskFailFree:
		return Finding{Severity: SeverityFail, Title: title, Detail: m.T(KeyDiskFree, free/(1<<20)), Advice: m.T(KeyAdviceDisk)}
	case free < diskWarnFree:
		return Finding{Severity: SeverityWarn, Title: title, Detail: m.T(KeyDiskFree, free/(1<<20)), Advice: m.T(KeyAdviceDisk)}
	default:
		return ok(title)
	}
}

func ClockFinding(m Messages, serverNow, databaseNow time.Time) Finding {
	title := m.T(KeyCheckClock)
	skew := serverNow.Sub(databaseNow).Abs()
	switch {
	case skew >= clockSkewFail:
		return Finding{Severity: SeverityFail, Title: title, Advice: m.T(KeyAdviceClock)}
	case skew >= clockSkewWarn:
		return Finding{Severity: SeverityWarn, Title: title, Advice: m.T(KeyAdviceClock)}
	default:
		return ok(title)
	}
}

type ServerCheck struct {
	Code     string
	Severity Severity
	Title    string
	Fix      string
}

type ServerInputs struct {
	Readiness    []ReadinessResult
	StateDir     string
	Sys          System
	Now          func() time.Time
	DatabaseTime func(ctx context.Context) (time.Time, error)
}

func ServerChecks(ctx context.Context, in ServerInputs, m Messages) []ServerCheck {
	out := make([]ServerCheck, 0, len(in.Readiness)+3)
	for _, r := range in.Readiness {
		out = append(out, serverCheck(r.Name, ReadinessFinding(m, r.Name, r.Ok)))
	}
	if in.Sys == nil {
		return out
	}
	if in.StateDir != "" {
		free, err := in.Sys.FreeBytes(in.StateDir)
		out = append(out, serverCheck(CheckDisk, DiskFinding(m, m.T(KeyCheckStateDisk), free, err)))
	}
	if in.DatabaseTime != nil && in.Now != nil {
		if dbNow, err := in.DatabaseTime(ctx); err == nil {
			out = append(out, serverCheck(CheckClock, ClockFinding(m, in.Now(), dbNow)))
		}
	}
	return append(out, serverCheck(CheckMPTCP, engine.KernelMPTCP(in.Sys, m)))
}

func serverCheck(code string, f Finding) ServerCheck {
	c := ServerCheck{Code: code, Severity: f.Severity, Title: f.Title}
	if f.Severity >= SeverityWarn {
		c.Fix = f.Advice
	}
	return c
}
