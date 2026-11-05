package pipelines

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	deployments "github.com/Khaymah/dhow/apps/server/internal/Core/Deployments"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines/Models"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/pipelinespec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	inputArtifact       = "artifact"
	inputHealthURL      = "health_check_url"
	inputHealthDelay    = "health_check_start_delay"
	inputHealthRetries  = "health_check_retries"
	inputHealthInterval = "health_check_interval"
	inputHealthTimeout  = "health_check_timeout"

	defaultHealthRetries  = 10
	defaultHealthInterval = 5 * time.Second
	defaultHealthTimeout  = 5 * time.Second
	maxHealthDelay        = 300 * time.Second
	maxHealthRetries      = 60
	maxHealthInterval     = 60 * time.Second
	maxHealthTimeout      = 30 * time.Second
	healthBudgetMax       = 10 * time.Minute
	rollingBudgetMax      = 60 * time.Minute
	shaPrefixLength       = 8
	missingShaPrefix      = "nosha"
)

type candidate struct {
	plan   *pipelinespec.RunJob
	spec   *runnerv1.JobSpec
	eval   pipelinespec.Context
	origin []int
}

type targetStep struct {
	index   int
	origin  int
	dynamic bool
	name    string
	image   string
}

type denial struct {
	reason string
	names  []string
}

func (s *Service) UseTargets(t deployments.Targets) { s.targets = t }

func (c candidate) targetSteps() []targetStep {
	var out []targetStep
	for n, st := range c.spec.GetSteps() {
		p := st.GetPlugin()
		if p == nil || n >= len(c.origin) {
			continue
		}
		o := c.origin[n]
		name, literal := p.GetInputs()[jobspec.TargetInput]
		_, expression := c.plan.Expressions[fmt.Sprintf("Steps[%d].With.%s", o, jobspec.TargetInput)]
		secret := o < len(c.plan.Steps) && c.plan.Steps[o].Plugin != nil && c.plan.Steps[o].Plugin.Secrets[jobspec.TargetInput] != ""
		if !literal && !expression && !secret {
			continue
		}
		out = append(out, targetStep{index: n, origin: o, dynamic: expression || secret, name: name, image: p.GetImage()})
	}
	return out
}

func (s *Service) attachTargets(ctx context.Context, q *sqlc.Queries, st *step, i int, c candidate, env *deployments.Environment) (denial, error) {
	steps := c.targetSteps()
	if len(steps) > 0 && st.run.IsFork {
		return denial{reason: models.ReasonForkEnvironment}, nil
	}
	if env != nil && s.targets != nil {
		if d, err := s.keysNotReferenced(ctx, q, st, c, env); err != nil || d.reason != "" {
			return d, err
		}
	}
	if len(steps) == 0 {
		return denial{}, nil
	}
	if env == nil {
		return denial{reason: models.ReasonTargetNeedsEnvironment}, nil
	}
	if s.targets == nil {
		return denial{reason: models.ReasonEnvironments}, nil
	}
	if len(steps) > jobspec.MaxTargets {
		return denial{reason: models.ReasonJobSpecInvalid}, nil
	}
	if len(c.spec.GetNetwork().GetEgress()) > 0 && !env.AllowExtraDeployEgress {
		return denial{reason: models.ReasonTargetEgress}, nil
	}
	endpoints := []string{}
	for n, ts := range steps {
		t, d, err := s.checkTarget(ctx, q, st, i, c, env, ts)
		if err != nil || d.reason != "" {
			return d, err
		}
		health, d := healthOf(c.spec.GetSteps()[ts.index].GetPlugin().GetInputs(), t, env.AllowExtraDeployEgress)
		if d.reason != "" {
			return d, nil
		}
		attachTarget(st, c.spec, n, ts, t)
		for _, e := range append(hostEndpoints(t), health...) {
			if !slices.Contains(endpoints, e) {
				endpoints = append(endpoints, e)
			}
		}
	}
	if c.spec.Network == nil {
		c.spec.Network = &runnerv1.Network{}
	}
	c.spec.Network.ExactEndpoints = endpoints
	return denial{}, nil
}

