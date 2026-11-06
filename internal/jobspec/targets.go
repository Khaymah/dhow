package jobspec

import (
	"encoding/base64"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Khaymah/dhow/internal/hostname"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	FeatureDeployTargets = "deploy_targets"
	TargetInput          = "target"
	TargetArtifactInput  = "artifact"
	TargetModeInput      = "mode"
	TargetModeDeploy     = "deploy"
	TargetModeCheck      = "check"
	TargetKindSSH        = "ssh"
	TargetKindCompose    = "compose"
	DeploySSHImage       = "ghcr.io/khaymah/dhow-deploy-ssh"
	DeployComposeImage   = "ghcr.io/khaymah/dhow-deploy-compose"
	TargetKeyPrefix      = "dhow.target."
	targetKeySuffix      = ".key"
	TargetKeyFileBase    = "target-key"

	MaxTargets         = 4
	MaxTargetHosts     = 10
	MaxExactEndpoints  = 32
	MaxTargetShared    = 20
	MaxTargetHook      = 1024
	MinKeepReleases    = 2
	MaxKeepReleases    = 50
	MaxBatchPause      = 600
	MaxKnownHostsBytes = 1024
	defaultSSHPort     = 22

	DeployRollbackBudget = 3 * time.Minute
	DeployCleanupBudget  = 30 * time.Second
	DeployUnlockBudget   = 10 * time.Second
	DeployConnGrace      = 15 * time.Second
)

var (
	targetNamePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	targetUserPattern     = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	deployPathPattern     = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,200}$`)
	releaseIDPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	sharedNamePattern     = regexp.MustCompile(`^[A-Za-z0-9.][A-Za-z0-9._-]{0,63}$`)
	composeProjectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	composeFilePattern    = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/-]{0,99}$`)
	systemPaths           = []string{
		"/bin", "/boot", "/dev", "/etc", "/lib", "/lib32", "/lib64", "/proc", "/root", "/run", "/sbin", "/sys", "/usr",
		"/var/lib/docker", "/var/run",
	}
	knownHostTypes = []string{"ssh-ed25519", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ssh-rsa"}
)

func TargetKeySecret(index int) string {
	return TargetKeyPrefix + strconv.Itoa(index) + targetKeySuffix
}

func ParseTargetKeySecret(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, TargetKeyPrefix)
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, targetKeySuffix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 || n >= MaxTargets || strconv.Itoa(n) != digits {
		return 0, false
	}
	return n, true
}

func TargetKeyFile(index int) string {
	if index == 0 {
		return TargetKeyFileBase
	}
	return TargetKeyFileBase + "-" + strconv.Itoa(index)
}

func OfficialDeployImages() []string { return []string{DeploySSHImage, DeployComposeImage} }

func IsOfficialDeployImage(image string) bool {
	repo, _, _ := strings.Cut(QualifyImage(image), "@")
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	return slices.Contains(OfficialDeployImages(), repo)
}

var pinnedImagePattern = regexp.MustCompile(`^[a-z0-9.-]+(:[0-9]+)?/[a-z0-9._/-]+@sha256:[0-9a-f]{64}$`)

func ValidPinnedDeployImage(image string) bool {
	return pinnedImagePattern.MatchString(image) && IsOfficialDeployImage(image)
}

func DeployCancelGrace() time.Duration {
	return DeployRollbackBudget + DeployConnGrace + DeployCleanupBudget + DeployUnlockBudget
}

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

func KnownHostsPattern(host string, port int) string {
	if port == defaultSSHPort {
		return host
	}
	return "[" + host + "]:" + strconv.Itoa(port)
}

func KnownHostsLine(host string, port int, authorizedKey string) string {
	return KnownHostsPattern(host, port) + " " + authorizedKey
}

func validKnownHostsLine(host string, port int, line string) bool {
	if len(line) > MaxKnownHostsBytes {
		return false
	}
	fields := strings.Split(line, " ")
	if len(fields) != 3 || fields[0] != KnownHostsPattern(host, port) || !slices.Contains(knownHostTypes, fields[1]) {
		return false
	}
	blob, err := base64.StdEncoding.DecodeString(fields[2])
	return err == nil && len(blob) >= 4+len(fields[1]) && string(blob[4:4+len(fields[1])]) == fields[1]
}

func validTargetHost(host string) bool {
	if host == "" || strings.HasPrefix(host, "-") {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Zone() == ""
	}
	return hostname.Check(host) == nil
}

func oneLine(v string, limit int) bool {
	if !utf8.ValidString(v) || len(v) > limit {
		return false
	}
	return !strings.ContainsFunc(v, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
}

func Endpoint(host string, port int) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}

