package installer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engine "github.com/Khaymah/dhow/internal/installer"
	"github.com/Khaymah/dhow/internal/installer/fakesys"
)

var checksNow = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func healthyInputs(sys System) ServerInputs {
	return ServerInputs{
		Readiness: []ReadinessResult{{"database", true}, {"migrations", true}, {"settings", true}},
		StateDir:  "/srv/dhow-private-state",
		Sys:       sys,
		Now:       func() time.Time { return checksNow },
		DatabaseTime: func(context.Context) (time.Time, error) {
			return checksNow, nil
		},
	}
}

func byCode(checks []ServerCheck) map[string]ServerCheck {
	out := map[string]ServerCheck{}
	for _, c := range checks {
		out[c.Code] = c
	}
	return out
}

func TestServerChecksAreAllOkOnAHealthyHost(t *testing.T) {
	checks := ServerChecks(t.Context(), healthyInputs(fakesys.New()), NewMessages(LangEn))
	require.Len(t, checks, 6)
	for _, c := range checks {
		assert.Equal(t, SeverityOK, c.Severity, c.Code)
		assert.Empty(t, c.Fix, c.Code)
	}
}

func TestServerChecksUseTheSameFixSentenceAsTheCommandLine(t *testing.T) {
	for _, lang := range []string{LangEn, LangAr} {
		m := NewMessages(lang)
		in := healthyInputs(fakesys.New())
		in.Readiness = []ReadinessResult{{"database", false}, {"migrations", false}, {"settings", false}}
		got := byCode(ServerChecks(t.Context(), in, m))
		assert.Equal(t, SeverityFail, got["database"].Severity)
		assert.Equal(t, m.T(KeyAdviceLogs), got["database"].Fix, lang)
		assert.Equal(t, m.T(KeyAdviceLogs), got["migrations"].Fix, lang)
		assert.Equal(t, m.T(KeyAdviceSettings), got["settings"].Fix, lang)
		assert.Equal(t, m.T(KeyCheckNamed, "database"), got["database"].Title, lang)
	}
	assert.NotEqual(t, NewMessages(LangEn).T(KeyAdviceLogs), NewMessages(LangAr).T(KeyAdviceLogs))
}

func TestServerChecksDiskThresholds(t *testing.T) {
	tests := []struct {
		name string
		free uint64
		want Severity
	}{
		{"plenty", 50 * gib, SeverityOK},
		{"just above the warning line", diskWarnFree, SeverityOK},
		{"below the warning line", diskWarnFree - 1, SeverityWarn},
		{"just above the failing line", diskFailFree, SeverityWarn},
		{"below the failing line", diskFailFree - 1, SeverityFail},
	}
	m := NewMessages(LangEn)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sys := fakesys.New()
			sys.Free = tt.free
			got := byCode(ServerChecks(t.Context(), healthyInputs(sys), m))[CheckDisk]
			assert.Equal(t, tt.want, got.Severity)
			if tt.want == SeverityOK {
				assert.Empty(t, got.Fix)
				return
			}
			assert.Equal(t, m.T(KeyAdviceDisk), got.Fix)
		})
	}
}

func TestServerChecksClockSkew(t *testing.T) {
	tests := []struct {
		name string
		skew time.Duration
		want Severity
	}{
		{"in sync", 0, SeverityOK},
		{"a little ahead", clockSkewWarn - time.Millisecond, SeverityOK},
		{"ahead", clockSkewWarn, SeverityWarn},
		{"behind", -clockSkewWarn, SeverityWarn},
		{"far behind", -clockSkewFail, SeverityFail},
		{"far ahead", clockSkewFail, SeverityFail},
	}
	m := NewMessages(LangEn)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := healthyInputs(fakesys.New())
			in.DatabaseTime = func(context.Context) (time.Time, error) { return checksNow.Add(tt.skew), nil }
			got := byCode(ServerChecks(t.Context(), in, m))[CheckClock]
			assert.Equal(t, tt.want, got.Severity)
			if tt.want != SeverityOK {
				assert.Equal(t, m.T(KeyAdviceClock), got.Fix)
			}
		})
	}
}

func TestServerChecksSkipTheClockWhenTheDatabaseCannotAnswer(t *testing.T) {
	in := healthyInputs(fakesys.New())
	in.DatabaseTime = func(context.Context) (time.Time, error) { return time.Time{}, errors.New("connection refused") }
	got := byCode(ServerChecks(t.Context(), in, NewMessages(LangEn)))
	assert.NotContains(t, got, CheckClock)
	assert.Contains(t, got, CheckDisk)
}

func TestServerChecksTreatAnUnreadableDiskAsInformation(t *testing.T) {
	in := healthyInputs(failingFree{fakesys.New()})
	got := byCode(ServerChecks(t.Context(), in, NewMessages(LangEn)))[CheckDisk]
	assert.Equal(t, SeverityInfo, got.Severity)
	assert.Empty(t, got.Fix)
}

type failingFree struct{ *fakesys.Sys }

func (failingFree) FreeBytes(string) (uint64, error) {
	return 0, errors.New("statfs /srv/dhow-private-state: denied")
}

func TestServerChecksReportMPTCPWithTheCommandLineAdvice(t *testing.T) {
	sys := fakesys.New()
	sys.Put(engine.MPTCPEnabledPath, "1\n", 0o644)
	m := NewMessages(LangEn)
	got := byCode(ServerChecks(t.Context(), healthyInputs(sys), m))[CheckMPTCP]
	assert.Equal(t, SeverityWarn, got.Severity)
	assert.Equal(t, m.T(engine.KeyAdviceMPTCP, m.T(engine.KeyKernelAdvisoriesURL)), got.Fix)
}

func TestServerChecksNeverCarryAPathOrADetail(t *testing.T) {
	sys := fakesys.New()
	sys.Free = 1
	sys.Put(engine.MPTCPEnabledPath, "1\n", 0o644)
	sys.Put(engine.KernelReleasePath, "6.8.0-secret-kernel\n", 0o444)
	in := healthyInputs(failingFree{sys})
	in.Readiness = []ReadinessResult{{"database", false}, {"keyring", false}}
	for _, lang := range []string{LangEn, LangAr} {
		for _, c := range ServerChecks(t.Context(), in, NewMessages(lang)) {
			for _, text := range []string{c.Title, c.Fix} {
				assert.NotContains(t, text, "/srv/dhow-private-state", c.Code)
				assert.NotContains(t, text, "statfs", c.Code)
				assert.NotContains(t, text, "secret-kernel", c.Code)
				assert.NotContains(t, text, engine.MPTCPEnabledPath, c.Code)
			}
		}
	}
}

func TestServerChecksWithoutAHostStopAtReadiness(t *testing.T) {
	in := healthyInputs(nil)
	checks := ServerChecks(t.Context(), in, NewMessages(LangEn))
	assert.Len(t, checks, 3)
}
