package system

import (
	"context"
	"time"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	installer "github.com/Khaymah/dhow/apps/server/internal/Core/Installer"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Models"
	"github.com/Khaymah/dhow/internal/buildinfo"
)

const (
	doctorPath    = "/system/doctor"
	messageDoctor = "system.doctor.loaded"
)

type ClockSource interface {
	GetDatabaseTime(ctx context.Context) (time.Time, error)
}

type DoctorOptions struct {
	Host     installer.System
	StateDir string
	Clock    ClockSource
	Now      func() time.Time
}

type DoctorService struct {
	health *HealthService
	build  buildinfo.Info
	opts   DoctorOptions
}

func NewDoctorService(health *HealthService, build buildinfo.Info, opts DoctorOptions) *DoctorService {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &DoctorService{health: health, build: build, opts: opts}
}

func buildOf(info buildinfo.Info) models.Build {
	return models.Build{Version: info.Version, Commit: info.Commit, BuiltAt: info.BuiltAt}
}

func (s *DoctorService) Report(ctx context.Context, withChecks bool) models.DoctorReport {
	health := s.health.Readiness(ctx)
	report := models.DoctorReport{Build: buildOf(s.build), Health: health}
	if withChecks {
		report.Checks = s.checks(ctx, health)
	}
	return report
}

func (s *DoctorService) checks(ctx context.Context, health models.HealthStatus) []models.DoctorCheck {
	readiness := make([]installer.ReadinessResult, 0, len(health.Checks))
	for _, c := range health.Checks {
		readiness = append(readiness, installer.ReadinessResult{Name: string(c.Name), Ok: c.Ok})
	}
	in := installer.ServerInputs{Readiness: readiness, StateDir: s.opts.StateDir, Sys: s.opts.Host, Now: s.opts.Now}
	if s.opts.Clock != nil {
		in.DatabaseTime = s.opts.Clock.GetDatabaseTime
	}
	found := installer.ServerChecks(ctx, in, installer.NewMessages(installerLang(i18n.LocaleFrom(ctx))))
	out := make([]models.DoctorCheck, 0, len(found))
	for _, c := range found {
		out = append(out, models.DoctorCheck{Code: c.Code, Status: statusOf(c.Severity), Title: c.Title, Fix: c.Fix})
	}
	return out
}

func installerLang(l i18n.Locale) string {
	if l == i18n.En {
		return installer.LangEn
	}
	return installer.LangAr
}

func statusOf(s installer.Severity) models.DoctorStatus {
	switch s {
	case installer.SeverityOK:
		return models.DoctorOK
	case installer.SeverityInfo:
		return models.DoctorInfo
	case installer.SeverityWarn:
		return models.DoctorWarn
	default:
		return models.DoctorFail
	}
}

func (c *Controller) WithDoctor(d *DoctorService) *Controller {
	c.doctor = d
	return c
}

func (c *Controller) GetDoctor(ctx context.Context, _ gen.GetDoctorRequestObject) (gen.GetDoctorResponseObject, error) {
	subject, _ := auth.SubjectFrom(ctx)
	owner := policy.CanInstance(subject, policy.InstanceSettingsManage, policy.InstanceSettings{})
	data := doctorToGen(c.doctor.Report(ctx, owner), owner)
	return gen.GetDoctor200JSONResponse{Success: true, Message: ptr(messageDoctor), Data: &data}, nil
}

func doctorToGen(r models.DoctorReport, withChecks bool) gen.DoctorReport {
	out := gen.DoctorReport{ServerBuild: buildToGen(r.Build), Health: healthToGen(r.Health)}
	if !withChecks {
		return out
	}
	checks := make([]gen.DoctorCheck, 0, len(r.Checks))
	for _, c := range r.Checks {
		item := gen.DoctorCheck{Code: c.Code, Status: gen.DoctorStatus(c.Status), Title: c.Title}
		if c.Fix != "" {
			item.Fix = ptr(c.Fix)
		}
		checks = append(checks, item)
	}
	out.Checks = &checks
	return out
}

func buildToGen(b models.Build) gen.ServerBuild {
	out := gen.ServerBuild{Version: b.Version, Commit: b.Commit}
	if !b.BuiltAt.IsZero() {
		out.BuiltAt = ptr(b.BuiltAt)
	}
	return out
}