func splitEndpoint(entry string) (string, string, bool) {
	if rest, ok := strings.CutPrefix(entry, "["); ok {
		host, port, found := strings.Cut(rest, "]:")
		return host, port, found
	}
	i := strings.LastIndexByte(entry, ':')
	if i < 0 || strings.Contains(entry[:i], ":") {
		return "", "", false
	}
	return entry[:i], entry[i+1:], true
}

var canonicalID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validID(id string) bool { return canonicalID.MatchString(id) }

func ParseEndpoint(entry string) (string, int, bool) {
	host, portText, split := splitEndpoint(entry)
	if !split || len(entry) > MaxEgressEntryLength {
		return "", 0, false
	}
	port, ok := parsePort(portText)
	if !ok || !validTargetHost(host) || Endpoint(host, port) != entry {
		return "", 0, false
	}
	return host, port, true
}

func (c *checker) targets(spec *runnerv1.JobSpec) {
	list := spec.GetTargets()
	if len(list) > MaxTargets {
		c.add("Targets", ReasonTooMany)
		return
	}
	steps := map[int32]bool{}
	for i, t := range list {
		field := at("Targets", i)
		c.targetStep(field, t, spec, steps)
		c.targetIdentity(field, t)
		c.targetHosts(field, t)
		c.targetSettings(field, t)
	}
	c.targetSecrets(spec)
	c.exactEndpoints(spec)
}

func (c *checker) targetStep(field string, t *runnerv1.StepTarget, spec *runnerv1.JobSpec, seen map[int32]bool) {
	idx := t.GetStepIndex()
	if idx < 0 || int(idx) >= len(spec.GetSteps()) {
		c.add(field+".StepIndex", ReasonOutOfRange)
		return
	}
	step := spec.GetSteps()[idx]
	switch {
	case seen[idx]:
		c.add(field+".StepIndex", ReasonDuplicate)
	case step.GetPlugin() == nil || !step.GetMutating() || step.GetPlugin().GetInputs()[TargetInput] != t.GetName():
		c.add(field+".StepIndex", ReasonInvalid)
	}
	seen[idx] = true
}

func (c *checker) targetIdentity(field string, t *runnerv1.StepTarget) {
	if !validID(t.GetTargetId()) {
		c.add(field+".TargetId", ReasonInvalid)
	}
	if t.GetVersion() < 1 {
		c.add(field+".Version", ReasonOutOfRange)
	}
	if !targetNamePattern.MatchString(t.GetName()) {
		c.add(field+".Name", ReasonInvalid)
	}
	if t.GetKind() != TargetKindSSH && t.GetKind() != TargetKindCompose {
		c.add(field+".Kind", ReasonInvalid)
	}
	if !targetUserPattern.MatchString(t.GetUser()) {
		c.add(field+".User", ReasonInvalid)
	}
	if !ValidDeployPath(t.GetDeployPath()) {
		c.add(field+".DeployPath", ReasonInvalid)
	}
	if !releaseIDPattern.MatchString(t.GetReleaseId()) {
		c.add(field+".ReleaseId", ReasonInvalid)
	}
	if k := t.GetKeepReleases(); k < MinKeepReleases || k > MaxKeepReleases {
		c.add(field+".KeepReleases", ReasonOutOfRange)
	}
}

func (c *checker) targetHosts(field string, t *runnerv1.StepTarget) {
	hosts := t.GetHosts()
	if len(hosts) == 0 {
		c.add(field+".Hosts", ReasonRequired)
		return
	}
	if len(hosts) > MaxTargetHosts {
		c.add(field+".Hosts", ReasonTooMany)
		return
	}
	ids := map[string]bool{}
	addresses := map[string]bool{}
	for i, h := range hosts {
		hf := at(field+".Hosts", i)
		if !validID(h.GetHostId()) || ids[h.GetHostId()] {
			c.add(hf+".HostId", ReasonInvalid)
		}
		ids[h.GetHostId()] = true
		if !validTargetHost(h.GetHost()) {
			c.add(hf+".Host", ReasonInvalid)
		}
		if h.GetPort() < 1 || h.GetPort() > maxPort {
			c.add(hf+".Port", ReasonOutOfRange)
		}
		if !validKnownHostsLine(h.GetHost(), int(h.GetPort()), h.GetKnownHostsLine()) {
			c.add(hf+".KnownHostsLine", ReasonInvalid)
		}
		address := Endpoint(h.GetHost(), int(h.GetPort()))
		if addresses[address] {
			c.add(hf+".Host", ReasonDuplicate)
		}
		addresses[address] = true
	}
}

