package environments

import (
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/hostname"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	fieldKind          = "Kind"
	fieldUsername      = "Username"
	fieldKeyName       = "KeySecretName"
	fieldDeployPath    = "DeployPath"
	fieldKeepReleases  = "KeepReleases"
	fieldSettings      = "Settings"
	fieldAllowedImages = "AllowedImages"
	fieldHosts         = "Hosts"
	fieldHost          = "Host"
	fieldPort          = "Port"
	fieldHostKey       = "HostKey"
	fieldHostIDs       = "HostIds"
)

var (
	usernamePattern   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	deployPathPattern = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,200}$`)
	secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	sharedPattern     = regexp.MustCompile(`^[A-Za-z0-9.][A-Za-z0-9._-]{0,63}$`)
	projectPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	composePattern    = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,100}$`)

	systemPaths = []string{
		"/bin", "/boot", "/dev", "/etc", "/lib", "/lib32", "/lib64", "/proc", "/root", "/run", "/sbin", "/sys", "/usr",
		"/var/lib/docker", "/var/run",
	}
)

func ValidDeployPath(p string) bool {
	if !deployPathPattern.MatchString(p) || strings.HasSuffix(p, "/") || strings.Contains(p, "//") {
		return false
	}
	parts := strings.Split(p[1:], "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "." || part == ".." || strings.HasPrefix(part, "-") {
			return false
		}
	}
	for _, sys := range systemPaths {
		if p == sys || strings.HasPrefix(p, sys+"/") {
			return false
		}
	}
	return true
}

func validUsername(u string) bool { return usernamePattern.MatchString(u) }

func validSecretName(n string) bool {
	return secretNamePattern.MatchString(n) && !strings.HasPrefix(strings.ToUpper(n), "DHOW_")
}

func oneLine(v string, max int) bool {
	if !utf8.ValidString(v) || len(v) > max {
		return false
	}
	return !strings.ContainsFunc(v, func(r rune) bool { return unicode.IsControl(r) || invisible(r) })
}

func settingsPath(parts ...string) string { return fieldSettings + "." + strings.Join(parts, ".") }

func validateRolling(r *models.Rolling, hosts int) []apperrors.IErrorDetail {
	if r == nil {
		return nil
	}
	var details []apperrors.IErrorDetail
	if r.BatchSize < 1 || r.BatchSize > max(hosts, 1) {
		details = append(details, invalid(settingsPath("Rolling", "BatchSize")))
	}
	if r.BatchPauseSeconds < 0 || r.BatchPauseSeconds > models.MaxBatchPauseSeconds {
		details = append(details, invalid(settingsPath("Rolling", "BatchPauseSeconds")))
	}
	return details
}

func validateSSHSettings(s *models.SSHSettings) []apperrors.IErrorDetail {
	var details []apperrors.IErrorDetail
	for _, hook := range []struct{ name, value string }{
		{"ReloadCommand", s.ReloadCommand}, {"PreSwitchCommand", s.PreSwitchCommand}, {"PostSwitchCommand", s.PostSwitchCommand},
	} {
		if !oneLine(hook.value, models.MaxHookLength) {
			details = append(details, invalid(settingsPath("Ssh", hook.name)))
		}
	}
	if len(s.Shared) > models.MaxSharedNames {
		details = append(details, invalid(settingsPath("Ssh", "Shared")))
	}
	for i, n := range s.Shared {
		if !sharedPattern.MatchString(n) || n == "." || strings.Contains(n, "..") {
			details = append(details, invalid(settingsPath("Ssh", "Shared["+strconv.Itoa(i)+"]")))
		}
	}
	return details
}

func validateComposeSettings(c *models.ComposeSettings) []apperrors.IErrorDetail {
	var details []apperrors.IErrorDetail
	if !projectPattern.MatchString(c.ProjectName) {
		details = append(details, invalid(settingsPath("Compose", "ProjectName")))
	}
	if !composePattern.MatchString(c.ComposeFile) || strings.HasPrefix(c.ComposeFile, "/") || strings.HasPrefix(c.ComposeFile, "-") || strings.Contains(c.ComposeFile, "..") {
		details = append(details, invalid(settingsPath("Compose", "ComposeFile")))
	}
	return details
}

