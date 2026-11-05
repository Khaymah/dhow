package executor

import (
	"errors"
	"os"
	"slices"
	"strings"

	plugin "github.com/Khaymah/dhow/apps/runner/internal/Executor/Plugin"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var (
	ErrTargetKeyMissing = errors.New("the server sent no key for the deploy target of this step")
	ErrArtifactMissing  = errors.New("the plugin asks for an artifact that was not delivered to this step")
)

type archive struct {
	path   string
	digest string
}

func targetAt(spec *runnerv1.JobSpec, idx int32) (int, *runnerv1.StepTarget, bool) {
	for n, t := range spec.GetTargets() {
		if t.GetStepIndex() == idx {
			return n, t, true
		}
	}
	return 0, nil, false
}

func targetArtifact(spec *runnerv1.JobSpec, idx int32) (string, bool) {
	if _, _, ok := targetAt(spec, idx); !ok || idx < 0 || int(idx) >= len(spec.GetSteps()) {
		return "", false
	}
	name, ok := spec.GetSteps()[idx].GetPlugin().GetInputs()[jobspec.TargetArtifactInput]
	if !ok || !slices.ContainsFunc(spec.GetInputs(), func(in *runnerv1.ArtifactInput) bool { return in.GetName() == name }) {
		return "", false
	}
	return name, true
}

func archiveNames(spec *runnerv1.JobSpec) []string {
	var out []string
	for i := range spec.GetSteps() {
		if name, ok := targetArtifact(spec, int32(i)); ok && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func foreignTargetKey(spec *runnerv1.JobSpec, name string, idx int32) bool {
	n, reserved := jobspec.ParseTargetKeySecret(name)
	if !reserved {
		return false
	}
	own, _, ok := targetAt(spec, idx)
	return !ok || own != n
}

func (a *attempt) targetEnv(idx int32) (map[string]string, error) {
	spec := a.grant.GetSpec()
	n, t, ok := targetAt(spec, idx)
	if !ok {
		return nil, nil
	}
	if len(a.secrets[jobspec.TargetKeySecret(n)]) == 0 {
		return nil, ErrTargetKeyMissing
	}
	env, err := jobspec.TargetEnv(t, n, sandbox.SecretsPath)
	if err != nil {
		return nil, err
	}
	env[plugin.EnvDeployReport] = sandbox.ReportPath
	return env, nil
}

func (a *attempt) artifactEnv(m plugin.Manifest, with map[string]string, idx int32) (map[string]string, error) {
	names := m.ArtifactNames(with)
	if len(names) == 0 {
		return nil, nil
	}
	want, delivered := targetArtifact(a.grant.GetSpec(), idx)
	env := map[string]string{}
	for input, name := range names {
		got, ok := a.archives[name]
		if !delivered || name != want || !ok {
			return nil, ErrArtifactMissing
		}
		key := plugin.InputEnvPrefix + strings.ToUpper(input)
		env[key+plugin.FileSuffix] = sandbox.InputsPath + "/" + name
		env[key+plugin.DigestSuffix] = got.digest
	}
	return env, nil
}

func (a *attempt) inputFiles(names []string) []sandbox.InputFile {
	out := make([]sandbox.InputFile, 0, len(names))
	for _, name := range names {
		if ar, ok := a.archives[name]; ok {
			out = append(out, sandbox.InputFile{Name: name, Path: ar.path, Digest: ar.digest})
		}
	}
	return out
}

func (a *attempt) dropArchives(names []string) {
	for _, name := range names {
		if ar, ok := a.archives[name]; ok {
			_ = os.Remove(ar.path)
			delete(a.archives, name)
		}
	}
}
