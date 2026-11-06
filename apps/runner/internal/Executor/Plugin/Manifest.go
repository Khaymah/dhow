package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	ManifestVersion   = 1
	MaxManifestBytes  = 64 << 10
	MaxEntrypointArgs = 32
	MaxArgBytes       = 4 << 10
	MaxOutputs        = 64
	MaxOutputValue    = 1 << 10
	maxExitReasons    = 32
	InputEnvPrefix    = "DHOW_INPUT_"
	EnvOutput         = "DHOW_OUTPUT"
	EnvDeployReport   = "DHOW_DEPLOY_REPORT"
	FileSuffix        = "_FILE"
	DigestSuffix      = "_DIGEST"

	KindValue    = "value"
	KindArtifact = "artifact"
)

var (
	ErrManifest     = errors.New("the plugin image has no valid io.dhow.plugin.manifest label")
	ErrInputUnknown = errors.New("the step sets an input that the plugin does not declare")
	ErrInputMissing = errors.New("the step leaves out an input that the plugin requires")
	ErrOutputs      = errors.New("the step wrote outputs that are not key=value lines within the limits")
	inputPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	outputPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

type Input struct {
	Required bool    `json:"Required"`
	Default  *string `json:"Default"`
	Kind     string  `json:"Kind"`
}

func (in Input) IsArtifact() bool { return in.Kind == KindArtifact }

type Manifest struct {
	Version    int              `json:"Version"`
	Entrypoint []string         `json:"Entrypoint"`
	Inputs     map[string]Input `json:"Inputs"`
	Outputs    []string         `json:"Outputs"`
	Mutating   bool             `json:"Mutating"`

	ExitReasons map[string]string `json:"ExitReasons"`
}

var deployReasons = []string{
	"DEPLOY_FAILED", "DEPLOY_ROLLED_BACK", "DEPLOY_HEALTH_FAILED", "DEPLOY_ROLLBACK_FAILED", "TARGET_LOCKED", "CURRENT_NOT_A_SYMLINK",
	"TARGET_HOST_UNREACHABLE", "TARGET_STATE_CORRUPT", "TARGET_CHECK_FAILED", "HOST_KEY_MISMATCH", "ARTIFACT_UNSAFE", "PLUGIN_INPUT_INVALID",
}

func (m Manifest) ExitReason(code int32) (string, bool) {
	name, ok := m.ExitReasons[strconv.Itoa(int(code))]
	return name, ok && m.Mutating
}

func Parse(label string) (Manifest, error) {
	if label == "" || len(label) > MaxManifestBytes {
		return Manifest{}, ErrManifest
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(label)))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil || dec.More() {
		return Manifest{}, ErrManifest
	}
	if err := m.check(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (m Manifest) check() error {
	if m.Version != ManifestVersion || len(m.Entrypoint) == 0 || len(m.Entrypoint) > MaxEntrypointArgs {
		return ErrManifest
	}
	first := m.Entrypoint[0]
	if !path.IsAbs(first) || path.Clean(first) != first {
		return fmt.Errorf("%w: the entrypoint must start with an absolute path", ErrManifest)
	}
	for _, a := range m.Entrypoint {
		if len(a) > MaxArgBytes || strings.ContainsRune(a, 0) {
			return ErrManifest
		}
	}
	if len(m.Inputs) > jobspec.MaxPluginInputs || len(m.Outputs) > MaxOutputs {
		return ErrManifest
	}
	for name, in := range m.Inputs {
		if !inputPattern.MatchString(name) || (in.Default != nil && (len(*in.Default) > jobspec.MaxPluginInputBytes || strings.ContainsRune(*in.Default, 0))) {
			return ErrManifest
		}
		if in.Kind != "" && in.Kind != KindValue && in.Kind != KindArtifact {
			return fmt.Errorf("%w: input %s has the unknown kind %q", ErrManifest, name, in.Kind)
		}
		if in.IsArtifact() && (in.Default != nil || (!in.Required && !m.Mutating)) {
			return fmt.Errorf("%w: the artifact input %s must have no default and, unless the plugin is mutating, be required", ErrManifest, name)
		}
	}
	for _, o := range m.Outputs {
		if !outputPattern.MatchString(o) {
			return ErrManifest
		}
	}
	return m.checkExitReasons()
}

func (m Manifest) checkExitReasons() error {
	if len(m.ExitReasons) > 0 && !m.Mutating {
		return fmt.Errorf("%w: only a mutating plugin maps exit codes to failure reasons", ErrManifest)
	}
	if len(m.ExitReasons) > maxExitReasons {
		return ErrManifest
	}
	for code, name := range m.ExitReasons {
		n, err := strconv.Atoi(code)
		if err != nil || n < 1 || n > 255 || strconv.Itoa(n) != code || !slices.Contains(deployReasons, name) {
			return fmt.Errorf("%w: the exit code %q maps to an unknown failure reason", ErrManifest, code)
		}
	}
	return nil
}

func (m Manifest) Env(with map[string]string) (map[string]string, error) {
	names := make([]string, 0, len(with))
	for name := range with {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if _, ok := m.Inputs[name]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrInputUnknown, name)
		}
	}
	env := map[string]string{}
	declared := make([]string, 0, len(m.Inputs))
	for name := range m.Inputs {
		declared = append(declared, name)
	}
	slices.Sort(declared)
	for _, name := range declared {
		in := m.Inputs[name]
		v, ok := with[name]
		switch {
		case ok:
		case in.Default != nil:
			v = *in.Default
		case in.Required:
			return nil, fmt.Errorf("%w: %s", ErrInputMissing, name)
		default:
			continue
		}
		env[InputEnvPrefix+strings.ToUpper(name)] = v
	}
	return env, nil
}

func ParseOutputs(raw []byte, allowed []string) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || !outputPattern.MatchString(k) || len(v) > MaxOutputValue || strings.ContainsAny(v, "\r\x00") {
			return nil, ErrOutputs
		}
		if allowed != nil && !slices.Contains(allowed, k) {
			return nil, fmt.Errorf("%w: %s is not in the plugin manifest", ErrOutputs, k)
		}
		out[k] = v
	}
	if len(out) > MaxOutputs {
		return nil, ErrOutputs
	}
	return out, nil
}
