package jobspec

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	imageDomain    = `(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)(?:\.(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?))*(?::[0-9]{1,5})?`
	imageComponent = `[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*`
	imageTag       = `[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}`
	maxPort        = 65535
)

var (
	imagePattern = regexp.MustCompile(`^(?:` + imageDomain + `/)?` + imageComponent + `(?:/` + imageComponent + `)*(?::` + imageTag + `)?@sha256:[a-f0-9]{64}$`)
	imageAnyRef  = regexp.MustCompile(`^(?:` + imageDomain + `/)?` + imageComponent + `(?:/` + imageComponent + `)*(?::` + imageTag + `)?$`)
	envName      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	secretName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	secretFile   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	labelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?(=[A-Za-z0-9]([A-Za-z0-9._-]{0,62}[A-Za-z0-9])?)?$`)
)

type checker struct {
	violations []Violation
}

func (c *checker) add(field, reason string) {
	c.violations = append(c.violations, Violation{Field: field, Reason: reason})
}

func at(field string, i int) string { return fmt.Sprintf("%s[%d]", field, i) }

func Validate(spec *runnerv1.JobSpec) error {
	if spec == nil {
		return ErrNilSpec
	}
	c := &checker{}
	if proto.Size(spec) > MaxSpecBytes {
		c.add("Spec", ReasonTooLong)
		return &Error{Violations: c.violations}
	}
	if HasRunStep(spec) || spec.GetImage() != "" {
		c.image(spec.GetImage())
	}
	c.steps(spec.GetSteps(), spec.GetTimeoutSeconds())
	c.checkout(spec)
	c.env("Env", spec.GetEnv())
	c.secrets(spec)
	c.registryAuth(spec)
	c.resources(spec.GetResources())
	if t := spec.GetTimeoutSeconds(); t < MinTimeoutSeconds || t > MaxTimeoutSeconds {
		c.add("TimeoutSeconds", ReasonOutOfRange)
	}
	c.egress(spec.GetNetwork().GetEgress())
	if IsolationName(spec.GetIsolation()) == "" {
		c.add("Isolation", ReasonInvalid)
	}
	c.labels(spec.GetLabels())
	c.transfers(spec)
	c.derived(spec)
	if len(c.violations) > 0 {
		return &Error{Violations: c.violations}
	}
	return nil
}

func (c *checker) image(v string) {
	if r := ImageReason(v); r != "" {
		c.add("Image", r)
	}
}

func ImageReason(v string) string {
	switch {
	case v == "":
		return ReasonRequired
	case len(v) > MaxImageLength:
		return ReasonTooLong
	case imagePattern.MatchString(v):
		return ""
	case imageAnyRef.MatchString(v):
		return ReasonNotPinned
	default:
		return ReasonInvalid
	}
}

func validText(v string, maxRunes int) string {
	switch {
	case strings.TrimSpace(v) == "":
		return ReasonRequired
	case utf8.RuneCountInString(v) > maxRunes:
		return ReasonTooLong
	case !utf8.ValidString(v) || strings.IndexFunc(v, unicode.IsControl) >= 0:
		return ReasonInvalid
	default:
		return ""
	}
}

func (c *checker) steps(steps []*runnerv1.Step, jobTimeout int32) {
	switch {
	case len(steps) == 0:
		c.add("Steps", ReasonRequired)
		return
	case len(steps) > MaxSteps:
		c.add("Steps", ReasonTooMany)
		return
	}
	ids := map[string]bool{}
	for i, s := range steps {
		field := at("Steps", i)
		if r := validText(s.GetName(), MaxStepNameLength); r != "" {
			c.add(field+".Name", r)
		}
		c.stepID(field, s.GetId(), ids)
		c.stepBody(field, s)
		c.env(field+".Env", s.GetEnv())
		if t := s.GetTimeoutSeconds(); t < 0 || t > jobTimeout {
			c.add(field+".TimeoutSeconds", ReasonOutOfRange)
		}
	}
}

func (c *checker) envName(field, name string) {
	switch {
	case name == "" || !envName.MatchString(name):
		c.add(field, ReasonInvalid)
	case len(name) > MaxEnvNameLength:
		c.add(field, ReasonTooLong)
	case strings.HasPrefix(strings.ToUpper(name), ReservedEnvPrefix):
		c.add(field, ReasonReserved)
	}
}

func (c *checker) env(field string, env map[string]string) {
	if len(env) > MaxEnvEntries {
		c.add(field, ReasonTooMany)
		return
	}
	for _, name := range sortedKeys(env) {
		c.envName(field+"."+name, name)
		v := env[name]
		switch {
		case len(v) > MaxEnvValueBytes:
			c.add(field+"."+name, ReasonTooLong)
		case strings.ContainsRune(v, 0):
			c.add(field+"."+name, ReasonInvalid)
		}
	}
}

func (c *checker) secrets(spec *runnerv1.JobSpec) {
	secrets := spec.GetSecrets()
	if len(secrets) > MaxSecrets {
		c.add("Secrets", ReasonTooMany)
		return
	}
	targets := map[string]bool{}
	for i, s := range secrets {
		field := at("Secrets", i)
		switch {
		case s.GetName() == CheckoutSecret:
		case s.GetName() == "" || !secretName.MatchString(s.GetName()):
			c.add(field+".Name", ReasonInvalid)
		case len(s.GetName()) > MaxSecretNameLength:
			c.add(field+".Name", ReasonTooLong)
		}
		target := c.secretTarget(field, s, spec)
		if target != "" && targets[target] {
			c.add(field, ReasonDuplicate)
		}
		targets[target] = true
		for _, idx := range s.GetStepIndexes() {
			if idx < 0 || int(idx) >= len(spec.GetSteps()) {
				c.add(field+".Steps", ReasonOutOfRange)
				break
			}
		}
	}
}

func (c *checker) secretTarget(field string, s *runnerv1.SecretRef, spec *runnerv1.JobSpec) string {
	switch t := s.GetTarget().(type) {
	case *runnerv1.SecretRef_AsEnv:
		if strings.HasPrefix(t.AsEnv, PluginInputEnvPrefix) {
			c.pluginSecret(field, t.AsEnv, s, spec)
			return "env:" + t.AsEnv + ":" + fmt.Sprint(s.GetStepIndexes())
		}
		c.envName(field+".AsEnv", t.AsEnv)
		if usesEnv(spec, t.AsEnv) {
			c.add(field+".AsEnv", ReasonDuplicate)
		}
		return "env:" + t.AsEnv
	case *runnerv1.SecretRef_AsFile:
		name := t.AsFile
		switch {
		case name == "." || name == ".." || !secretFile.MatchString(name):
			c.add(field+".AsFile", ReasonInvalid)
		case len(name) > MaxSecretFileLength:
			c.add(field+".AsFile", ReasonTooLong)
		}
		return "file:" + name
	default:
		c.add(field+".AsEnv", ReasonRequired)
		return ""
	}
}

func ValidSecretFile(name string) bool {
	return name != "." && name != ".." && secretFile.MatchString(name) && len(name) <= MaxSecretFileLength
}

func usesEnv(spec *runnerv1.JobSpec, name string) bool {
	if _, ok := spec.GetEnv()[name]; ok {
		return true
	}
	for _, s := range spec.GetSteps() {
		if _, ok := s.GetEnv()[name]; ok {
			return true
		}
	}
	return false
}

func (c *checker) resources(r *runnerv1.Resources) {
	bounds := []struct {
		field    string
		v        int64
		min, max int64
	}{
		{"Resources.CpuMillis", r.GetCpuMillis(), MinCPUMillis, MaxCPUMillis},
		{"Resources.MemoryBytes", r.GetMemoryBytes(), MinMemoryBytes, MaxMemoryBytes},
		{"Resources.Pids", r.GetPids(), MinPids, MaxPids},
		{"Resources.DiskBytes", r.GetDiskBytes(), MinDiskBytes, MaxDiskBytes},
	}
	for _, b := range bounds {
		if b.v < b.min || b.v > b.max {
			c.add(b.field, ReasonOutOfRange)
		}
	}
}

func (c *checker) egress(entries []string) {
	if len(entries) > MaxEgressEntries {
		c.add("Network.Egress", ReasonTooMany)
		return
	}
	for i, e := range entries {
		if !validEgress(e) {
			c.add(at("Network.Egress", i), ReasonInvalid)
		}
	}
}

func validEgress(e string) bool {
	_, err := ParseEgress(e)
	return err == nil
}

func (c *checker) labels(labels []string) {
	if len(labels) > MaxLabels {
		c.add("Labels", ReasonTooMany)
		return
	}
	for i, l := range labels {
		if !ValidLabel(l) {
			c.add(at("Labels", i), ReasonInvalid)
		}
	}
}

func ValidLabel(l string) bool {
	return len(l) <= MaxLabelLength && labelPattern.MatchString(l)
}

func (c *checker) registryAuth(spec *runnerv1.JobSpec) {
	auths := spec.GetRegistryAuth()
	if len(auths) > MaxRegistryAuth {
		c.add("RegistryAuth", ReasonTooMany)
		return
	}
	hosts := map[string]bool{}
	for i, r := range auths {
		field := at("RegistryAuth", i)
		host, err := NormalizeHost(r.GetHost())
		if name, port, ok := strings.Cut(r.GetHost(), ":"); ok {
			host, err = NormalizeHost(name)
			if p, perr := strconv.Atoi(port); perr != nil || p < 1 || p > 65535 || strconv.Itoa(p) != port {
				err = errors.New("bad port")
			}
			host += ":" + port
		}
		switch {
		case err != nil || host != r.GetHost():
			c.add(field+".Host", ReasonInvalid)
		case hosts[host]:
			c.add(field+".Host", ReasonDuplicate)
		}
		hosts[host] = true
		suffix, ok := strings.CutPrefix(r.GetSecretName(), RegistrySecretPrefix)
		if n, perr := strconv.Atoi(suffix); !ok || perr != nil || n < 0 || strconv.Itoa(n) != suffix {
			c.add(field+".SecretName", ReasonInvalid)
		}
	}
}