func (s *Service) checkTarget(
	ctx context.Context, q *sqlc.Queries, st *step, i int, c candidate, env *deployments.Environment, ts targetStep,
) (deployments.DeployTarget, denial, error) {
	if ts.dynamic {
		return deployments.DeployTarget{}, denial{reason: models.ReasonTargetNotLiteral}, nil
	}
	t, found, err := s.targets.Find(ctx, q, st.run.OrganizationId, env.ID, ts.name)
	if err != nil {
		return t, denial{}, err
	}
	if !found {
		return t, denial{reason: models.ReasonTargetNotFound, names: []string{ts.name}}, nil
	}
	if reason := TargetPluginRefusal(ts.image, t.AllowedImages); reason != "" {
		return t, denial{reason: reason, names: []string{ts.name}}, nil
	}
	if unpinned := unpinnedHosts(t); len(unpinned) > 0 {
		return t, denial{reason: models.ReasonTargetHostKey, names: unpinned}, nil
	}
	if !t.KeyFound {
		return t, denial{reason: models.ReasonTargetKeyNotFound, names: nonEmpty(t.KeySecretName)}, nil
	}
	inputs := c.spec.GetSteps()[ts.index].GetPlugin().GetInputs()
	if !s.artifactAvailable(st, st.rows[i], inputs[inputArtifact], isOfficialDeploy(ts.image)) {
		return t, denial{reason: models.ReasonTargetArtifact}, nil
	}
	return t, denial{}, nil
}

func nonEmpty(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}

func unpinnedHosts(t deployments.DeployTarget) []string {
	if len(t.Hosts) == 0 {
		return []string{t.Name}
	}
	var out []string
	for _, h := range t.Hosts {
		if h.HostKey == "" {
			out = append(out, jobspec.Endpoint(h.Host, h.Port))
		}
	}
	return out
}

func (s *Service) artifactAvailable(st *step, rj sqlc.RunJob, name string, required bool) bool {
	if name == "" {
		return !required
	}
	for _, key := range rj.Needs {
		plan, ok := st.c.byKey[key]
		if ok && slices.ContainsFunc(plan.Artifacts, func(a pipelinespec.ArtifactPlan) bool { return a.Name == name }) {
			return true
		}
	}
	return false
}

func hostEndpoints(t deployments.DeployTarget) []string {
	out := make([]string, 0, len(t.Hosts))
	for _, h := range t.Hosts {
		out = append(out, jobspec.Endpoint(h.Host, h.Port))
	}
	return out
}

func healthOf(inputs map[string]string, t deployments.DeployTarget, extra bool) ([]string, denial) {
	hosts := make([]string, 0, len(t.Hosts))
	for _, h := range t.Hosts {
		hosts = append(hosts, h.Host)
	}
	endpoints, err := jobspec.HealthEndpoints(inputs[inputHealthURL], hosts)
	if errors.Is(err, jobspec.ErrHealthURL) {
		return nil, denial{reason: models.ReasonHealthURLInvalid}
	}
	for _, e := range endpoints {
		if host, _, ok := jobspec.ParseEndpoint(e); !ok || !jobspec.HealthHostAllowed(host, hosts, extra) {
			return nil, denial{reason: models.ReasonHealthURLNotAllowed}
		}
	}
	perHost := time.Duration(0)
	if inputs[inputHealthURL] != "" {
		perHost = healthBudget(inputs)
	}
	if perHost > healthBudgetMax {
		return nil, denial{reason: models.ReasonHealthBudget}
	}
	batch := max(t.Settings.BatchSize, 1)
	batches := (len(t.Hosts) + batch - 1) / batch
	pause := time.Duration(t.Settings.BatchPauseSeconds) * time.Second
	if time.Duration(batches)*perHost+time.Duration(max(batches-1, 0))*pause > rollingBudgetMax {
		return nil, denial{reason: models.ReasonRollingBudget}
	}
	return endpoints, denial{}
}

func seconds(inputs map[string]string, name string, fallback, limit time.Duration) time.Duration {
	raw, ok := inputs[name]
	if !ok {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return fallback
	}
	return min(d, limit)
}