func (c *checker) targetSettings(field string, t *runnerv1.StepTarget) {
	s := t.GetSettings()
	if s == nil {
		c.add(field+".Settings", ReasonRequired)
		return
	}
	f := field + ".Settings"
	if s.GetBatchSize() < 1 || int(s.GetBatchSize()) > max(len(t.GetHosts()), 1) {
		c.add(f+".BatchSize", ReasonOutOfRange)
	}
	if s.GetBatchPauseSeconds() < 0 || s.GetBatchPauseSeconds() > MaxBatchPause {
		c.add(f+".BatchPauseSeconds", ReasonOutOfRange)
	}
	for _, h := range []struct{ name, value string }{
		{".ReloadCommand", s.GetReloadCommand()}, {".PreSwitchCommand", s.GetPreSwitchCommand()}, {".PostSwitchCommand", s.GetPostSwitchCommand()},
	} {
		if !oneLine(h.value, MaxTargetHook) {
			c.add(f+h.name, ReasonInvalid)
		}
	}
	c.targetShared(f, s)
	c.targetCompose(f, t.GetKind(), s)
}

func (c *checker) targetShared(field string, s *runnerv1.TargetSettings) {
	if len(s.GetShared()) > MaxTargetShared {
		c.add(field+".Shared", ReasonTooMany)
		return
	}
	for i, n := range s.GetShared() {
		if !sharedNamePattern.MatchString(n) || n == "." || strings.Contains(n, "..") {
			c.add(at(field+".Shared", i), ReasonInvalid)
		}
	}
}

func (c *checker) targetCompose(field, kind string, s *runnerv1.TargetSettings) {
	compose := s.GetComposeProject() != "" || s.GetComposeFile() != "" || s.GetAllowUnsafeCompose()
	switch kind {
	case TargetKindSSH:
		if compose {
			c.add(field+".ComposeProject", ReasonInvalid)
		}
	case TargetKindCompose:
		if len(s.GetShared()) > 0 || s.GetReloadCommand() != "" || s.GetPreSwitchCommand() != "" || s.GetPostSwitchCommand() != "" {
			c.add(field+".Shared", ReasonInvalid)
		}
		if !composeProjectPattern.MatchString(s.GetComposeProject()) {
			c.add(field+".ComposeProject", ReasonInvalid)
		}
		file := s.GetComposeFile()
		if !composeFilePattern.MatchString(file) || strings.HasPrefix(file, "/") || strings.Contains(file, "..") {
			c.add(field+".ComposeFile", ReasonInvalid)
		}
	}
}

func (c *checker) targetSecrets(spec *runnerv1.JobSpec) {
	seen := map[int]bool{}
	for i, s := range spec.GetSecrets() {
		n, reserved := ParseTargetKeySecret(s.GetName())
		if !reserved {
			if strings.HasPrefix(s.GetName(), TargetKeyPrefix) {
				c.add(at("Secrets", i)+".Name", ReasonReserved)
			}
			continue
		}
		field := at("Secrets", i)
		if n >= len(spec.GetTargets()) || seen[n] {
			c.add(field+".Name", ReasonReserved)
			continue
		}
		seen[n] = true
		idx := s.GetStepIndexes()
		if s.GetAsFile() != TargetKeyFile(n) || len(idx) != 1 || idx[0] != spec.GetTargets()[n].GetStepIndex() {
			c.add(field, ReasonReserved)
		}
	}
	for n := range spec.GetTargets() {
		if !seen[n] {
			c.add(at("Targets", n)+".Key", ReasonRequired)
		}
	}
}

func (c *checker) exactEndpoints(spec *runnerv1.JobSpec) {
	entries := spec.GetNetwork().GetExactEndpoints()
	if len(entries) == 0 && len(spec.GetTargets()) == 0 {
		return
	}
	if len(spec.GetTargets()) == 0 {
		c.add("Network.ExactEndpoints", ReasonReserved)
		return
	}
	if len(entries) > MaxExactEndpoints {
		c.add("Network.ExactEndpoints", ReasonTooMany)
		return
	}
	seen := map[string]bool{}
	for i, e := range entries {
		if _, _, ok := ParseEndpoint(e); !ok {
			c.add(at("Network.ExactEndpoints", i), ReasonInvalid)
		} else if seen[e] {
			c.add(at("Network.ExactEndpoints", i), ReasonDuplicate)
		}
		seen[e] = true
	}
	for i, t := range spec.GetTargets() {
		for _, h := range t.GetHosts() {
			if !seen[Endpoint(h.GetHost(), int(h.GetPort()))] {
				c.add(at("Targets", i)+".Hosts", ReasonInvalid)
				break
			}
		}
	}
}

func isTargetKey(name string) bool {
	_, ok := ParseTargetKeySecret(name)
	return ok
}