func validateSettings(kind string, st models.TargetSettings, hosts int) []apperrors.IErrorDetail {
	details := validateRolling(st.Rolling, hosts)
	if kind == models.TargetKindSSH && st.Compose != nil {
		details = append(details, invalid(settingsPath("Compose")))
	}
	if kind == models.TargetKindCompose && st.SSH != nil {
		details = append(details, invalid(settingsPath("Ssh")))
	}
	if st.SSH != nil && kind == models.TargetKindSSH {
		details = append(details, validateSSHSettings(st.SSH)...)
	}
	if st.Compose != nil && kind == models.TargetKindCompose {
		details = append(details, validateComposeSettings(st.Compose)...)
	}
	return details
}

func normalizeSettings(kind, name string, st models.TargetSettings) models.TargetSettings {
	if st.Rolling == nil {
		st.Rolling = &models.Rolling{BatchSize: models.DefaultBatchSize}
	}
	switch kind {
	case models.TargetKindSSH:
		if st.SSH == nil {
			st.SSH = &models.SSHSettings{}
		}
		if st.SSH.Shared == nil {
			st.SSH.Shared = []string{}
		}
		st.SSH.Shared = uniqueInOrder(st.SSH.Shared)
	case models.TargetKindCompose:
		if st.Compose == nil {
			st.Compose = &models.ComposeSettings{}
		}
		if st.Compose.ProjectName == "" {
			st.Compose.ProjectName = name
		}
		if st.Compose.ComposeFile == "" {
			st.Compose.ComposeFile = models.DefaultComposeFile
		}
	}
	return st
}

func validateImages(images []string) []apperrors.IErrorDetail {
	if len(images) > models.MaxAllowedImages {
		return []apperrors.IErrorDetail{invalid(fieldAllowedImages)}
	}
	var details []apperrors.IErrorDetail
	for i, img := range images {
		if !ValidPluginPrefix(img) {
			details = append(details, invalid(fieldAllowedImages+"["+strconv.Itoa(i)+"]"))
		}
	}
	return details
}

type targetFields struct {
	kind          string
	username      string
	keySecretName string
	deployPath    string
	keepReleases  int
	settings      models.TargetSettings
	images        []string
	hosts         int
}

func (f targetFields) validate() *apperrors.AppError {
	var details []apperrors.IErrorDetail
	if f.kind != models.TargetKindSSH && f.kind != models.TargetKindCompose {
		details = append(details, invalid(fieldKind))
	}
	if !validUsername(f.username) || (f.username == models.RootUsername && !f.settings.AllowRoot) {
		details = append(details, invalid(fieldUsername))
	}
	if f.keySecretName != "" && !validSecretName(f.keySecretName) {
		details = append(details, invalid(fieldKeyName))
	}
	if f.keepReleases < models.MinKeepReleases || f.keepReleases > models.MaxKeepReleases {
		details = append(details, invalid(fieldKeepReleases))
	}
	details = append(details, validateSettings(f.kind, f.settings, f.hosts)...)
	details = append(details, validateImages(f.images)...)
	if len(details) > 0 {
		return apperrors.ValidationFailed(details...)
	}
	if !ValidDeployPath(f.deployPath) {
		return apperrors.DeployPathInvalid(invalid(fieldDeployPath))
	}
	return nil
}

func blockedAtSave(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.IsUnspecified() || addr.IsMulticast() {
		return true
	}
	candidates := append([]netip.Addr{addr}, netguard.EmbeddedIPv4(addr)...)
	for _, c := range candidates {
		if slices.Contains(netguard.MetadataAddrs(), c) {
			return true
		}
		for _, p := range netguard.NeverAllowed() {
			if p.Contains(c) {
				return true
			}
		}
	}
	return false
}

func normalizeHost(raw string, field string) (string, *apperrors.AppError) {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if addr.Zone() != "" {
			return "", apperrors.ValidationFailed(invalid(field))
		}
		if blockedAtSave(addr) {
			return "", apperrors.TargetHostBlocked(invalid(field))
		}
		return addr.Unmap().String(), nil
	}
	if hostname.Check(host) != nil {
		return "", apperrors.ValidationFailed(invalid(field))
	}
	return host, nil
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

func hostField(i int, sub string) string { return fieldHosts + "[" + strconv.Itoa(i) + "]." + sub }