func healthBudget(inputs map[string]string) time.Duration {
	retries := defaultHealthRetries
	if n, err := strconv.Atoi(inputs[inputHealthRetries]); err == nil && n > 0 {
		retries = min(n, maxHealthRetries)
	}
	delay := seconds(inputs, inputHealthDelay, 0, maxHealthDelay)
	interval := seconds(inputs, inputHealthInterval, defaultHealthInterval, maxHealthInterval)
	timeout := seconds(inputs, inputHealthTimeout, defaultHealthTimeout, maxHealthTimeout)
	return delay + time.Duration(retries)*(interval+timeout)
}

func releaseID(run sqlc.PipelineRun) string {
	sha := run.Sha.String
	if len(sha) < shaPrefixLength {
		sha = missingShaPrefix
	}
	return fmt.Sprintf("%d-%s-%d", run.Number, sha[:min(len(sha), shaPrefixLength)], run.Round)
}

func int32Of(n int) int32 {
	return int32(n) //nolint:gosec // Every value here is bounded by a limit of the job spec.
}

func attachTarget(st *step, spec *runnerv1.JobSpec, n int, ts targetStep, t deployments.DeployTarget) {
	hosts := make([]*runnerv1.TargetHost, 0, len(t.Hosts))
	for _, h := range t.Hosts {
		hosts = append(hosts, &runnerv1.TargetHost{
			HostId: h.ID.String(), Host: h.Host, Port: int32Of(h.Port), KnownHostsLine: jobspec.KnownHostsLine(h.Host, h.Port, h.HostKey),
		})
	}
	s := t.Settings
	spec.Steps[ts.index].Mutating = true
	spec.Targets = append(spec.Targets, &runnerv1.StepTarget{
		StepIndex: int32Of(ts.index), TargetId: t.ID.String(), Version: t.Version, Name: t.Name, Kind: t.Kind, User: t.Username,
		DeployPath: t.DeployPath, ReleaseId: releaseID(st.run), KeepReleases: int32Of(t.KeepReleases), Hosts: hosts,
		Settings: &runnerv1.TargetSettings{
			BatchSize: int32Of(max(s.BatchSize, 1)), BatchPauseSeconds: int32Of(s.BatchPauseSeconds), ReloadCommand: s.Hooks.Reload,
			PreSwitchCommand: s.Hooks.PreSwitch, PostSwitchCommand: s.Hooks.PostSwitch, Shared: slices.Clone(s.Shared),
			ComposeProject: s.ComposeProject, ComposeFile: s.ComposeFile, AllowUnsafeCompose: s.AllowUnsafe,
		},
	})
	spec.Secrets = append(spec.Secrets, &runnerv1.SecretRef{
		Name: jobspec.TargetKeySecret(n), Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.TargetKeyFile(n)}, StepIndexes: []int32{int32Of(ts.index)},
	})
}

func (d denial) detail() []byte {
	if len(d.names) == 0 {
		return nil
	}
	return failureDetail(d.names)
}

func repositoryOf(image string) string {
	repo, _, _ := strings.Cut(image, "@")
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	return repo
}

func isOfficialDeploy(image string) bool {
	return slices.Contains(jobspec.OfficialDeployImages(), repositoryOf(jobspec.QualifyImage(image)))
}

func TargetPluginRefusal(image string, allowed []string) string {
	switch {
	case !isOfficialDeploy(image) && (len(allowed) == 0 || !PluginAllowed(image, allowed)):
		return models.ReasonTargetPluginNotAllowed
	case !strings.Contains(image, digestMarker):
		return models.ReasonTargetPluginNotPinned
	default:
		return ""
	}
}

func (s *Service) keysNotReferenced(ctx context.Context, q *sqlc.Queries, st *step, c candidate, env *deployments.Environment) (denial, error) {
	keys, err := s.targets.KeyNames(ctx, q, st.run.OrganizationId, env.ID)
	if err != nil || len(keys) == 0 {
		return denial{}, err
	}
	var hit []string
	for _, n := range userSecretNames(c.spec) {
		if slices.ContainsFunc(keys, func(k string) bool { return strings.EqualFold(k, n) }) && !slices.Contains(hit, n) {
			hit = append(hit, n)
		}
	}
	if len(hit) == 0 {
		return denial{}, nil
	}
	return denial{reason: models.ReasonTargetKeyReferenced, names: hit}, nil
}
