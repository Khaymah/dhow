package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	environments "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	pipelines "github.com/Khaymah/dhow/apps/server/internal/Modules/Pipelines"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const deliveredValueMax = 256

type pipelineSecrets struct {
	runs   *pipelines.Service
	envs   *environments.Service
	store  secretstore.Store
	logins secretstore.RegistryLogins
	bus    *eventbus.Bus
	log    *slog.Logger
}

type registryValue struct {
	Username string
	Password string
}

var _ jobs.SecretResolver = (*pipelineSecrets)(nil)

func (p *pipelineSecrets) Accepts(_ context.Context, origin jobs.Origin, _ []string) error {
	if origin.Kind != jobs.OriginPipeline || origin.ID == uuid.Nil {
		return jobs.ErrSecretsUnavailable
	}
	return nil
}

func (p *pipelineSecrets) Resolve(ctx context.Context, req jobs.SecretRequest) (map[string][]byte, error) {
	scope, err := p.runs.SecretScopeOfJob(ctx, req.OrganizationID, req.JobID)
	if errors.Is(err, pipelines.ErrNoSecretScope) {
		return nil, jobs.ErrSecretsUnavailable
	}
	if err != nil {
		return nil, err
	}
	if scope.IsFork {
		p.log.WarnContext(ctx, "a job of a fork asked for secrets at start and got none", "job_id", req.JobID.String())
		return nil, jobs.ErrSecretsUnavailable
	}
	if err := p.checkEnvironment(ctx, req, scope); err != nil {
		return nil, err
	}
	names, registries := splitRegistry(req)
	list, err := p.resolveNamed(ctx, req, scope, names)
	if errors.Is(err, secretstore.ErrMissingName) || errors.Is(err, secretstore.ErrFork) ||
		errors.Is(err, secretstore.ErrTooMany) || errors.Is(err, secretstore.ErrTooLarge) {
		return nil, jobs.ErrSecretsUnavailable
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(list)+len(registries))
	for _, s := range list {
		out[s.Name] = s.Value
	}
	hosts, err := p.resolveRegistries(ctx, req, scope, registries, out)
	if err != nil {
		wipeAll(out)
		return nil, err
	}
	if err := p.audit(ctx, req, scope, list, hosts); err != nil {
		wipeAll(out)
		return nil, err
	}
	return out, nil
}

func (p *pipelineSecrets) checkEnvironment(ctx context.Context, req jobs.SecretRequest, scope pipelines.SecretScope) error {
	if scope.EnvironmentID == nil {
		return nil
	}
	if p.envs == nil {
		return jobs.ErrSecretsUnavailable
	}
	env, err := p.envs.CheckStart(ctx, environments.StartCheck{
		OrganizationID: req.OrganizationID, RunJobID: scope.RunJobID, JobID: req.JobID,
		RefKind: scope.Target.Kind, RefName: scope.Target.Name, PullRequest: scope.Target.PullRequest,
	})
	if errors.Is(err, environments.ErrRuleChanged) || (err == nil && env != *scope.EnvironmentID) {
		p.log.WarnContext(ctx, "the environment of a job changed after the gate, so it gets no secret", "job_id", req.JobID.String())
		return jobs.ErrEnvironmentRuleChanged
	}
	return err
}

func wipeAll(values map[string][]byte) {
	for _, v := range values {
		clear(v)
	}
}

func splitRegistry(req jobs.SecretRequest) ([]string, map[string]string) {
	names := make([]string, 0, len(req.Names))
	registries := map[string]string{}
	for _, n := range req.Names {
		if host, ok := req.Registries[n]; ok && strings.HasPrefix(n, jobspec.RegistrySecretPrefix) {
			registries[n] = host
			continue
		}
		names = append(names, n)
	}
	return names, registries
}

func (p *pipelineSecrets) resolveNamed(ctx context.Context, req jobs.SecretRequest, scope pipelines.SecretScope, names []string) ([]secretstore.Secret, error) {
	if len(names) == 0 {
		return nil, nil
	}
	return p.store.Resolve(ctx, secretstore.Request{
		OrganizationID: req.OrganizationID, ProjectID: scope.ProjectID, EnvironmentID: scope.EnvironmentID, Names: names,
		PullRequest: scope.PullRequest,
	})
}

func (p *pipelineSecrets) resolveRegistries(
	ctx context.Context, req jobs.SecretRequest, scope pipelines.SecretScope, registries map[string]string, out map[string][]byte,
) ([]string, error) {
	hosts := make([]string, 0, len(registries))
	if len(registries) > 0 && scope.PullRequest {
		return nil, jobs.ErrSecretsUnavailable
	}
	for _, name := range slices.Sorted(maps.Keys(registries)) {
		if p.logins == nil {
			return nil, jobs.ErrSecretsUnavailable
		}
		login, ok, err := p.logins.RegistryLogin(ctx, req.OrganizationID, scope.ProjectID, registries[name])
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, jobs.ErrSecretsUnavailable
		}
		raw, err := json.Marshal(registryValue{Username: login.Username, Password: string(login.Password)})
		login.Password.Wipe()
		if err != nil {
			return nil, err
		}
		out[name] = raw
		hosts = append(hosts, registries[name])
	}
	return hosts, nil
}

func deliveredList(list []secretstore.Secret) (string, bool) {
	parts := make([]string, 0, len(list))
	for _, s := range list {
		parts = append(parts, s.Name+":"+s.Scope+":v"+strconv.FormatInt(int64(s.Version), 10))
	}
	v := strings.Join(parts, ",")
	if len(v) <= deliveredValueMax {
		return v, false
	}
	end := deliveredValueMax
	for end > 0 && !utf8.RuneStart(v[end]) {
		end--
	}
	return v[:end], true
}

func (p *pipelineSecrets) audit(ctx context.Context, req jobs.SecretRequest, scope pipelines.SecretScope, list []secretstore.Secret, hosts []string) error {
	value, cut := deliveredList(list)
	return p.runs.SecretsDelivered(ctx, pipelines.Delivered{
		OrganizationID: req.OrganizationID, JobID: req.JobID, AttemptID: req.AttemptID, Scope: scope, Names: value, Truncated: cut, Registries: hosts,
	})
}
