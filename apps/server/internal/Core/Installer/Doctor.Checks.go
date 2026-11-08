package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/netip"
	"strings"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

const (
	gib         = 1 << 30
	minPublicV4 = 16
	minPublicV6 = 32
)

type permCheck struct {
	Path string
	Perm fs.FileMode
}

func (d Doctor) installed() Finding {
	var missing []string
	for _, p := range []string{BinaryPath, UnitPath, EnvFilePath} {
		if !exists(d.Sys, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return Finding{Severity: SeverityFail, Title: d.M.T(KeyCheckInstalled), Detail: strings.Join(missing, ", "), Advice: d.M.T(KeyAdviceReinstall)}
	}
	return ok(d.M.T(KeyCheckInstalled))
}

func (d Doctor) unitDrift() Finding {
	current, err := d.Sys.ReadFile(UnitPath)
	if err == nil && bytes.Equal(current, UnitFile) {
		return ok(d.M.T(KeyCheckUnit))
	}
	fix := d.b.unitStep()
	restart := Cmd("systemctl", "restart", ServiceName)
	apply := fix.Apply
	fix.Apply = func(ctx context.Context) error {
		if err := apply(ctx); err != nil {
			return err
		}
		_, err := run(ctx, d.Sys, restart)
		return err
	}
	fix.Commands = append(fix.Commands, restart.String())
	fix.Done = nil
	return Finding{Severity: SeverityWarn, Title: d.M.T(KeyCheckUnit), Detail: d.M.T(KeyUnitDrift, UnitPath), Fix: &fix}
}

func (d Doctor) permissions() Finding {
	return d.permissionsOf([]permCheck{
		{BinaryPath, modeRootExec.Perm},
		{ConfigDir, modeRootDir.Perm},
		{CredentialsDir, modeRootDir.Perm},
		{SecretKeyPath, modeRootPrivate.Perm},
		{DatabaseURLPath, modeRootPrivate.Perm},
		{KeyringPath, modeRootPrivate.Perm},
		{EnvFilePath, modeRootPrivate.Perm},
		{AnswersPath, modeRootPrivate.Perm},
		{UnitPath, modeRootPublic.Perm},
	})
}

func (d Doctor) permissionsOf(checks []permCheck) Finding {
	var wrong []permCheck
	var details []string
	for _, c := range checks {
		info, err := d.Sys.Stat(c.Path)
		if err != nil {
			continue
		}
		if info.Mode.Perm() != c.Perm || info.UID != 0 || info.GID != 0 {
			wrong = append(wrong, c)
			details = append(details, fmt.Sprintf("%s %04o uid=%d gid=%d (%s %04o root:root)", c.Path, info.Mode.Perm(), info.UID, info.GID, d.M.T(KeyWant), c.Perm))
		}
	}
	if len(wrong) == 0 {
		return ok(d.M.T(KeyCheckPermissions))
	}
	fix := Step{Title: d.M.T(KeyFixPermissions), Apply: func(context.Context) error {
		for _, c := range wrong {
			if err := d.Sys.Chmod(c.Path, FileMode{Perm: c.Perm}); err != nil {
				return err
			}
		}
		return nil
	}}
	for _, c := range wrong {
		fix.Commands = append(fix.Commands, fmt.Sprintf("chown root:root %s && chmod %04o %s", c.Path, c.Perm, c.Path))
	}
	return Finding{Severity: SeverityFail, Title: d.M.T(KeyCheckPermissions), Detail: strings.Join(details, "; "), Fix: &fix}
}

func (d Doctor) secrets() Finding {
	var problems []string
	key, err := d.secretContent(SecretKeyPath)
	if err == nil {
		err = config.ValidateSecretKey(key)
	}
	if err != nil {
		problems = append(problems, config.CredentialSecretKey+": "+err.Error())
	}
	url, err := d.secretContent(DatabaseURLPath)
	if err == nil {
		err = config.ValidateDatabaseURL(url)
	}
	if err != nil {
		problems = append(problems, config.CredentialDatabaseURL+": "+err.Error())
	}
	if len(problems) > 0 {
		return Finding{Severity: SeverityFail, Title: d.M.T(KeyCheckSecrets), Detail: strings.Join(problems, "; "), Advice: d.M.T(KeyAdviceSecrets)}
	}
	return ok(d.M.T(KeyCheckSecrets))
}

func (d Doctor) secretContent(path string) (string, error) {
	data, err := d.Sys.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("%s %w", path, config.ErrSecretFileEmpty)
	}
	return v, nil
}

func (d Doctor) service(ctx context.Context) Finding {
	active := succeeds(ctx, d.Sys, Cmd("systemctl", "is-active", "--quiet", ServiceName))
	enabled := succeeds(ctx, d.Sys, Cmd("systemctl", "is-enabled", "--quiet", ServiceName))
	if active && enabled {
		return ok(d.M.T(KeyCheckService))
	}
	fix := CommandStep(d.Sys, d.M.T(KeyFixService), nil, []Command{
		Cmd("systemctl", "enable", ServiceName), Cmd("systemctl", "restart", ServiceName),
	}, nil)
	detail := d.M.T(KeyServiceState, yesNo(d.M, active), yesNo(d.M, enabled))
	return Finding{Severity: SeverityFail, Title: d.M.T(KeyCheckService), Detail: detail, Fix: &fix, Advice: "journalctl -u " + ServiceName + " -n 50"}
}

type envelope struct {
	Data json.RawMessage
}

const readyCheckSettings = "settings"

type readyData struct {
	Status string
	Checks []struct {
		Name string
		Ok   bool
	}
}

type metaData struct {
	SetupRequired bool
}

func (d Doctor) local(path string) string { return "http://" + upstream(d.A) + path }

func (d Doctor) health(ctx context.Context) []Finding {
	res, err := d.Sys.HTTP(ctx, Probe{URL: d.local("/api/v1/health/ready")})
	if err != nil {
		return []Finding{{Severity: SeverityFail, Title: d.M.T(KeyCheckHealth), Detail: err.Error(), Advice: d.M.T(KeyAdviceLogs)}}
	}
	var env envelope
	var data readyData
	if json.Unmarshal(res.Body, &env) != nil || json.Unmarshal(env.Data, &data) != nil {
		return []Finding{{Severity: SeverityFail, Title: d.M.T(KeyCheckHealth), Detail: fmt.Sprintf("status %d", res.Status)}}
	}
	var out []Finding
	for _, c := range data.Checks {
		f := ReadinessFinding(d.M, c.Name, c.Ok)
		if c.Ok {
			out = append(out, f)
			continue
		}
		if c.Name == "database" && d.A.Mode == ModeNative && d.A.Database.Mode == DatabaseInstall {
			svc := d.Det.Distro.PostgresService()
			fix := CommandStep(d.Sys, d.M.T(KeyFixPostgres, svc), nil, []Command{
				Cmd("systemctl", "start", svc), Cmd("systemctl", "restart", ServiceName),
			}, nil)
			f.Fix = &fix
		}
		out = append(out, f)
	}
	return append(out, d.setupState(ctx))
}

func (d Doctor) setupState(ctx context.Context) Finding {
	res, err := d.Sys.HTTP(ctx, Probe{URL: d.local("/api/v1/meta")})
	var env envelope
	var meta metaData
	if err != nil || json.Unmarshal(res.Body, &env) != nil || json.Unmarshal(env.Data, &meta) != nil {
		return Finding{Severity: SeverityInfo, Title: d.M.T(KeyCheckSetup), Detail: d.M.T(KeyUnknown)}
	}
	if meta.SetupRequired {
		where := "journalctl -u " + ServiceName + " | grep setup_url"
		if d.A.Mode == ModeCompose {
			where = "docker compose --project-directory " + ComposeDir + " logs dhow | grep setup_url"
		}
		return Finding{Severity: SeverityInfo, Title: d.M.T(KeyCheckSetup), Detail: d.M.T(KeySetupPending, where)}
	}
	return ok(d.M.T(KeyCheckSetup))
}

func (d Doctor) configuredTrusted() string {
	if d.A.Mode == ModeCompose {
		return EnvValue(ParseEnv(d.b.readExisting(ComposeEnvPath)), config.EnvTrustedProxies)
	}
	return EnvValue(ParseEnv(d.b.readExisting(EnvFilePath)), config.EnvTrustedProxies)
}

func (d Doctor) trustedProxies() Finding {
	title := d.M.T(KeyCheckTrusted)
	raw := d.configuredTrusted()
	var prefixes []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := netip.ParsePrefix(part)
		if err != nil {
			a, aerr := netip.ParseAddr(part)
			if aerr != nil {
				return Finding{Severity: SeverityFail, Title: title, Detail: d.M.T(KeyTrustedInvalid, part)}
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		prefixes = append(prefixes, p)
	}
	for _, p := range prefixes {
		if p.Bits() == 0 {
			return Finding{Severity: SeverityFail, Title: title, Detail: d.M.T(KeyTrustedEveryone, p.String())}
		}
		if broadPublic(p) {
			return Finding{Severity: SeverityWarn, Title: title, Detail: d.M.T(KeyTrustedBroad, p.String())}
		}
	}
	return d.trustedForProxy(title, raw, prefixes)
}

func broadPublic(p netip.Prefix) bool {
	a := p.Addr()
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	if a.Is4() {
		return p.Bits() < minPublicV4
	}
	return p.Bits() < minPublicV6
}

func (d Doctor) trustedForProxy(title, raw string, prefixes []netip.Prefix) Finding {
	if d.A.ManagedProxy() && d.A.Mode == ModeNative {
		loop := netip.MustParseAddr("127.0.0.1")
		for _, p := range prefixes {
			if p.Contains(loop) {
				return ok(title)
			}
		}
		fix := FileStep(d.Sys, d.M.T(KeyFixTrusted), EnvFilePath, nativeEnv(d.A, ParseEnv(d.b.readExisting(EnvFilePath))), modeRootPrivate)
		fix.Done = nil
		apply := fix.Apply
		fix.Apply = func(ctx context.Context) error {
			if err := apply(ctx); err != nil {
				return err
			}
			_, err := run(ctx, d.Sys, Cmd("systemctl", "restart", ServiceName))
			return err
		}
		return Finding{Severity: SeverityFail, Title: title, Detail: d.M.T(KeyTrustedMissingLocal, raw), Fix: &fix}
	}
	if d.A.Proxy.Kind == ProxyExisting && len(prefixes) == 0 {
		return Finding{Severity: SeverityWarn, Title: title, Detail: d.M.T(KeyNoteNoTrustedProxies)}
	}
	return ok(title)
}

func (d Doctor) disk() []Finding {
	paths := []string{"/var/lib"}
	if d.A.Mode == ModeNative && d.A.Database.Mode == DatabaseInstall && exists(d.Sys, PostgresDataDir(d.Det.Distro)) {
		paths = append(paths, PostgresDataDir(d.Det.Distro))
	}
	var out []Finding
	for _, p := range paths {
		free, err := d.Sys.FreeBytes(p)
		out = append(out, DiskFinding(d.M, d.M.T(KeyCheckDisk, p), free, err))
	}
	return out
}
